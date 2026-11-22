package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestWindowsInstallArtifacts (T90.8, §7.11): install.ps1 is ASCII and free of
// backticks (for Windows PowerShell and AMSI), scripts/installer.nsi defines
// the per-user NSIS installer, and packaging/winget holds the WinGet manifest.
func TestWindowsInstallArtifacts(t *testing.T) {
	root := testutil.ModuleRoot(t)

	ps1, err := os.ReadFile(filepath.Join(root, "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range ps1 {
		if b > 0x7f {
			t.Fatalf("install.ps1 byte %d (0x%02x) is not ASCII", i, b)
		}
		if b == '`' {
			t.Fatalf("install.ps1 byte %d is a backtick", i)
		}
	}
	ps1Text := string(ps1)
	for _, want := range []string{
		`Programs\i-mux`,
		`imux-core.exe`,
		`imux.exe`,
		`checksums.txt`,
		`Get-FileHash -Algorithm SHA256`,
	} {
		if !strings.Contains(ps1Text, want) {
			t.Errorf("install.ps1 missing %q", want)
		}
	}

	nsiBytes, err := os.ReadFile(filepath.Join(root, "scripts", "installer.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	nsi := string(nsiBytes)
	for _, want := range []string{
		`RequestExecutionLevel user`,
		`InstallDir "$LOCALAPPDATA\Programs\i-mux"`,
		`wt.exe`,
		`$SMPROGRAMS\i-mux.lnk`,
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\i-mux`,
		`WriteUninstaller "$INSTDIR\uninstall.exe"`,
		// D89: the Nerd Font comes with the install; /NOFONT skips it.
		`nsExec::ExecToLog '"$INSTDIR\imux.exe" font install --yes'`,
		`${GetOptions} $R0 "/NOFONT" $R1`,
	} {
		if !strings.Contains(nsi, want) {
			t.Errorf("scripts/installer.nsi missing %q", want)
		}
	}

	wgBytes, err := os.ReadFile(filepath.Join(root, "packaging", "winget", "hamzayogurtcuoglu.i-mux.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	wg := string(wgBytes)
	for _, want := range []string{
		`PackageIdentifier: hamzayogurtcuoglu.i-mux`,
		`InstallerType: nullsoft`,
		`Scope: user`,
		`Architecture: x64`,
		`Architecture: arm64`,
		`_windows_amd64_setup.exe`,
		`_windows_arm64_setup.exe`,
	} {
		if !strings.Contains(wg, want) {
			t.Errorf("winget manifest missing %q", want)
		}
	}
}
