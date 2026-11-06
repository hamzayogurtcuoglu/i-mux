// Package testutil provides isolated homes, fake agent stores and a frozen
// clock for tests. Tests must never read the developer's real ~/.claude,
// ~/.gemini/antigravity-cli, ~/.copilot, ~/.codex or ~/.imux.
package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/clock"
)

// FrozenTime is the default instant used by tests (a Thursday afternoon UTC).
var FrozenTime = time.Date(2026, 11, 5, 14, 32, 0, 0, time.UTC)

// Homes are the temp directories a test runs with.
type Homes struct {
	Root        string // temp root
	Home        string // $HOME
	Imux        string // $IMUX_HOME
	Claude      string // $CLAUDE_CONFIG_DIR
	Antigravity string // ~/.gemini/antigravity-cli
	Copilot     string // $COPILOT_HOME
	Codex       string // $CODEX_HOME
	Project     string // a project directory to use as cwd
}

// Isolate points HOME, IMUX_HOME and every agent home at fresh temp dirs and
// returns them. Agent store directories are created empty.
func Isolate(t testing.TB) *Homes {
	t.Helper()
	root := t.TempDir()
	// Resolve symlinks (macOS /var → /private/var) so that cwd comparisons
	// against os.Getwd() results hold.
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	h := &Homes{
		Root:    root,
		Home:    filepath.Join(root, "home"),
		Project: filepath.Join(root, "home", "proj"),
	}
	h.Imux = filepath.Join(h.Home, ".imux")
	h.Claude = filepath.Join(h.Home, ".claude")
	h.Antigravity = filepath.Join(h.Home, ".gemini", "antigravity-cli")
	h.Copilot = filepath.Join(h.Home, ".copilot")
	h.Codex = filepath.Join(h.Home, ".codex")
	for _, d := range []string{
		h.Home, h.Project,
		filepath.Join(h.Claude, "projects"),
		filepath.Join(h.Antigravity, "brain"),
		filepath.Join(h.Copilot, "session-state"),
		filepath.Join(h.Codex, "sessions"),
	} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", h.Home)
	t.Setenv("IMUX_HOME", h.Imux)
	t.Setenv("CLAUDE_CONFIG_DIR", h.Claude)
	t.Setenv("CODEX_HOME", h.Codex)
	t.Setenv("COPILOT_HOME", h.Copilot)
	t.Setenv("GEMINI_CLI_HOME", "")
	t.Setenv("IMUX_LANG", "en")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h.Home, ".config"))
	return h
}

// Clock returns a clock frozen at FrozenTime.
func Clock() *clock.Fixed { return clock.NewFixed(FrozenTime) }

// WriteFile writes data to path, creating parent directories.
func WriteFile(t testing.TB, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// Touch sets a file's modification time.
func Touch(t testing.TB, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// ModuleRoot returns the directory containing go.mod.
func ModuleRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// AgyTurn is one request and reply of a synthetic Antigravity conversation.
type AgyTurn struct {
	At           time.Time
	User, Reply  string
	Input, Cache int64 // tokens of the reply
	Output       int64
}

// WriteAntigravity writes a synthetic Antigravity CLI conversation in its
// real layout: brain/<id>/.system_generated/logs/transcript_full.jsonl, plus
// one history.jsonl line per request naming cwd (how imux finds the
// directory). The transcript's mtime is set to mtime. It returns the path.
func WriteAntigravity(t testing.TB, h *Homes, id, cwd string, mtime time.Time, turns ...AgyTurn) string {
	t.Helper()
	var tr, hist []byte
	for i, turn := range turns {
		at := turn.At.UTC().Format(time.RFC3339)
		user, _ := json.Marshal(map[string]any{
			"step_index": 2 * i, "source": "USER_EXPLICIT", "type": "USER_INPUT", "status": "DONE", "created_at": at,
			"content": "<USER_REQUEST>\n" + turn.User + "\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nThe current local time is: " + at + ".\n</ADDITIONAL_METADATA>",
		})
		reply, _ := json.Marshal(map[string]any{
			"step_index": 2*i + 1, "source": "MODEL", "type": "PLANNER_RESPONSE", "status": "DONE", "created_at": at,
			"content": turn.Reply, "input_tokens": turn.Input, "cache_read_tokens": turn.Cache, "output_tokens": turn.Output,
		})
		tr = append(append(append(append(tr, user...), '\n'), reply...), '\n')
		h, _ := json.Marshal(map[string]any{"display": turn.User, "timestamp": turn.At.UnixMilli(), "workspace": cwd})
		hist = append(append(hist, h...), '\n')
	}
	p := filepath.Join(h.Antigravity, "brain", id, ".system_generated", "logs", "transcript_full.jsonl")
	WriteFile(t, p, tr)
	Touch(t, p, mtime)
	hp := filepath.Join(h.Antigravity, "history.jsonl")
	f, err := os.OpenFile(hp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(hist); err != nil {
		t.Fatal(err)
	}
	return p
}
