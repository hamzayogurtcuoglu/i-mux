//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/gemini"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// synthetic writes a synthetic session of agent for the project directory,
// with the given first request and modification time, and returns its id.
func (h *H) synthetic(agent string, n int, text string, mtime time.Time) string {
	h.t.Helper()
	id := fmt.Sprintf("%08x-5e55-4000-8000-%012d", n, n)
	ts := mtime.UTC().Format(time.RFC3339)
	cwd := h.Homes.Project
	var p string
	var body string
	switch agent {
	case "claude":
		p = filepath.Join(h.Homes.Claude, "projects", claude.EncodeCWD(cwd), id+".jsonl")
		body = fmt.Sprintf(`{"type":"user","sessionId":%q,"cwd":%q,"timestamp":%q,"uuid":"u","message":{"role":"user","content":%q}}`+"\n"+
			`{"type":"assistant","sessionId":%q,"timestamp":%q,"uuid":"a","message":{"id":"m-%s","role":"assistant","model":"m","content":[{"type":"text","text":"ok"}]}}`+"\n",
			id, cwd, ts, text, id, ts, id)
	case "codex":
		p = filepath.Join(h.Homes.Codex, "sessions", mtime.UTC().Format("2006/01/02"), "rollout-"+mtime.UTC().Format("2006-01-02T15-04-05")+"-"+id+".jsonl")
		body = fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"id":%q,"cwd":%q}}`+"\n"+
			`{"timestamp":%q,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}}`+"\n",
			ts, id, cwd, ts, text)
	case "gemini":
		p = filepath.Join(h.Homes.Gemini, "tmp", gemini.ProjectHash(cwd), "chats", "session-"+mtime.UTC().Format("2006-01-02T15-04")+"-"+id[:8]+".json")
		body = fmt.Sprintf(`{"sessionId":%q,"startTime":%q,"lastUpdated":%q,"messages":[{"id":"u","timestamp":%q,"type":"user","content":%q}]}`,
			id, ts, ts, ts, text)
	case "copilot":
		dir := filepath.Join(h.Homes.Copilot, "session-state", id)
		testutil.WriteFile(h.t, filepath.Join(dir, "workspace.yaml"), []byte(fmt.Sprintf("id: %s\ncwd: %s\ncreated_at: %s\n", id, cwd, ts)))
		p = filepath.Join(dir, "events.jsonl")
		body = fmt.Sprintf(`{"type":"user.message","timestamp":%q,"data":{"content":%q}}`+"\n", ts, text)
	default:
		h.t.Fatalf("unknown agent %s", agent)
	}
	testutil.WriteFile(h.t, p, []byte(body))
	testutil.Touch(h.t, p, mtime)
	if agent == "copilot" {
		_ = os.Chtimes(filepath.Dir(p), mtime, mtime)
	}
	return id
}
