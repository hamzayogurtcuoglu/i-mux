// Package agenthook is what agents i-mux starts tell it through their hooks
// (ARCHITECTURE "Agent hooks", D71): Claude Code's hook events and Codex's
// notify program become a pane's state (working, blocked, idle), and Claude's
// the session it writes.
package agenthook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// States a hook reports (the sidebar's names).
const (
	Working = "working"
	Blocked = "blocked"
	Idle    = "idle"
)

// Report is one hook's news: the state ("" when it tells none) and, for
// Claude, the session it writes.
type Report struct {
	State     string
	SessionID string
	File      string // the session's transcript
}

// ClaudeEvents are the Claude Code hook events i-mux listens to; none runs
// for every tool call, which would make Claude wait each time.
var ClaudeEvents = []string{"SessionStart", "UserPromptSubmit", "Notification", "Stop", "SessionEnd"}

// claudeInput is the JSON Claude Code gives a hook on stdin.
type claudeInput struct {
	Event            string `json:"hook_event_name"`
	SessionID        string `json:"session_id"`
	TranscriptPath   string `json:"transcript_path"`
	NotificationType string `json:"notification_type"`
	Message          string `json:"message"`
}

// Claude reads a Claude Code hook's input; ok is false for input that is
// no hook event.
func Claude(in []byte) (Report, bool) {
	var e claudeInput
	if json.Unmarshal(in, &e) != nil || e.Event == "" {
		return Report{}, false
	}
	r := Report{}
	switch e.Event {
	case "UserPromptSubmit":
		r.State = Working
	case "Stop", "SessionStart", "SessionEnd":
		r.State = Idle
	case "Notification":
		switch {
		case e.NotificationType == "permission_prompt" || e.NotificationType == "elicitation_dialog":
			r.State = Blocked
		case e.NotificationType == "idle_prompt":
			r.State = Idle
		case e.NotificationType == "" && strings.Contains(strings.ToLower(e.Message), "permission"):
			r.State = Blocked // before notification_type: "Claude needs your permission to use Bash"
		}
	}
	if e.Event == "SessionStart" || e.Event == "UserPromptSubmit" {
		r.SessionID, r.File = e.SessionID, e.TranscriptPath
	}
	return r, true
}

// Codex reads the JSON Codex gives its notify program (its last argument):
// a finished turn is idle.
func Codex(arg string) (Report, bool) {
	var e struct {
		Type string `json:"type"`
	}
	if json.Unmarshal([]byte(arg), &e) != nil || e.Type == "" {
		return Report{}, false
	}
	if e.Type == "agent-turn-complete" {
		return Report{State: Idle}, true
	}
	return Report{}, true
}

// ClaudeHooks is the "hooks" object of Claude's settings: each event runs
// command.
func ClaudeHooks(command string) map[string]any {
	h := map[string]any{}
	for _, ev := range ClaudeEvents {
		h[ev] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}}
	}
	return h
}

// CodexHome is Codex's folder: $CODEX_HOME, else ~/.codex.
func CodexHome(home string) string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	return filepath.Join(home, ".codex")
}

// CodexHasNotify reports whether the user's Codex config sets its own
// notify program: i-mux's would replace it, so it is not added then.
func CodexHasNotify(codexHome string) bool {
	b, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		return false
	}
	var m map[string]any
	if _, err := toml.Decode(string(b), &m); err != nil {
		return true // unreadable: leave it alone
	}
	_, ok := m["notify"]
	return ok
}

// CodexArgs are the arguments that make Codex run `self __hook codex` when
// a turn ends.
func CodexArgs(self string) []string {
	b, _ := json.Marshal([]string{self, "__hook", "codex"})
	return []string{"-c", "notify=" + string(b)}
}
