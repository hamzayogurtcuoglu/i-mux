package limits

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/jsonl"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func usageLine(ts time.Time, id, model string, in, out int) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":%q,"model":%q,"role":"assistant","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":10,"cache_creation_input_tokens":1}}}`+"\n",
		ts.UTC().Format(time.RFC3339Nano), id, model, in, out)
}

func TestClaudeUsage(t *testing.T) {
	h := testutil.Isolate(t)
	now := testutil.FrozenTime
	dir := filepath.Join(h.Claude, "projects", claude.EncodeCWD(h.Project))
	var b bytes.Buffer
	b.WriteString(usageLine(now.Add(-6*time.Hour), "old", "m", 1000, 1000)) // outside the window
	b.WriteString(`{"type":"user","timestamp":"` + now.Add(-4*time.Hour).UTC().Format(time.RFC3339) + `","message":{"role":"user","content":"hi"}}` + "\n")
	b.WriteString(usageLine(now.Add(-4*time.Hour), "m1", "m", 100, 10))
	b.WriteString(usageLine(now.Add(-4*time.Hour), "m1", "m", 100, 10)) // same message, second block
	b.WriteString(usageLine(now.Add(-time.Hour), "m2", "m", 200, 20))
	b.WriteString(usageLine(now.Add(-time.Hour), "e1", "<synthetic>", 0, 0)) // API error reply
	b.WriteString("not json\n")
	p := filepath.Join(dir, "s1.jsonl")
	testutil.WriteFile(t, p, b.Bytes())
	testutil.Touch(t, p, now.Add(-time.Hour))
	// Another project, same message id m2 (counted once), plus one more.
	q := filepath.Join(h.Claude, "projects", "-other", "s2.jsonl")
	testutil.WriteFile(t, q, []byte(usageLine(now.Add(-time.Hour), "m2", "m", 200, 20)+usageLine(now.Add(-30*time.Minute), "m3", "m", 300, 30)))
	testutil.Touch(t, q, now.Add(-30*time.Minute))
	// A file not modified in the window is not read.
	r := filepath.Join(dir, "s3.jsonl")
	testutil.WriteFile(t, r, []byte(usageLine(now.Add(-time.Hour), "m9", "m", 9999, 9999)))
	testutil.Touch(t, r, now.Add(-10*time.Hour))

	w := ClaudeUsage(session.Env{}, now, 5*time.Hour)
	if w.Replies != 3 || w.Input != 600 || w.Output != 60 || w.CacheRead != 30 || w.CacheWrite != 3 || w.Files != 2 {
		t.Fatalf("window %+v", w)
	}
	if !w.From.Equal(now.Add(-5*time.Hour)) || !w.To.Equal(now) {
		t.Fatalf("bounds %v %v", w.From, w.To)
	}
}

// TestClaudeUsageLargeFixture: the 64 MB session is handled in < 300 ms
// (T4.1).
func TestClaudeUsageLargeFixture(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("performance test (skipped with -short and -race)")
	}
	h := testutil.Isolate(t)
	p := filepath.Join(h.Claude, "projects", claude.EncodeCWD(h.Project), "big.jsonl")
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.GenClaudeSession(f, 64<<20, h.Project, "big"); err != nil {
		t.Fatal(err)
	}
	fi, _ := f.Stat()
	rv := jsonl.NewReverse(f, fi.Size(), 0)
	var end time.Time
	for rv.Scan() {
		if ts, ok := lineTime(rv.Bytes()); ok {
			end = ts
			break
		}
	}
	f.Close()
	_ = os.Chtimes(p, end, end)
	best := time.Hour
	var w ClaudeWindow
	for i := 0; i < 3; i++ {
		start := time.Now()
		w = ClaudeUsage(session.Env{}, end.Add(time.Minute), 5*time.Hour)
		if d := time.Since(start); d < best {
			best = d
		}
	}
	t.Logf("64 MB fixture: %v, %d replies in the window", best, w.Replies)
	if w.Replies == 0 {
		t.Fatal("no usage found")
	}
	if best > 300*time.Millisecond {
		t.Errorf("took %v, budget 300 ms", best)
	}
}
