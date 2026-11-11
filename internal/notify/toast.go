package notify

import (
	"encoding/base64"
	"encoding/xml"
	"strings"
)

// ToastApp is the app a Windows toast comes from: Windows PowerShell's
// AppUserModelID, which every Windows has registered (a toast needs one,
// and imux installs no shortcut of its own). The toast says i-mux in its
// attribution line.
const ToastApp = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

// toastXML is the toast for m: title, body and "i-mux", with the default
// notification sound or none.
func toastXML(m Message) string {
	esc := func(s string) string {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(clean(s)))
		return b.String()
	}
	audio := `<audio silent="true"/>`
	if m.Sound {
		audio = `<audio src="ms-winsoundevent:Notification.Default"/>`
	}
	return `<toast><visual><binding template="ToastGeneric">` +
		`<text>` + esc(m.Title) + `</text><text>` + esc(m.Body) + `</text>` +
		`<text placement="attribution">i-mux</text>` +
		`</binding></visual>` + audio + `</toast>`
}

// toastScript shows m as a toast from Windows PowerShell (5.1, which reaches
// the Windows Runtime; pwsh does not). The XML goes as UTF-8 in base64, so
// no text needs PowerShell quoting.
func toastScript(m Message) string {
	b64 := base64.StdEncoding.EncodeToString([]byte(toastXML(m)))
	return `$ErrorActionPreference='Stop'
$null=[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime]
$null=[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType=WindowsRuntime]
$x=New-Object Windows.Data.Xml.Dom.XmlDocument
$x.LoadXml([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + b64 + `')))
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('` + ToastApp + `').Show([Windows.UI.Notifications.ToastNotification]::new($x))`
}
