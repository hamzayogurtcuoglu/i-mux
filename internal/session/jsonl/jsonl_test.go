package jsonl

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func collect(t *testing.T, data string, opt Options) ([]string, Stats) {
	t.Helper()
	var got []string
	st, err := ReadAt(strings.NewReader(data), int64(len(data)), opt, func(l []byte, off int64) error {
		if !strings.HasPrefix(data[off:], string(l)) {
			t.Fatalf("offset %d does not point at line %q", off, l)
		}
		got = append(got, string(l))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got, st
}

func TestWholeFile(t *testing.T) {
	got, st := collect(t, "a\nb\r\n\nc", Options{})
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) || st.Gap {
		t.Fatalf("got %q %+v", got, st)
	}
}

func TestEmptyFile(t *testing.T) {
	got, st := collect(t, "", Options{HeadBytes: 10, TailBytes: 10})
	if len(got) != 0 || st.Lines != 0 {
		t.Fatalf("got %q", got)
	}
}

func TestFinalLineWithoutNewline(t *testing.T) {
	got, _ := collect(t, "{\"a\":1}\n{\"b\":", Options{})
	if !reflect.DeepEqual(got, []string{`{"a":1}`, `{"b":`}) {
		t.Fatalf("got %q", got)
	}
}

func TestCRLF(t *testing.T) {
	got, _ := collect(t, "x\r\ny\r\n", Options{})
	if !reflect.DeepEqual(got, []string{"x", "y"}) {
		t.Fatalf("got %q", got)
	}
}

func lines(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "line-%03d\n", i) // 9 bytes each
	}
	return b.String()
}

func TestWindowsSplitLines(t *testing.T) {
	data := lines(100) // 900 bytes
	// Head boundary at 40 falls inside line 4 (36..44): line 4 starts before
	// 40 and is read whole. Tail of 95 bytes starts at 805, inside line 89
	// (801..809): the tail begins at line 90.
	got, st := collect(t, data, Options{HeadBytes: 40, TailBytes: 95})
	want := []string{"line-000", "line-001", "line-002", "line-003", "line-004"}
	for i := 90; i < 100; i++ {
		want = append(want, fmt.Sprintf("line-%03d", i))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if !st.Gap || st.TailStart != 810 || st.GapBytes != 810-45 {
		t.Fatalf("stats %+v", st)
	}
}

func TestTailStartsOnLineBoundary(t *testing.T) {
	data := lines(10)
	got, _ := collect(t, data, Options{HeadBytes: 9, TailBytes: 18})
	if !reflect.DeepEqual(got, []string{"line-000", "line-008", "line-009"}) {
		t.Fatalf("got %q", got)
	}
}

func TestWindowsOverlap(t *testing.T) {
	data := lines(10)
	got, st := collect(t, data, Options{HeadBytes: 50, TailBytes: 45})
	if len(got) != 10 || st.Gap {
		t.Fatalf("got %d lines, gap=%v", len(got), st.Gap)
	}
	// Head ends inside the tail window: no line twice.
	got, st = collect(t, data, Options{HeadBytes: 44, TailBytes: 50})
	if len(got) != 10 || st.Gap {
		t.Fatalf("overlap: got %d %+v", len(got), st)
	}
}

func TestOversizedLineSkipped(t *testing.T) {
	big := strings.Repeat("x", 17<<20)
	data := "first\n" + big + "\nlast\n"
	got, st := collect(t, data, Options{})
	if !reflect.DeepEqual(got, []string{"first", "last"}) || st.SkippedLong != 1 {
		t.Fatalf("got %d lines, skipped %d", len(got), st.SkippedLong)
	}
	if w := st.Warnings(); len(w) != 1 || !strings.Contains(w[0], "oversized") {
		t.Fatalf("warnings %v", w)
	}
}

func TestLongButAllowedLine(t *testing.T) {
	big := strings.Repeat("y", 1<<20) // larger than the bufio buffer
	got, _ := collect(t, "a\n"+big+"\nb", Options{})
	if len(got) != 3 || got[1] != big {
		t.Fatal("long line mangled")
	}
}

func TestStop(t *testing.T) {
	n := 0
	_, err := ReadAt(strings.NewReader(lines(10)), 90, Options{}, func([]byte, int64) error {
		n++
		if n == 3 {
			return ErrStop
		}
		return nil
	})
	if err != nil || n != 3 {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

func TestReadFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(lines(3)), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []string
	if _, err := Read(p, Options{}, func(l []byte, _ int64) error { got = append(got, string(l)); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatal(got)
	}
	if _, err := Read(filepath.Join(t.TempDir(), "missing"), Options{}, nil); err == nil {
		t.Fatal("missing file should error")
	}
}

func reverse(t *testing.T, data string, maxLine int) ([]string, int) {
	t.Helper()
	rv := NewReverse(strings.NewReader(data), int64(len(data)), maxLine)
	rv.buf = make([]byte, 7) // tiny chunks exercise boundaries
	var got []string
	for rv.Scan() {
		got = append(got, string(rv.Bytes()))
	}
	if rv.Err() != nil {
		t.Fatal(rv.Err())
	}
	return got, rv.Skipped()
}

func TestReverse(t *testing.T) {
	got, _ := reverse(t, "one\ntwo\r\n\nthree\nfour", 0)
	if !reflect.DeepEqual(got, []string{"four", "three", "two", "one"}) {
		t.Fatalf("got %q", got)
	}
	got, _ = reverse(t, "", 0)
	if len(got) != 0 {
		t.Fatal(got)
	}
	got, skipped := reverse(t, "a\n"+strings.Repeat("z", 40)+"\nb\n", 10)
	if !reflect.DeepEqual(got, []string{"b", "a"}) || skipped != 1 {
		t.Fatalf("got %q skipped %d", got, skipped)
	}
	data := lines(50)
	got, _ = reverse(t, data, 0)
	if len(got) != 50 || got[0] != "line-049" || got[49] != "line-000" {
		t.Fatalf("got %d lines", len(got))
	}
}

func BenchmarkTailScan(b *testing.B) {
	var buf bytes.Buffer
	line := `{"type":"assistant","message":{"id":"msg_x","content":[{"type":"text","text":"` + strings.Repeat("lorem ipsum ", 40) + `"}]}}` + "\n"
	for buf.Len() < 32<<20 {
		buf.WriteString(line)
	}
	data := buf.Bytes()
	r := bytes.NewReader(data)
	opt := Options{HeadBytes: 1 << 20, TailBytes: 8 << 20}
	b.SetBytes(9 << 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ReadAt(r, int64(len(data)), opt, func([]byte, int64) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}
