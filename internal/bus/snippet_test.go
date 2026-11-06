package bus

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestSnippetVariants(t *testing.T) {
	for target, file := range SnippetTargets {
		s, err := Snippet(target)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{file, "ajan panes", "ajan read", "ajan send", "ajan wait", "[from <agent> <pane>]", "not from the user"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s snippet lacks %q", target, want)
			}
		}
	}
	if _, err := Snippet("vim"); err == nil {
		t.Fatal("unknown target accepted")
	}
}

// TestNothingWritesAgentInstructionFiles: ajan never edits CLAUDE.md,
// GEMINI.md or AGENTS.md. No source file that names them does file I/O.
func TestNothingWritesAgentInstructionFiles(t *testing.T) {
	root := testutil.ModuleRoot(t)
	names := regexp.MustCompile(`(CLAUDE|GEMINI|AGENTS)\.md`)
	io := regexp.MustCompile(`os\.(WriteFile|Create|OpenFile|Rename|Remove)|paths\.WriteFile|ioutil\.`)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !names.Match(b) {
			return nil
		}
		if io.Match(b) {
			rel, _ := filepath.Rel(root, p)
			t.Errorf("%s mentions an agent instruction file and performs file I/O", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
