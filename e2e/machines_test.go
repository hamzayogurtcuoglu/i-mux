//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// fakeSSH is an ssh that reaches a second i-mux on this computer: the
// options up to "--" skipped, the destination logged, the command run by a
// shell with the "machine's" IMUX_HOME and tmux socket. While $R/down
// exists it fails as an unreachable host does (also when a command it ran
// ends: the connection dropped).
const fakeSSH = `#!/bin/sh
R=@R@
while [ $# -gt 0 ]; do
	case "$1" in
	--) shift; break ;;
	-O) exit 0 ;;
	*) shift ;;
	esac
done
echo "$1" >> "$R/dests"
shift
down() { echo "ssh: connect to host box port 22: Connection refused" >&2; exit 255; }
[ -e "$R/down" ] && down
unset TMUX TMUX_PANE
export IMUX_HOME="$R/home" IMUX_TMUX_SOCKET=@SOCK@ IMUX_FAKE_LOG="$R/log" IMUX_TMUX_RECORD="$R/record" PATH="$R/bin:$PATH"
if [ $# -eq 0 ]; then exec /bin/sh; fi
/bin/sh -c "$1"
code=$?
[ -e "$R/down" ] && down
exit $code
`

// TestMachines (M78, D73): a machine added over (a fake) SSH shows its
// project and agents in the sidebar; its agent's question is notified; the
// project opens in a tab here attached to it; the connection lost shows
// the machine offline, and back connected; the tab connects again.
func TestMachines(t *testing.T) {
	dir := t.TempDir()
	notified := filepath.Join(dir, "notified")
	notifier := filepath.Join(dir, "fake-notifier")
	testutil.WriteFile(t, notifier, []byte("#!/bin/sh\nprintf '%s|%s\\n' \"$1\" \"$2\" >> '"+notified+"'\n"))
	if err := os.Chmod(notifier, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, sidebarConfig+"\n[notify]\nenabled = true\nsound = false\ncommand = \""+notifier+"\"\n")

	// The machine: its own i-mux home, tmux server and a project.
	r := filepath.Join(h.Homes.Root, "box")
	sock := h.Socket + "-box"
	for _, d := range []string{"home", "log", "bin", "srv-web"} {
		if err := os.MkdirAll(filepath.Join(r, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(h.Bin, filepath.Join(r, "bin", "imux")); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, filepath.Join(r, "home", "config.toml"), []byte(h.readFile(filepath.Join(h.Homes.Imux, "config.toml"))))
	ssh := filepath.Join(dir, "ssh")
	testutil.WriteFile(t, ssh, []byte(strings.NewReplacer("@R@", r, "@SOCK@", sock).Replace(fakeSSH)))
	if err := os.Chmod(ssh, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IMUX_SSH", ssh) // before the watcher starts
	remoteEnv := []string{"IMUX_HOME=" + filepath.Join(r, "home"), "IMUX_TMUX_SOCKET=" + sock, "IMUX_FAKE_LOG=" + filepath.Join(r, "log"), "IMUX_TMUX_RECORD=" + filepath.Join(r, "record")}
	remote := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(h.Bin, args...)
		cmd.Dir = filepath.Join(r, "srv-web")
		cmd.Env = append(os.Environ(), remoteEnv...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("remote imux %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	rtmux := func(args ...string) string {
		out, _ := exec.Command(h.Tmux, append([]string{"-L", sock}, args...)...).CombinedOutput()
		return string(out)
	}
	t.Cleanup(func() {
		cmd := exec.Command(h.Bin, "__watch", "stop")
		cmd.Env = append(os.Environ(), remoteEnv...)
		_ = cmd.Run()
		_ = exec.Command(h.Tmux, "-L", sock, "kill-server").Run()
	})
	remote("claude", "-d")
	// Its agent's state is set by hand: no watcher of its own.
	remote("__watch", "stop")
	var rsession, rpane string
	h.waitFor("the machine's agent", 10*time.Second, func() bool {
		for _, l := range strings.Split(rtmux("list-panes", "-a", "-F", "#{session_name} #{pane_id} #{@imux_agent}"), "\n") {
			if f := strings.Fields(l); len(f) == 3 {
				rsession, rpane = f[0], f[1]
			}
		}
		return rpane != ""
	})
	rtmux("set-option", "-p", "-t", rpane, "@imux_state", "working")

	_, bar := h.startWithSidebar()
	out, errOut, err := h.imux("machine", "add", "ali@box.example.com")
	if err != nil {
		t.Fatalf("machine add: %v\n%s\n%s", err, out, errOut)
	}
	if !strings.Contains(h.readFile(filepath.Join(r, "dests")), "ali@box.example.com") {
		t.Fatal("ssh was not asked")
	}
	if ls := h.mustImux("machine", "ls"); !strings.Contains(ls, "box") || !strings.Contains(ls, "ali@box.example.com") {
		t.Fatalf("ls:\n%s", ls)
	}
	if fi, err := os.Stat(filepath.Join(h.Homes.Imux, "machines.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("machines.json: %v %v", fi, err)
	}

	// In the sidebar: the machine, its project, its agent.
	h.waitFor("the machine in the sidebar", 20*time.Second, func() bool {
		s := h.screen(bar)
		return strings.Contains(s, "💻 box") && strings.Contains(s, "● connected") && strings.Contains(s, "srv-web") &&
			strings.Contains(s, "box · srv-web")
	})

	// Its agent asks: a notification here.
	rtmux("set-option", "-p", "-t", rpane, "@imux_state", "blocked")
	h.waitFor("the question notified", 20*time.Second, func() bool {
		return strings.Contains(h.readFile(notified), "Claude needs you · box · srv-web|on box · srv-web")
	})

	// Open it here: a tab attached to the machine's project.
	h.mustImux("machine", "open", "box", "--session", rsession, "--remote-pane", rpane)
	var view string
	h.waitFor("the machine's tab", 10*time.Second, func() bool {
		for _, l := range strings.Split(h.mustTmux("list-panes", "-a", "-F", "#{pane_id} #{@imux_remote}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[1] == "box/"+rsession {
				view = f[0]
			}
		}
		return view != ""
	})
	attached := func() bool { return strings.TrimSpace(rtmux("list-clients", "-F", "#{client_name}")) != "" }
	h.waitFor("attached to the machine", 10*time.Second, attached)
	// Opening it again goes to the same tab.
	h.mustImux("machine", "open", "box", "--session", rsession)
	if n := strings.Count(h.mustTmux("list-panes", "-a", "-F", "#{@imux_remote}"), "box/"); n != 1 {
		t.Fatalf("%d tabs of the machine", n)
	}

	// The connection drops: offline in the sidebar, the tab waits.
	testutil.WriteFile(t, filepath.Join(r, "down"), nil)
	rtmux("detach-client", "-a")
	rtmux("detach-client")
	h.waitFor("the tab connecting again", 10*time.Second, func() bool {
		return strings.Contains(h.capture(view), "The connection to box dropped")
	})
	h.waitFor("offline in the sidebar", 30*time.Second, func() bool {
		s := h.screen(bar)
		return strings.Contains(s, "✕ offline") && strings.Contains(s, "srv-web")
	})
	// And back.
	if err := os.Remove(filepath.Join(r, "down")); err != nil {
		t.Fatal(err)
	}
	h.waitFor("attached again", 15*time.Second, attached)
	h.waitFor("connected in the sidebar", 30*time.Second, func() bool { return strings.Contains(h.screen(bar), "● connected") })

	// Removed: gone from the sidebar, its tab closed.
	h.mustImux("machine", "rm", "box")
	h.waitFor("the machine gone", 10*time.Second, func() bool {
		return !strings.Contains(h.screen(bar), "💻 box") && !strings.Contains(h.mustTmux("list-panes", "-a", "-F", "#{@imux_remote}"), "box/")
	})
}
