package approve

import (
	"strings"
	"testing"
)

// Synthetic screens of each CLI's permission prompt, as its TUI shows them at
// the bottom of the pane (D112): the choices, then only blank lines, borders
// and key hints, the question above. No real session text is used.
func TestFindKey(t *testing.T) {
	for _, c := range []struct {
		name   string
		agent  string
		screen string
		key    string
	}{
		{
			"claude command",
			"claude",
			"● Bash(npm test)\n  $ npm test\n  This command requires approval\n\n" +
				"Do you want to proceed?\n" +
				"❯ 1. Yes\n" +
				"  2. Yes, and don't ask again for: npm test *\n" +
				"  3. Yes, and switch to auto mode\n" +
				"  4. No\n" +
				"  esc to cancel · tab to amend\n",
			"1",
		},
		{
			"claude edit",
			"claude",
			"Do you want to make this edit to main.go?\n" +
				"❯ 1. Yes\n" +
				"  2. Yes, and allow all edits during this session\n" +
				"  3. No, and tell Claude what to do differently (esc)\n",
			"1",
		},
		{
			"claude mcp permissions",
			"claude",
			"Claude requested permissions to use WebFetch, but you haven't granted it yet.\n" +
				"❯ 1. Yes\n" +
				"  2. Yes, and don't ask again for this domain\n" +
				"  3. No, and tell Claude what to do differently (esc)\n",
			"1",
		},
		{
			"copilot command",
			"copilot",
			"Do you want to run this command?\n$ npm test\n\n" +
				"❯ 1. Yes, proceed\n" +
				"  2. Yes, and approve shell for the rest of the running session\n" +
				"  3. No, and tell Copilot what to do differently (Esc)\n",
			"1",
		},
		{
			"copilot organization policy",
			"copilot",
			"Organization policy requires confirmation.\n" +
				"Do you want to run this command?\n" +
				"❯ 1. Yes\n" +
				"  2. Yes, and approve shell for the rest of the running session\n" +
				"  3. No, and tell Copilot what to do differently (Esc)\n",
			"1",
		},
		{
			// As Copilot really shows it under an organization policy: the
			// question and the choices inside a box, the hint of its keys
			// below them, then the box's bottom edge.
			"copilot organization policy in a box",
			"copilot",
			"Organization policy requires confirmation.\n" +
				"┌──────────────────────────────────────────────────────────────┐\n" +
				"│ Show the recent commits                                      │\n" +
				"│ git log --oneline -15                                         │\n" +
				"│                                                              │\n" +
				"│ Do you want to run this command?                             │\n" +
				"│                                                              │\n" +
				"│ ❯ 1. Yes                                                     │\n" +
				"│   2. No, and tell Copilot what to do differently (Esc to stop) │\n" +
				"│                                                              │\n" +
				"│ ↑/↓ to navigate · enter to select · esc to cancel            │\n" +
				"└──────────────────────────────────────────────────────────────┘\n",
			"1",
		},
		{
			"codex command with its keys",
			"codex",
			"Would you like to run the following command?\n\n$ rm -rf build\n\n" +
				"1. Yes, proceed (y)\n" +
				"2. Yes, and don't ask again for commands that start with `rm` (p)\n" +
				"3. No, and tell Codex what to do differently (esc)\n" +
				"\n" +
				"enter to confirm · esc to cancel\n",
			"y",
		},
		{
			"codex numbered, no keys shown",
			"codex",
			"Would you like to run the following command?\n$ rm -rf build\n" +
				"› 1. Yes, proceed\n" +
				"  2. Yes, and don't ask again for this command in this session\n" +
				"  3. No, and tell Codex what to do differently\n",
			"1",
		},
		{
			"codex edits",
			"codex",
			"Would you like to make the following edits?\n" +
				"1. Yes, proceed\n" +
				"2. Yes, and don't ask again for these files\n" +
				"3. No, and tell Codex what to do differently (esc)\n",
			"1",
		},
		{
			"antigravity file creation",
			"antigravity",
			"Allow creation of this file?\n" +
				"> 1. Yes, allow creation\n" +
				"  2. No, deny creation\n",
			"y",
		},
		{
			"antigravity command",
			"antigravity",
			"Do you want to proceed?\n" +
				"❯ 1. Yes, run the command\n" +
				"  2. No, and tell me what to do differently\n",
			"y",
		},
		{
			"opencode buttons in a row",
			"opencode",
			"△ Permission required\n  bash command: rm -rf build\n\n" +
				" Allow once  Allow always  Reject   ctrl+g fullscreen  ⇆ select  enter confirm\n",
			"Enter",
		},
		{
			"opencode buttons in a column",
			"opencode",
			"△ Permission required\n" +
				" Allow once\n" +
				" Allow always\n" +
				" Reject\n" +
				" enter confirm\n",
			"Enter",
		},
		{
			"opencode without always",
			"opencode",
			"△ Permission required\n" +
				" Allow once\n" +
				" Reject\n" +
				" ⇆ select  enter confirm\n",
			"Enter",
		},
	} {
		q := Find(c.screen)
		if q == nil {
			t.Errorf("%s: no prompt found in:\n%s", c.name, c.screen)
			continue
		}
		key, ok := Key(c.agent, q)
		if !ok || key != c.key {
			t.Errorf("%s: key %q ok %v, want %q", c.name, key, ok, c.key)
		}
	}
}

// Screens that are not a permission prompt, or a prompt i-mux leaves to the
// user: nothing is answered.
func TestFindKeyNotAnswered(t *testing.T) {
	for _, c := range []struct {
		name   string
		agent  string
		screen string
	}{
		{
			"empty",
			"claude",
			"",
		},
		{
			"plain output",
			"claude",
			"Tests: 14 passed\n> \n",
		},
		{
			"a transcript's numbered list above the input box",
			"claude",
			"I updated the tests. Do you want me to also update the docs?\n" +
				"1. Yes\n2. No\n" +
				"╭──────────────╮\n│ >           │\n╰──────────────╯\n" +
				"? for shortcuts\n",
		},
		{
			"a question without choices",
			"claude",
			"Do you want to make this edit to main.go?\n> \n",
		},
		{
			"a plan whose first option is not the plain yes",
			"claude",
			"Ready to code?\n" +
				"❯ 1. Yes, and auto-accept edits\n" +
				"  2. Yes, and manually approve edits\n" +
				"  3. No, keep planning\n",
		},
		{
			"choices that are not a yes/no",
			"claude",
			"Which file did you mean?\n" +
				"❯ 1. main.go\n" +
				"  2. main_test.go\n",
		},
		{
			"a menu of the CLI's own",
			"codex",
			"/model\n" +
				"› 1. o4-mini\n" +
				"  2. gpt-5\n" +
				"enter to select\n",
		},
		{
			"the trust question",
			"claude",
			"Do you trust the files in this folder?\n" +
				"❯ 1. Yes, I trust\n" +
				"  2. No, exit\n",
		},
		{
			"an unnumbered list",
			"codex",
			"Would you like to run the following command?\n" +
				"Yes, proceed\nNo\n",
		},
		{
			"a line of work below the choices",
			"claude",
			"Do you want to proceed?\n" +
				"❯ 1. Yes\n" +
				"  2. No\n" +
				"⠋ Reticulating splines… (2s)\n",
		},
		{
			"the input hint below the choices",
			"claude",
			"Do you want me to also update the docs?\n" +
				"❯ 1. Yes\n" +
				"  2. No\n" +
				"⏵⏵ accept edits on (shift+tab to cycle)\n",
		},
		{
			"opencode without the once button",
			"opencode",
			"△ Permission required\n" +
				" Allow always\n" +
				" Reject\n" +
				" enter confirm\n",
		},
		{
			"opencode with something else on the row",
			"opencode",
			"△ Permission required\n" +
				" Allow once  Reject   3 files changed\n",
		},
		{
			"an agent i-mux does not know",
			"aider",
			"Do you want to proceed?\n❯ 1. Yes\n  2. No\n",
		},
	} {
		q := Find(c.screen)
		if q == nil {
			continue
		}
		if key, ok := Key(c.agent, q); ok {
			t.Errorf("%s: answered %q", c.name, key)
		}
	}
}

// The same question asked twice is one prompt while it stays, a new one once
// it left the screen: the hash tells them apart (the watcher answers each
// once, D112).
func TestHash(t *testing.T) {
	a := Find("Do you want to proceed?\n❯ 1. Yes\n  2. No\n")
	b := Find("Do you want to proceed?\n❯ 1. Yes\n  2. No\n")
	c := Find("Do you want to run this command?\n❯ 1. Yes\n  2. No\n")
	if a == nil || b == nil || c == nil {
		t.Fatal("no prompt")
	}
	if a.Hash() != b.Hash() {
		t.Error("the same prompt has two hashes")
	}
	if a.Hash() == c.Hash() {
		t.Error("two prompts share a hash")
	}
}

// A wrapped label is still one choice, and the choices still end the screen.
func TestWrappedLabels(t *testing.T) {
	screen := "Would you like to run the following command?\n" +
		"1. Yes, proceed\n" +
		"2. Yes, and don't ask again for commands that start with `rm`\n" +
		"   in /home/test/project\n" +
		"3. No, and tell Codex what to do differently (esc)\n"
	q := Find(screen)
	if q == nil || len(q.Options) != 3 {
		t.Fatalf("wrapped label: %+v", q)
	}
	if key, ok := Key("codex", q); !ok || key != "1" {
		t.Errorf("codex: key %q ok %v", key, ok)
	}
	if !strings.Contains(q.Options[1].Label, "in /home/test/project") {
		t.Errorf("wrapped label: %q", q.Options[1].Label)
	}
}

// OpenCode's "Allow once" is the button the colors show as highlighted, and
// only that one is answered with Enter (D112): a capture with SGR
// sequences, as `capture-pane -e` prints them.
func TestHighlightedOnce(t *testing.T) {
	esc := "\x1b["
	for _, c := range []struct {
		name    string
		capture string
		want    bool
	}{
		{
			"once highlighted, 256 colors",
			"△ Permission required\n" +
				esc + "48;5;71m Allow once " + esc + "48;5;24m Allow always " + esc + "48;5;24m Reject " + esc + "49m  enter confirm\n",
			true,
		},
		{
			"once highlighted, true color",
			esc + "48;2;10;30;10m Allow once " + esc + "48;2;40;40;60m Allow always " + esc + "48;2;40;40;60m Reject " + esc + "0m\n",
			true,
		},
		{
			"once highlighted among two buttons: cannot tell",
			esc + "48;5;71m Allow once " + esc + "48;5;24m Reject " + esc + "49m\n",
			false,
		},
		{
			"reject highlighted",
			esc + "48;5;24m Allow once " + esc + "48;5;24m Allow always " + esc + "48;5;71m Reject " + esc + "49m\n",
			false,
		},
		{
			"always highlighted",
			esc + "48;5;24m Allow once " + esc + "48;5;71m Allow always " + esc + "48;5;24m Reject " + esc + "49m\n",
			false,
		},
		{
			"all one background: cannot tell",
			esc + "48;5;24m Allow once  Allow always  Reject " + esc + "49m\n",
			false,
		},
		{
			"the direct-mode footer: unselected buttons are transparent",
			"△ Permission required\n" +
				esc + "48;5;71m Allow once " + esc + "m Allow always " + esc + "m Reject permission " + esc + "49m  enter confirm\n",
			true,
		},
		{
			"the direct-mode footer with once unselected",
			"△ Permission required\n" +
				esc + "m Allow once " + esc + "48;5;71m Allow always " + esc + "m Reject permission " + esc + "49m\n",
			false,
		},
		{
			"no colors",
			" Allow once  Allow always  Reject \n",
			false,
		},
	} {
		if got := HighlightedOnce(c.capture); got != c.want {
			t.Errorf("%s: HighlightedOnce = %v, want %v", c.name, got, c.want)
		}
	}
}
