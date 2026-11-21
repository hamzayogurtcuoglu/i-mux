//go:build e2e && windows

package e2e

// windowsSkips are the e2e tests that cannot apply on Windows (T90.9), each
// with its reason; every other test runs there on i-mux-core.
var windowsSkips = map[string]string{
	// install.sh is the installer of macOS and Linux; install.ps1 has its own test
	// (internal/update, TestInstallPS1Windows) and CI's windows-installer job.
	"TestInstallScript":             "install.sh (macOS and Linux)",
	"TestInstallScriptWithoutTmux":  "install.sh (macOS and Linux)",
	"TestInstallScriptAsksForTmux":  "install.sh (macOS and Linux)",
	"TestInstallScriptOffersFont":   "install.sh (macOS and Linux)",
	"TestInstallScriptReplacesBoth": "install.sh (macOS and Linux)",

	// They find i-mux's processes by command line with pgrep or ps.
	"TestWatcherLifecycle":      "pgrep finds the watcher by its command line",
	"TestSidebarRevivesWatcher": "pgrep finds the watcher by its command line",
	"TestNoResidentProcesses":   "ps lists the processes' command lines",

	// Their fakes are sh scripts that Windows cannot run as programs.
	"TestExplorerHover":      "the fake editor is an sh script",
	"TestExplorerMenus":      "the fake editor is an sh script",
	"TestExplorerAndSearch":  "the fake editor is an sh script",
	"TestExplorerTitle":      "the fake editor is an sh script",
	"TestGoToFile":           "the fake editor is an sh script",
	"TestSkillsInJobs":       "the fake editor is an sh script",
	"TestEnvNewPopup":        "the fake editor is an sh script",
	"TestGetMicroDialog":     "the fake nano and micro are sh scripts",
	"TestCompletionEdit":     "the fake nvim is an sh script",
	"TestMachines":           "the fake ssh is an sh script",
	"TestNotifications":      "the fake notifier is an sh command",
	"TestPasteImage":         "the fake screenshot tool is an sh command",
	"TestLivePreview":        "the fake browser is an sh redirect",
	"TestVersionsInstall":    "the configured install command is sh",
	"TestSourceControlMenus": "the copy command (IMUX_COPY_COMMAND) is sh",

	// What only macOS and Linux have.
	"TestClick":                       "macOS and Linux install commands (npm); Windows' are winget's",
	"TestInstallMissingAgent":         "macOS and Linux install commands (npm); Windows' are winget's",
	"TestOnlyTheEngine":               "the fake tmux and engine are sh scripts",
	"TestOpenPickerAndClose":          "'/'-separated folders (~/code/acme-api)",
	"TestSidebarProjectsNamesChanges": "'/'-separated folders (~/blog)",

	// Latency budgets measured for macOS and Linux: a process starts slower on Windows.
	"TestPaneMenu":      "the menu latency budget of macOS and Linux",
	"TestSessionPicker": "the picker latency budget of macOS and Linux",

	// What Windows' clients cannot have: a theme change paints an attached terminal's
	// background by writing OSC 11 to the client's tty device (D77), and a Windows client has
	// none; it gets the background when it attaches, as on every system.
	"TestThemeTerminalBackground": "a theme change's OSC 11 is written to a client's tty device (D77); Windows clients have none",
}
