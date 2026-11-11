//go:build windows

package tmux

import (
	"reflect"
	"strings"
	"testing"
)

func TestGlobalizeFormat(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "",
		"no formats":             "no formats",
		"#{pane_id}":             "#{pane_id}~#{session_name}",
		"#D":                     "#{pane_id}~#{session_name}",
		"#{window_id}":           "#{window_id}~#{session_name}",
		"#{client_name}":         "#{client_name}~#{client_session}",
		"#{pane_index}":          "#{pane_index}",
		"#{session_name}":        "#{session_name}",
		"--pane #{pane_id} -c x": "--pane #{pane_id}~#{session_name} -c x",
		"#{pane_id}\t#{window_id}\t#{client_name}": "#{pane_id}~#{session_name}\t#{window_id}~#{session_name}\t#{client_name}~#{client_session}",
	} {
		if got := GlobalizeFormat(in); got != want {
			t.Errorf("GlobalizeFormat(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitID(t *testing.T) {
	for _, c := range []struct{ in, local, session string }{
		{"%3~proj-1a2b", "%3", "proj-1a2b"},
		{"@2~proj-1a2b", "@2", "proj-1a2b"},
		{"/dev/pts/1~s", "/dev/pts/1", "s"},
		{"%3", "%3", ""},
		{"", "", ""},
		{"~s", "~s", ""}, // no id before the session
		{"a~b~c", "a~b", "c"},
	} {
		if l, s := SplitID(c.in); l != c.local || s != c.session {
			t.Errorf("SplitID(%q) = %q, %q; want %q, %q", c.in, l, s, c.local, c.session)
		}
	}
}

func TestLocalTarget(t *testing.T) {
	for _, c := range []struct{ in, local, session string }{
		{"%3~proj", "%3", "proj"},
		{"@2~proj", "@2", "proj"},
		{"client-1~proj", "client-1", "proj"},
		{"=proj:", "=proj:", "proj"},
		{"proj:1.2", "proj:1.2", "proj"},
		{"proj:@2~proj", "proj:@2", "proj"},
		{"%3", "%3", ""},
		{"@2", "@2", ""},
		{"=", "=", ""},
		{"proj", "proj", ""},
		{"", "", ""},
	} {
		if l, s := localTarget(c.in); l != c.local || s != c.session {
			t.Errorf("localTarget(%q) = %q, %q; want %q, %q", c.in, l, s, c.local, c.session)
		}
	}
}

func TestArgvForm(t *testing.T) {
	for _, c := range []struct{ in, want Cmd }{
		// A program and arguments become psmux's argv form.
		{Cmd{"split-window", "-h", "-t", "%3", "-l", "30", "prog", "a b"}, Cmd{"split-window", "-h", "-t", "%3", "-l", "30", "--", "prog", "a b"}},
		{Cmd{"new-session", "-d", "-s", "proj", "-c", `C:\p`, "prog", "-x"}, Cmd{"new-session", "-d", "-s", "proj", "-c", `C:\p`, "--", "prog", "-x"}},
		{Cmd{"neww", "-P", "-F", "#{pane_id}", "prog", "x"}, Cmd{"neww", "-P", "-F", "#{pane_id}", "--", "prog", "x"}},
		{Cmd{"respawn-pane", "-k", "-t", "%1", "prog", "x"}, Cmd{"respawn-pane", "-k", "-t", "%1", "--", "prog", "x"}},
		// One command line for the shell, no program, or already argv form: as it is.
		{Cmd{"split-window", "-t", "%3", "prog a b"}, Cmd{"split-window", "-t", "%3", "prog a b"}},
		{Cmd{"split-window", "-h"}, Cmd{"split-window", "-h"}},
		{Cmd{"split-window", "--", "prog", "x"}, Cmd{"split-window", "--", "prog", "x"}},
		// Other commands are never rewritten.
		{Cmd{"send-keys", "-t", "%3", "a", "b"}, Cmd{"send-keys", "-t", "%3", "a", "b"}},
	} {
		in := append(Cmd(nil), c.in...)
		if got := argvForm(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("argvForm(%q) = %q, want %q", in, got, c.want)
		}
		if !reflect.DeepEqual(c.in, in) {
			t.Errorf("argvForm changed its input %q to %q", in, c.in)
		}
	}
}

func TestHasFlag(t *testing.T) {
	for _, c := range []struct {
		cmd  Cmd
		f    byte
		want bool
	}{
		{Cmd{"set", "-g", "@x", "1"}, 'g', true},
		{Cmd{"set", "-gq", "@x", "1"}, 'q', true},
		{Cmd{"set", "-gq", "@x", "1"}, 'u', false},
		{Cmd{"set", "-t", "%3", "-p", "@x", "1"}, 'p', true},
		{Cmd{"set", "-t", "%3", "@x", "1"}, 't', true},
		// A flag's value is not a flag; flags end at the first other word or --.
		{Cmd{"set", "-t", "-gx", "@x", "1"}, 'g', false},
		{Cmd{"set", "@x", "-g"}, 'g', false},
		{Cmd{"send-keys", "--", "-a"}, 'a', false},
		{Cmd{"set"}, 'g', false},
	} {
		if got := hasFlag(c.cmd, c.f); got != c.want {
			t.Errorf("hasFlag(%q, %c) = %v, want %v", c.cmd, c.f, got, c.want)
		}
	}
}

func TestGlobalKey(t *testing.T) {
	for _, c := range []struct {
		cmd  Cmd
		want string
	}{
		{Cmd{"set", "-g", "status", "on"}, "g status"},
		{Cmd{"set", "-s", "escape-time", "10"}, "s escape-time"},
		{Cmd{"setw", "-g", "mode-keys", "vi"}, "g mode-keys"},
		{Cmd{"set", "-ag", "terminal-features", ",x"}, ""}, // appends are not replayed
		{Cmd{"set", "-g"}, ""},
	} {
		if got := globalKey(c.cmd); got != c.want {
			t.Errorf("globalKey(%q) = %q, want %q", c.cmd, got, c.want)
		}
	}
	// One option has one key whatever other flags it is set with, so an
	// unset (-u) removes what a set wrote.
	set, quiet, unset := globalKey(Cmd{"set", "-g", "@imux_status", "x"}), globalKey(Cmd{"set", "-gq", "@imux_status", ""}), globalKey(Cmd{"set", "-gu", "@imux_status"})
	if set == "" || set != quiet || set != unset {
		t.Errorf("keys differ: %q %q %q", set, quiet, unset)
	}
	if globalKey(Cmd{"set", "-g", "a", "1"}) == globalKey(Cmd{"set", "-s", "a", "1"}) {
		t.Error("-g and -s options share a key")
	}
}

func TestLabelPanes(t *testing.T) {
	cmds := []Cmd{
		{"set-option", "-p", "-t", "%3", "@imux_label", "Claude"},
		{"set", "-pq", "-t", "%4", "@imux_flag", "!"},
		{"set", "-p", "-t", "%5", "@imux_name", "api"},
		{"set", "-p", "-t", "%6", "@imux_agent", "claude"}, // not shown on the border
		{"set", "-t", "%7", "@imux_label", "x"},            // not a pane option
		{"set", "-p", "@imux_label", "x"},                  // no target
		{"display-message", "-p", "-t", "%8", "@imux_label"},
	}
	if got, want := labelPanes(cmds), []string{"%3", "%4", "%5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("labelPanes = %q, want %q", got, want)
	}
	if got := labelPanes(nil); len(got) != 0 {
		t.Errorf("labelPanes(nil) = %q", got)
	}
}

func TestPercentResize(t *testing.T) {
	for _, c := range []struct {
		cmd  Cmd
		w, h string
		want Cmd
	}{
		// cells / (size-1), rounded, as a percentage
		{Cmd{"resize-pane", "-t", "%3", "-x", "40"}, "81", "24", Cmd{"resize-pane", "-t", "%3", "-x", "50"}},
		{Cmd{"resize-pane", "-t", "%3", "-y", "6"}, "81", "25", Cmd{"resize-pane", "-t", "%3", "-y", "25"}},
		{Cmd{"resize-pane", "-t", "%3", "-x", "20", "-y", "12"}, "201", "49", Cmd{"resize-pane", "-t", "%3", "-x", "10", "-y", "25"}},
		// kept within 1..99
		{Cmd{"resize-pane", "-x", "0"}, "80", "24", Cmd{"resize-pane", "-x", "1"}},
		{Cmd{"resize-pane", "-x", "500"}, "80", "24", Cmd{"resize-pane", "-x", "99"}},
		// not numbers, or no usable size: as it is
		{Cmd{"resize-pane", "-x", "30%"}, "80", "24", Cmd{"resize-pane", "-x", "30%"}},
		{Cmd{"resize-pane", "-x", "30"}, "", "24", Cmd{"resize-pane", "-x", "30"}},
		{Cmd{"resize-pane", "-x", "30"}, "1", "24", Cmd{"resize-pane", "-x", "30"}},
		{Cmd{"resize-pane", "-Z", "-t", "%3"}, "80", "24", Cmd{"resize-pane", "-Z", "-t", "%3"}},
	} {
		in := append(Cmd(nil), c.cmd...)
		if got := percentResize(c.cmd, c.w, c.h); !reflect.DeepEqual(got, c.want) {
			t.Errorf("percentResize(%q, %s, %s) = %q, want %q", in, c.w, c.h, got, c.want)
		}
		if !reflect.DeepEqual(c.cmd, in) {
			t.Errorf("percentResize changed its input %q to %q", in, c.cmd)
		}
	}
}

func TestPsmuxMenu(t *testing.T) {
	got := psmuxMenu(Cmd{"display-menu", "-c", "client-1", "-t", "%3", "-T", "Sessions ## proj", "-x", "C", "-y", "10",
		"--",
		"Claude ## 1", "1", "run-shell x",
		"",
		"-Disabled ## item", "", "",
		"Last", "l", "run-shell y"})
	want := Cmd{"display-menu", "-T", "Sessions # proj", "-y", "10",
		"Claude # 1", "1", "run-shell x",
		"",
		"Disabled # item", " ", "",
		"Last", "l", "run-shell y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("psmuxMenu:\n got %q\nwant %q", got, want)
	}
	// Without --, items start at the first word that is not a flag.
	got = psmuxMenu(Cmd{"menu", "-x", "5", "A", "a", "cmd"})
	if want := (Cmd{"menu", "-x", "5", "A", "a", "cmd"}); !reflect.DeepEqual(got, want) {
		t.Errorf("psmuxMenu: %q, want %q", got, want)
	}
}

func TestPsmuxPopup(t *testing.T) {
	got := psmuxPopup(Cmd{"display-popup", "-c", "client-1", "-t", "%3", "-w", "80%", "-h", "60%", "-d", `C:\p`, "-T", "title", "-EE", "--", "prog", "-x"})
	want := Cmd{"display-popup", "-w", "80%", "-h", "60%", "-d", `C:\p`, "-E", "prog", "-x"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("psmuxPopup:\n got %q\nwant %q", got, want)
	}
	got = psmuxPopup(Cmd{"popup", "-E", "-w", "50", "prog arg"})
	if want := (Cmd{"popup", "-E", "-w", "50", "prog arg"}); !reflect.DeepEqual(got, want) {
		t.Errorf("psmuxPopup: %q, want %q", got, want)
	}
}

// expand stands in for psmux expanding a -F format for a new pane %9 in
// session s, at window 1, pane 0.
func expand(format string) string {
	return strings.NewReplacer("#{pane_id}", "%9", "#{session_name}", "s", "#{window_index}", "1", "#{pane_index}", "0").Replace(format)
}

// formatOf is the -F value of a command.
func formatOf(t *testing.T, cmd Cmd) string {
	t.Helper()
	i := indexOf(cmd, "-F")
	if i < 0 || i+1 >= len(cmd) {
		t.Fatalf("no -F in %q", cmd)
	}
	return cmd[i+1]
}

// TestNewPaneID: a creating command is made to print the new pane's id, and
// takeNewPane gives that id and the output the caller asked for (none
// without -P; tmux's own -P line; the caller's -F format).
func TestNewPaneID(t *testing.T) {
	for _, c := range []struct {
		name string
		cmd  Cmd
		out  string // what the caller sees
	}{
		{"silent", Cmd{"split-window", "-h", "-t", "%3"}, ""},
		{"-P", Cmd{"split-window", "-P", "-t", "%3"}, "s:1.0\n"},
		{"-F", Cmd{"new-window", "-P", "-F", "#{pane_id}~#{session_name}\tx", "-t", "s:"}, "%9~s\tx\n"},
	} {
		follower := Cmd{"select-pane", "-T", "t"}
		in := []Cmd{append(Cmd(nil), c.cmd...), follower}
		got := askNewPane(in)
		if len(got) != 2 || !reflect.DeepEqual(got[1], follower) {
			t.Fatalf("%s: followers changed: %q", c.name, got)
		}
		if got[0][0] != c.cmd[0] || indexOf(got[0], "-P") < 0 {
			t.Fatalf("%s: %q does not print", c.name, got[0])
		}
		if !reflect.DeepEqual(in[0], c.cmd) {
			t.Fatalf("%s: input changed to %q", c.name, in[0])
		}
		for _, nl := range []string{"\n", "\r\n"} {
			id, rest := takeNewPane(expand(formatOf(t, got[0])) + nl)
			if id != "%9~s" || rest != strings.ReplaceAll(c.out, "\n", nl) {
				t.Errorf("%s: takeNewPane = %q, %q; want %%9~s, %q", c.name, id, rest, c.out)
			}
		}
	}
	if id, rest := takeNewPane("plain output\n"); id != "" || rest != "plain output\n" {
		t.Errorf("takeNewPane without an id = %q, %q", id, rest)
	}
}

func TestPlan(t *testing.T) {
	gs := plan([]Cmd{
		{"display-message", "-p", "-t", "%3~s1", "#{pane_id}"},
		{"send-keys", "-t", "%3~s1", "x"},
		{"send-keys", "-t", "%4~s2", "y"},
		{"set", "-g", "@a", "1"},
		{"list-clients", "-F", "#{client_name}"},
		{"split-window", "-t", "%3~s1", "prog", "a"},
		{"select-pane", "-T", "new"},
		{"new-session", "-d", "-s", "s3"},
		{"display-menu", "-c", "c1~s2", "-t", "%4~s2", "-x", "C", "--", "A", "a", "run-shell x"},
		{"kill-server"},
		{},
	})
	type g struct {
		route                      string
		broadcast, gather, creates bool
		cmds                       []Cmd
	}
	var got []g
	for _, x := range gs {
		got = append(got, g{x.route, x.broadcast, x.gather, x.creates, x.cmds})
	}
	want := []g{
		// Commands for one session go together, with their bare ids, and
		// the formats name the session.
		{route: "s1", cmds: []Cmd{
			{"display-message", "-p", "-t", "%3", "#{pane_id}~#{session_name}"},
			{"send-keys", "-t", "%3", "x"},
		}},
		{route: "s2", cmds: []Cmd{{"send-keys", "-t", "%4", "y"}}},
		// Global settings go to every server; list-clients asks every one.
		{broadcast: true, cmds: []Cmd{{"set", "-g", "@a", "1"}}},
		{gather: true, cmds: []Cmd{{"list-clients", "-F", "#{client_name}~#{client_session}"}}},
		// A creating command is a group of its own, in argv form; the
		// untargeted command after it stays with its session.
		{route: "s1", creates: true, cmds: []Cmd{{"split-window", "-t", "%3", "--", "prog", "a"}}},
		{route: "s1", cmds: []Cmd{{"select-pane", "-T", "new"}}},
		{route: "s3", creates: true, cmds: []Cmd{{"new-session", "-d", "-s", "s3"}}},
		// display-menu is routed by its client and rewritten for psmux; an
		// untargeted command after it goes to the same server.
		{route: "s2", cmds: []Cmd{
			{"display-menu", "A", "a", "run-shell x"},
			{"kill-server"},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("plan:\n got %+v\nwant %+v", got, want)
	}
	if gs := plan(nil); len(gs) != 0 {
		t.Errorf("plan(nil) = %+v", gs)
	}
}

// plan must not change the caller's commands (they may be reused).
func TestPlanKeepsInput(t *testing.T) {
	in := []Cmd{{"send-keys", "-t", "%3~s1", "#{pane_id}"}, {"split-window", "-t", "%3~s1", "prog", "a"}}
	keep := []Cmd{append(Cmd(nil), in[0]...), append(Cmd(nil), in[1]...)}
	plan(in)
	if !reflect.DeepEqual(in, keep) {
		t.Errorf("plan changed its input: %q", in)
	}
}
