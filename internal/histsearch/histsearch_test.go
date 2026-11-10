package histsearch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

func conv(msgs ...string) *session.Session {
	s := &session.Session{}
	for i, m := range msgs {
		role := session.RoleUser
		if i%2 == 1 {
			role = session.RoleAssistant
		}
		s.Entries = append(s.Entries, session.Entry{Role: role, Text: m})
	}
	return s
}

func TestFind(t *testing.T) {
	s := conv("fix the Login bug\nin auth.go", "The login flow calls redirect() twice.", "thanks", "")
	h, ok := Find(s, "login")
	if !ok || h.Count != 2 || len(h.Snips) != 2 {
		t.Fatalf("hit %+v", h)
	}
	if sn := h.Snips[0]; !sn.User || sn.Text != "fix the Login bug in auth.go" || string([]rune(sn.Text)[sn.Col:sn.Col+sn.Len]) != "Login" {
		t.Errorf("first %+v", sn)
	}
	if sn := h.Snips[1]; sn.User || !strings.HasPrefix(sn.Text, "The login flow") {
		t.Errorf("second %+v", sn)
	}
	// Uppercase: case-sensitive.
	if h, _ := Find(s, "Login"); h.Count != 1 {
		t.Errorf("Login: %d", h.Count)
	}
	// Turkish letters fold too; a long message is cut around the match.
	long := strings.Repeat("önce ", 20) + "Çalışan ajanlar burada" + strings.Repeat(" sonra", 40)
	h, ok = Find(conv(long), "çalışan")
	if !ok || !strings.HasPrefix(h.Snips[0].Text, "…") || !strings.HasSuffix(h.Snips[0].Text, "…") ||
		string([]rune(h.Snips[0].Text)[h.Snips[0].Col:h.Snips[0].Col+7]) != "Çalışan" {
		t.Fatalf("long %+v", h.Snips)
	}
	if _, ok := Find(conv("nothing"), "login"); ok {
		t.Error("found what is not there")
	}
}

// Only conversations whose files can hold the text are read; the hits come
// in order.
func TestSearch(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) session.Ref {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return session.Ref{Agent: "claude", ID: name, Path: p}
	}
	refs := []session.Ref{
		write("a", `{"text":"we fixed the LOGIN page"}`),
		write("b", `{"text":"nothing here"}`),
		write("c", `{"text":"login again"}`),
	}
	bodies := map[string]*session.Session{"a": conv("we fixed the LOGIN page"), "b": conv("nothing here"), "c": conv("login again")}
	var loaded []string
	load := func(_ context.Context, r session.Ref) (*session.Session, error) {
		loaded = append(loaded, r.ID)
		return bodies[r.ID], nil
	}
	out := make(chan Hit, 8)
	looked := 0
	Search(context.Background(), refs, "login", load, out, func() { looked++ })
	var ids []string
	for h := range out {
		ids = append(ids, h.Ref.ID)
	}
	if strings.Join(ids, ",") != "a,c" || strings.Join(loaded, ",") != "a,c" || looked != 3 {
		t.Fatalf("hits %v, loaded %v, looked %d", ids, loaded, looked)
	}
	// Text JSON may escape is looked for after parsing.
	loaded = nil
	out = make(chan Hit, 8)
	Search(context.Background(), refs, `"here"`, load, out, nil)
	for range out {
	}
	if len(loaded) != 3 {
		t.Errorf("a quoted query read %v", loaded)
	}
}
