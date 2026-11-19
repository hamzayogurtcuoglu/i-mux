//go:build windows

package update

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestInstallPS1Windows (T90.8, §7.11): runs install.ps1 in Windows PowerShell
// against a local release fixture: checksum verification, installing both
// imux-core.exe and imux.exe, rejecting a bad checksum, and rolling back when
// imux.exe cannot be replaced.
func TestInstallPS1Windows(t *testing.T) {
	script := filepath.Join(testutil.ModuleRoot(t), "install.ps1")
	archName := ArchiveName("9.9.9")
	oldImux, oldCore := []byte("old-imux-exe"), []byte("old-core-exe")
	newImux, newCore := []byte("new-imux-9.9.9"), []byte("new-core-9.9.9")

	archBytes, sum := makeTestArchive(t, archName, map[string][]byte{
		"imux.exe":      newImux,
		"imux-core.exe": newCore,
		"LICENSE":       []byte("MIT"),
	})
	sumsText := sum + "  " + archName + "\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v9.9.9/checksums.txt":
			_, _ = w.Write([]byte(sumsText))
		case "/v9.9.9/" + archName:
			_, _ = w.Write(archBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "Programs", "i-mux")
	runPS1 := func() (string, error) {
		cmd := exec.Command("powershell.exe",
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-File", script,
			"-Version", "v9.9.9",
			"-InstallDir", dir,
			"-NoModifyPath", "-NoFont",
		)
		cmd.Env = append(os.Environ(), "IMUX_INSTALL_BASE_URL="+srv.URL)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	hasContent := func(name string, want []byte) bool {
		b, err := os.ReadFile(filepath.Join(dir, name))
		return err == nil && bytes.Equal(b, want)
	}

	// 1. Fresh install of both binaries.
	out, err := runPS1()
	if err != nil {
		t.Fatalf("install.ps1: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Checksum verified.") || !hasContent("imux.exe", newImux) || !hasContent("imux-core.exe", newCore) {
		t.Fatalf("binaries not installed:\n%s", out)
	}

	// 2. Corrupt checksum rejected; existing binaries untouched.
	if err := os.WriteFile(filepath.Join(dir, "imux.exe"), oldImux, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "imux-core.exe"), oldCore, 0o755); err != nil {
		t.Fatal(err)
	}
	sumsText = strings.Repeat("0", 64) + "  " + archName + "\n"
	out, err = runPS1()
	if err == nil || !strings.Contains(out, "checksum mismatch") {
		t.Fatalf("corrupt checksum accepted: err=%v\n%s", err, out)
	}
	if !hasContent("imux.exe", oldImux) || !hasContent("imux-core.exe", oldCore) {
		t.Fatalf("corrupt install changed binaries")
	}

	// 3. Rollback when imux.exe.old is a directory: imux-core.exe is restored.
	sumsText = sum + "  " + archName + "\n"
	if err := os.Mkdir(filepath.Join(dir, "imux.exe.old"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err = runPS1()
	if err == nil || !strings.Contains(out, "nothing was installed") {
		t.Fatalf("expected rollback failure: err=%v\n%s", err, out)
	}
	if !hasContent("imux-core.exe", oldCore) || !hasContent("imux.exe", oldImux) {
		t.Fatalf("rollback failed in install.ps1")
	}
}
