package agenthook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestClaude (T76.2): Claude Code's hook events as states; its session on
// SessionStart and UserPromptSubmit.
func TestClaude(t *testing.T) {
	for in, want := range map[string]Report{
		`{"hook_event_name":"UserPromptSubmit","session_id":"s1","transcript_path":"/t/s1.jsonl"}`: {Working, "s1", "/t/s1.jsonl"},
		`{"hook_event_name":"Stop","session_id":"s1"}`:                                             {State: Idle},
		`{"hook_event_name":"SessionStart","session_id":"s2","transcript_path":"/t/s2.jsonl"}`:     {Idle, "s2", "/t/s2.jsonl"},
		`{"hook_event_name":"SessionEnd"}`:                                                         {State: Idle},
		`{"hook_event_name":"Notification","notification_type":"permission_prompt"}`:               {State: Blocked},
		`{"hook_event_name":"Notification","notification_type":"elicitation_dialog"}`:              {State: Blocked},
		`{"hook_event_name":"Notification","notification_type":"idle_prompt"}`:                     {State: Idle},
		`{"hook_event_name":"Notification","message":"Claude needs your permission to use Bash"}`:  {State: Blocked},
		`{"hook_event_name":"Notification","message":"Claude is waiting for your input"}`:          {},
		`{"hook_event_name":"PreToolUse"}`:                                                         {},
	} {
		if got, ok := Claude([]byte(in)); !ok || got != want {
			t.Errorf("%s: %+v %v, want %+v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "nope", `{"x":1}`} {
		if _, ok := Claude([]byte(bad)); ok {
			t.Errorf("%q read as a hook", bad)
		}
	}
}

func TestCodex(t *testing.T) {
	if r, ok := Codex(`{"type":"agent-turn-complete","turn-id":"1"}`); !ok || r.State != Idle {
		t.Errorf("turn complete: %+v %v", r, ok)
	}
	if r, ok := Codex(`{"type":"other"}`); !ok || r.State != "" {
		t.Errorf("other: %+v", r)
	}
	if _, ok := Codex("not json"); ok {
		t.Error("read garbage")
	}
	if a := CodexArgs("/opt/i mux/imux"); len(a) != 2 || a[0] != "-c" || a[1] != `notify=["/opt/i mux/imux","__hook","codex"]` {
		t.Errorf("args %q", a)
	}
	// The user's own notify is left alone.
	dir := t.TempDir()
	if CodexHasNotify(dir) {
		t.Error("notify without a config")
	}
	_ = os.WriteFile(filepath.Join(dir, "config.toml"), []byte("model = \"o3\"\n[tui]\nx = 1\n"), 0o600)
	if CodexHasNotify(dir) {
		t.Error("notify in a config without one")
	}
	_ = os.WriteFile(filepath.Join(dir, "config.toml"), []byte("notify = [\"terminal-notifier\"]\n"), 0o600)
	if !CodexHasNotify(dir) {
		t.Error("the user's notify missed")
	}
}

// TestClaudeHooks (T76.1): every listened event runs the command; none on
// every tool call.
func TestClaudeHooks(t *testing.T) {
	b, _ := json.Marshal(ClaudeHooks("'/x/imux' __hook claude"))
	var h map[string][]struct {
		Hooks []struct{ Type, Command string }
	}
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatal(err)
	}
	for _, ev := range ClaudeEvents {
		if len(h[ev]) != 1 || h[ev][0].Hooks[0].Type != "command" || h[ev][0].Hooks[0].Command != "'/x/imux' __hook claude" {
			t.Errorf("%s: %+v", ev, h[ev])
		}
	}
	if _, ok := h["PreToolUse"]; ok {
		t.Error("a hook on every tool call")
	}
}
