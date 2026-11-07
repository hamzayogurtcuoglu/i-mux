package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseClaudeStatusline(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	in := `{"cwd":"/p","workspace":{"current_dir":"/w"},"rate_limits":{"five_hour":{"used_percentage":72.5,"resets_at":1800003600},"seven_day":{"used_percentage":18,"resets_at":"2027-01-20T09:00:00Z"}}}`
	s, dir := ParseClaudeStatusline([]byte(in), now)
	if s == nil || dir != "/w" || s.FiveHour.UsedPercent != 72.5 || !s.FiveHour.ResetsAt.Equal(time.Unix(1_800_003_600, 0)) ||
		s.Weekly.UsedPercent != 18 || s.Weekly.ResetsAt.Year() != 2027 {
		t.Fatalf("%+v %q", s, dir)
	}
	// Milliseconds; one window only; none at all; junk.
	s, _ = ParseClaudeStatusline([]byte(`{"rate_limits":{"five_hour":{"used_percentage":3,"resets_at":1800003600000}}}`), now)
	if s == nil || s.Weekly != nil || !s.FiveHour.ResetsAt.Equal(time.Unix(1_800_003_600, 0)) {
		t.Fatalf("ms %+v", s)
	}
	if s, dir := ParseClaudeStatusline([]byte(`{"cwd":"/p"}`), now); s != nil || dir != "/p" {
		t.Fatalf("no limits %+v", s)
	}
	if s, _ := ParseClaudeStatusline([]byte(`not json`), now); s != nil {
		t.Fatal("junk")
	}

	p := filepath.Join(t.TempDir(), "usage", "claude.json")
	full, _ := ParseClaudeStatusline([]byte(in), now)
	if err := Save(p, full); err != nil {
		t.Fatal(err)
	}
	got := Load(p)
	if got == nil || got.FiveHour.UsedPercent != 72.5 {
		t.Fatalf("load %+v", got)
	}
	// After the 5-hour window reset, its percentage no longer holds.
	c := got.Current(now.Add(2 * time.Hour))
	if c.FiveHour.UsedPercent != 0 || c.Weekly.UsedPercent != 18 || got.FiveHour.UsedPercent != 72.5 {
		t.Fatalf("current %+v", c)
	}
	if Load(filepath.Join(t.TempDir(), "x")) != nil {
		t.Fatal("missing")
	}
}

func TestUserStatusLine(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	w := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(s), 0o600)
	}
	if UserStatusLine(home, dir) != "" {
		t.Fatal("none")
	}
	w(filepath.Join(home, ".claude/settings.json"), `{"statusLine":{"type":"command","command":"~/bin/line.sh"}}`)
	if UserStatusLine(home, dir) != "~/bin/line.sh" {
		t.Fatal("user")
	}
	w(filepath.Join(dir, ".claude/settings.json"), `{"statusLine":{"type":"command","command":"proj.sh"}}`)
	if UserStatusLine(home, dir) != "proj.sh" {
		t.Fatal("project wins")
	}
	w(filepath.Join(dir, ".claude/settings.local.json"), `{"statusLine":{"type":"command","command":"imux __statusline claude"}}`)
	if UserStatusLine(home, dir) != "proj.sh" {
		t.Fatal("imux's own line is skipped")
	}
}
