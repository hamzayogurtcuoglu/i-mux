//go:build windows

package notify

import (
	"os/exec"
	"strings"
	"testing"
)

// The toast's script loads WinRT's types and builds the toast, its text escaped, without showing
// it (Show is left out so the test puts nothing on the screen).
func TestToastScriptOnWindows(t *testing.T) {
	script := ToastScript(Message{Title: `it's <1> & "2"`, Body: "body\nline"})
	i := strings.Index(script, "[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier")
	if i < 0 {
		t.Fatalf("script %q", script)
	}
	check := script[:i] + "$t = New-Object Windows.UI.Notifications.ToastNotification $x; [Console]::Write($x.GetXml())"
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", check).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(string(out), `it's &lt;1&gt; &amp; "2"`) || !strings.Contains(string(out), "body line") {
		t.Errorf("toast %s", out)
	}
}
