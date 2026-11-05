package clean

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTextStripsWrappers(t *testing.T) {
	in := "<system-reminder>secret context\nmore</system-reminder>\nfix the login bug\n\n\n\n<ide_selection>x</ide_selection><ide_opened_file attr=\"1\">y</ide_opened_file>please<system-note>n</system-note>"
	got := Text(in, ClaudeRules)
	if got != "fix the login bug\n\nplease" {
		t.Fatalf("got %q", got)
	}
}

func TestLocalCommandStdoutDropped(t *testing.T) {
	if got := Text("<local-command-stdout>Total cost: $0.12</local-command-stdout>", ClaudeRules); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestTaskNotification(t *testing.T) {
	in := "<task-notification>\n<task-id>b1</task-id>\n<status>completed</status>\n<summary>Background command \"npm test\" finished\nwith exit 0</summary>\n</task-notification>"
	got := Text(in, ClaudeRules)
	if got != `[background task: Background command "npm test" finished with exit 0]` {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatal("notice must be one line")
	}
	got = Text("<task-notification>done</task-notification>", ClaudeRules)
	if got != "[background task: done]" {
		t.Fatalf("got %q", got)
	}
}

func TestSlashCommand(t *testing.T) {
	cmd, ok := SlashCommand("<command-message>review is running…</command-message>\n<command-name>/review</command-name>\n<command-args>PR 12</command-args>")
	if !ok || cmd != "/review PR 12" {
		t.Fatalf("%q %v", cmd, ok)
	}
	cmd, ok = SlashCommand("<command-name>compact</command-name><command-args></command-args>")
	if !ok || cmd != "/compact" {
		t.Fatalf("%q %v", cmd, ok)
	}
	if _, ok := SlashCommand("hello"); ok {
		t.Fatal("not a command")
	}
}

func TestTruncateRuneSafe(t *testing.T) {
	s := strings.Repeat("ğüşıöç🎉", 500) // 3500 runes, multi-byte
	for _, max := range []int{1, 2, 10, 37, 100, 999, 3499} {
		got := Truncate(s, max)
		if !utf8.ValidString(got) {
			t.Fatalf("max %d: invalid UTF-8", max)
		}
		if n := Runes(got); n > max {
			t.Fatalf("max %d: got %d runes", max, n)
		}
	}
	got := Truncate(s, 200)
	if !strings.Contains(got, "[… 3,500 chars omitted …]") {
		t.Fatalf("marker missing: %q", got)
	}
	if !strings.HasPrefix(got, "ğüş") || !strings.HasSuffix(got, "ç🎉") {
		t.Fatalf("head/tail lost: %q", got)
	}
	if Truncate("short", 10) != "short" {
		t.Fatal("short text changed")
	}
}

func TestTruncateHeadKeepsNewest(t *testing.T) {
	s := "old old old\n" + strings.Repeat("x", 100) + "\nnewest🎉"
	got := TruncateHead(s, 60)
	if Runes(got) > 60 || !strings.HasSuffix(got, "newest🎉") || !strings.HasPrefix(got, "[…") {
		t.Fatalf("got %q", got)
	}
}

func TestOneLine(t *testing.T) {
	if got := OneLine("  fix the\n login   redirect bug please now ", 20); got != "fix the login redir…" {
		t.Fatalf("got %q", got)
	}
	if got := OneLine("çok güzel", 40); got != "çok güzel" {
		t.Fatalf("got %q", got)
	}
}

func TestThousands(t *testing.T) {
	for in, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567", -1234: "-1,234"} {
		if got := Thousands(in); got != want {
			t.Errorf("%d: %q", in, got)
		}
	}
}

func FuzzTruncate(f *testing.F) {
	f.Add("ğüşıöç🎉 hello", 5)
	f.Add("", 0)
	f.Fuzz(func(t *testing.T, s string, max int) {
		if max < 0 || max > 1<<16 {
			return
		}
		got := Truncate(s, max)
		if utf8.ValidString(s) && !utf8.ValidString(got) {
			t.Fatal("invalid UTF-8")
		}
		if Runes(got) > max && Runes(s) > max {
			t.Fatalf("too long: %d > %d", Runes(got), max)
		}
		_ = OneLine(s, max)
		_ = TruncateHead(s, max)
		_ = Text(s, ClaudeRules)
	})
}
