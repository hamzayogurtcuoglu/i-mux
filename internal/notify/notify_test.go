package notify

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"strings"
	"testing"
)

type call struct {
	name string
	args []string
}

func fake(goos string, have []string, fail map[string]bool) (*Notifier, *[]call) {
	var calls []call
	n := &Notifier{
		GOOS: goos,
		LookPath: func(name string) (string, error) {
			for _, h := range have {
				if h == name {
					return "/usr/bin/" + name, nil
				}
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, name string, args ...string) error {
			calls = append(calls, call{name, args})
			if fail[name] {
				return errors.New("failed")
			}
			return nil
		},
	}
	return n, &calls
}

func TestSystem(t *testing.T) {
	m := Message{Title: `Claude finished · "api"`, Body: `tab 1 \ ok`, Sound: true}
	ctx := context.Background()

	n, calls := fake("darwin", []string{"osascript"}, nil)
	if how, err := n.System(ctx, m); err != nil || how != "osascript" {
		t.Fatalf("darwin: %s %v", how, err)
	}
	if c := (*calls)[0]; c.name != "osascript" || c.args[0] != "-e" ||
		c.args[1] != `display notification "tab 1 \\ ok" with title "Claude finished · \"api\"" sound name "Glass"` {
		t.Fatalf("osascript %q", c.args)
	}

	n, calls = fake("linux", []string{"notify-send", "gdbus"}, nil)
	if how, _ := n.System(ctx, m); how != "notify-send" {
		t.Fatalf("linux: %s", how)
	}
	if c := (*calls)[0]; strings.Join(c.args, "|") != "-a|i-mux|-h|string:sound-name:message-new-instant|"+m.Title+"|"+m.Body {
		t.Fatalf("notify-send %q", c.args)
	}

	// notify-send fails (no D-Bus session for it): gdbus is tried.
	n, calls = fake("linux", []string{"notify-send", "gdbus"}, map[string]bool{"notify-send": true})
	if how, err := n.System(ctx, m); how != "gdbus" || err != nil || len(*calls) != 2 || !strings.Contains(strings.Join((*calls)[1].args, " "), "org.freedesktop.Notifications.Notify") {
		t.Fatalf("gdbus fallback: %s %v %q", how, err, *calls)
	}

	// No desktop (SSH, a server): nothing; the caller writes to the terminals.
	n, _ = fake("linux", nil, nil)
	if _, err := n.System(ctx, m); !errors.Is(err, ErrNone) {
		t.Fatalf("no desktop: %v", err)
	}
	n, _ = fake("linux", []string{"gdbus"}, map[string]bool{"gdbus": true})
	if _, err := n.System(ctx, m); !errors.Is(err, ErrNone) {
		t.Fatalf("gdbus without a session: %v", err)
	}

	// A configured command wins on every platform.
	for _, goos := range []string{"darwin", "linux"} {
		n, calls = fake(goos, []string{"osascript", "notify-send"}, nil)
		n.Command = []string{"ntfy", "publish", "mytopic"}
		if how, _ := n.System(ctx, m); how != "command" || (*calls)[0].name != "ntfy" ||
			strings.Join((*calls)[0].args, "|") != "publish|mytopic|"+m.Title+"|"+m.Body {
			t.Fatalf("%s command: %q", goos, *calls)
		}
	}
}

type buf struct{ bytes.Buffer }

func (b *buf) Close() error { return nil }

func TestTerminals(t *testing.T) {
	m := Message{Title: "Codex needs you; now", Body: "line\x07one\nend"}
	if s := OSC("iTerm.app", m); s != "\x1b]9;Codex needs you; now: line one end\a" {
		t.Fatalf("iterm %q", s)
	}
	if s := OSC("ghostty", m); s != "\x1b]777;notify;Codex needs you, now;line one end\a" {
		t.Fatalf("osc 777 %q", s)
	}
	written := map[string]*buf{}
	n := &Notifier{OpenTTY: func(p string) (interface {
		Write([]byte) (int, error)
		Close() error
	}, error,
	) {
		if p == "/dev/gone" {
			return nil, errors.New("gone")
		}
		b := &buf{}
		written[p] = b
		return b, nil
	}}
	got := n.Terminals([]Client{{TTY: "/dev/pts/1", Program: "WezTerm"}, {TTY: "/dev/gone"}, {TTY: ""}}, m)
	if got != 1 || !strings.HasPrefix(written["/dev/pts/1"].String(), "\x1b]777;notify;") {
		t.Fatalf("terminals: %d %q", got, written)
	}
}

func TestWindowsToast(t *testing.T) {
	ctx := context.Background()
	m := Message{Title: `Claude finished · <api> & "x"`, Body: "tab 1\nok 'q'", Sound: true}
	n, calls := fake("windows", []string{"powershell.exe"}, nil)
	if how, err := n.System(ctx, m); err != nil || how != "toast" {
		t.Fatalf("windows: %s %v", how, err)
	}
	c := (*calls)[0]
	if c.name != "powershell.exe" || strings.Join(c.args[:3], " ") != "-NoProfile -NonInteractive -Command" {
		t.Fatalf("powershell %q %q", c.name, c.args)
	}
	script := c.args[3]
	if !strings.Contains(script, "CreateToastNotifier('"+ToastApp+"')") {
		t.Fatalf("app id:\n%s", script)
	}
	// The text reaches the toast only inside the base64 XML.
	if strings.Contains(script, "Claude") {
		t.Fatalf("text outside the base64:\n%s", script)
	}
	i := strings.Index(script, "FromBase64String('") + len("FromBase64String('")
	raw, err := base64.StdEncoding.DecodeString(script[i : i+strings.Index(script[i:], "'")])
	if err != nil {
		t.Fatal(err)
	}
	x := string(raw)
	for _, want := range []string{
		"<text>Claude finished · &lt;api&gt; &amp; &#34;x&#34;</text>",
		"<text>tab 1 ok &#39;q&#39;</text>",
		`<text placement="attribution">i-mux</text>`,
		`<audio src="ms-winsoundevent:Notification.Default"/>`,
	} {
		if !strings.Contains(x, want) {
			t.Errorf("toast lacks %q:\n%s", want, x)
		}
	}
	var v struct{ XMLName xml.Name }
	if err := xml.Unmarshal(raw, &v); err != nil || v.XMLName.Local != "toast" {
		t.Fatalf("not XML: %v\n%s", err, x)
	}
	// No sound asked: a silent toast.
	m.Sound = false
	if x := toastXML(m); !strings.Contains(x, `<audio silent="true"/>`) || strings.Contains(x, "ms-winsoundevent") {
		t.Fatalf("silent %s", x)
	}
	// Without PowerShell: nothing; a configured command still wins.
	n, _ = fake("windows", nil, nil)
	if _, err := n.System(ctx, m); !errors.Is(err, ErrNone) {
		t.Fatalf("no powershell: %v", err)
	}
	n, calls = fake("windows", []string{"powershell.exe"}, nil)
	n.Command = []string{"ntfy", "publish"}
	if how, _ := n.System(ctx, m); how != "command" || (*calls)[0].name != "ntfy" {
		t.Fatalf("command: %q", *calls)
	}
}
