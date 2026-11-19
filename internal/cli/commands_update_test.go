package cli

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/update"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
)

// engineRunner answers -V with version and list-sessions with running.
type engineRunner struct {
	version string
	running bool
}

func (r engineRunner) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	if slices.Equal(args, []string{"-V"}) {
		return []byte(r.version + "\n"), nil, nil
	}
	if slices.Contains(args, "list-sessions") && !r.running {
		return nil, []byte("no server running on /tmp/imux-core-501/imux\n"), errors.New("exit status 1")
	}
	return []byte("proj\n"), nil, nil
}

// TestCoreKeepsOld (T88.4): the update says the engine keeps its old version
// only when sessions run on i-mux-core; never for tmux, whose server is not
// i-mux's.
func TestCoreKeepsOld(t *testing.T) {
	for _, c := range []struct {
		version string
		running bool
		want    bool
	}{
		{"tmux 3.8 (imux-core 0.1.0)", true, true},
		{"tmux 3.8 (imux-core 0.1.0)", false, false},
		{"tmux 3.8", true, false},
		{"tmux 3.5a", false, false},
	} {
		cl := &tmux.Client{Bin: "engine", Socket: "imux", Runner: engineRunner{c.version, c.running}}
		if got := coreKeepsOld(context.Background(), cl); got != c.want {
			t.Errorf("%q running=%v: %v, want %v", c.version, c.running, got, c.want)
		}
	}
}

const exeVerMarker = "\nIMUX_TEST_VER="

// selfUpdateHelper turns the test binary into imux or imux-core when copied
// under those names by TestSelfUpdateEndToEnd. Each copy carries its version
// in a short trailer appended after the executable image.
func selfUpdateHelper() (int, bool) {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe")
	if base != "imux" && base != "imux-core" {
		return 0, false
	}
	ver := readExeVer()
	if base == "imux-core" {
		args := os.Args[1:]
		switch {
		case slices.Equal(args, []string{"-V"}):
			fmt.Printf("tmux 3.8 (imux-core %s)\n", ver)
			return 0, true
		case slices.Equal(args, []string{"--hold"}):
			fmt.Println("ready")
			_, _ = io.Copy(io.Discard, os.Stdin)
			return 0, true
		case slices.Contains(args, "list-sessions"):
			if os.Getenv("IMUX_TEST_CORE_RUNNING") == "1" {
				fmt.Println("proj")
				return 0, true
			}
			fmt.Fprintln(os.Stderr, "no server running")
			return 1, true
		default:
			return 0, true
		}
	}
	_ = os.Setenv("IMUX_LANG", "en")
	version.Version = ver
	return Main(os.Args), true
}

func readExeVer() string {
	p, err := os.Executable()
	if err != nil {
		return "9.9.9"
	}
	f, err := os.Open(p)
	if err != nil {
		return "9.9.9"
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return "9.9.9"
	}
	off := max(fi.Size()-128, 0)
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && !errors.Is(err, io.EOF) {
		return "9.9.9"
	}
	s := string(buf)
	i := strings.LastIndex(s, exeVerMarker)
	if i < 0 {
		return "9.9.9"
	}
	s = s[i+len(exeVerMarker):]
	if j := strings.IndexByte(s, '\n'); j >= 0 {
		s = s[:j]
	}
	return strings.TrimSpace(s)
}

func makeTaggedExe(t *testing.T, srcExe, ver string) []byte {
	t.Helper()
	b, err := os.ReadFile(srcExe)
	if err != nil {
		t.Fatal(err)
	}
	if ver == "9.9.9" {
		return b
	}
	return append(b, []byte(exeVerMarker+ver+"\n")...)
}

func packReleaseArchive(t *testing.T, name string, files map[string][]byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	if strings.HasSuffix(name, ".zip") {
		zw := zip.NewWriter(&buf)
		for k, v := range files {
			h := &zip.FileHeader{Name: k, Method: zip.Store}
			h.SetMode(0o755)
			w, err := zw.CreateHeader(h)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(v); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		for k, v := range files {
			if err := tw.WriteHeader(&tar.Header{Name: k, Mode: 0o755, Size: int64(len(v))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(v); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

// TestSelfUpdateEndToEnd (T90.7): a running imux executable updates itself
// and a running imux-core executable from a local release fixture:
//   - downloads and verifies checksums.txt and the archive;
//   - renames both running executables to *.old and writes the new ones;
//   - tells the user that a running engine server keeps its old version until
//     its sessions end;
//   - deletes *.old on the next start;
//   - rolls back imux-core when replacing imux fails.
func TestSelfUpdateEndToEnd(t *testing.T) {
	testutil.Isolate(t)
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Config(), []byte("[engine]\nbackend = \"core\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	selfExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	imuxName := "imux" + sysx.ExeSuffix
	coreName := "imux-core" + sysx.ExeSuffix
	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	imuxPath := filepath.Join(dir, imuxName)
	corePath := filepath.Join(dir, coreName)

	oldBin := makeTaggedExe(t, selfExe, "0.1.0")
	newBin := makeTaggedExe(t, selfExe, "9.9.9")
	if err := os.WriteFile(imuxPath, oldBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corePath, oldBin, 0o755); err != nil {
		t.Fatal(err)
	}

	archName := update.ArchiveName("9.9.9")
	archBytes, sum := packReleaseArchive(t, archName, map[string][]byte{
		imuxName: newBin,
		coreName: newBin,
	})
	sumsText := sum + "  " + archName + "\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			http.Redirect(w, r, "/releases/tag/v9.9.9", http.StatusFound)
		case "/releases/download/v9.9.9/checksums.txt":
			_, _ = w.Write([]byte(sumsText))
		case "/releases/download/v9.9.9/" + archName:
			_, _ = w.Write(archBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("IMUX_TEST_SELF_UPDATE", "1")
	t.Setenv("IMUX_TEST_CORE_RUNNING", "1")
	t.Setenv("IMUX_UPDATE_URL", srv.URL)
	t.Setenv("IMUX_TMUX_BIN", "")

	// Start the old imux-core so its executable image is live during the update.
	hold := exec.Command(corePath, "--hold")
	holdIn, err := hold.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	holdOut, err := hold.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := hold.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = holdIn.Close()
		_ = hold.Wait()
	}()
	if line, err := bufio.NewReader(holdOut).ReadString('\n'); err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("imux-core --hold: %v %q", err, line)
	}

	// 1. Run `imux update` from the running imuxPath while corePath is also running.
	cmd := exec.Command(imuxPath, "update")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("imux update: %v\n%s", err, out)
	}
	for _, want := range []string{
		"Updating i-mux 0.1.0 → 9.9.9",
		"Done: i-mux 9.9.9.",
		"Your sessions run on the i-mux-core engine, which keeps its old version until they all end",
	} {
		if !strings.Contains(string(out), want) {
			vout, verr := exec.Command(corePath, "-V").CombinedOutput()
			lout, lerr := exec.Command(corePath, "-L", "imux", "-u", "list-sessions").CombinedOutput()
			t.Fatalf("update output missing %q:\n%s\ncore -V: %q (%v)\ncore list-sessions: %q (%v)", want, out, vout, verr, lout, lerr)
		}
	}

	// Both *.old files exist right after the update process exits.
	for _, old := range []string{imuxPath + ".old", corePath + ".old"} {
		if _, err := os.Stat(old); err != nil {
			t.Fatalf("expected %s to exist after update: %v", filepath.Base(old), err)
		}
	}

	// End the running old engine server so its .old file can be removed too.
	_ = holdIn.Close()
	_ = hold.Wait()

	// 2. Next start of imux runs the new 9.9.9 binary and cleans up *.old.
	vcmd := exec.Command(imuxPath, "version")
	vout, err := vcmd.CombinedOutput()
	if err != nil || !strings.HasPrefix(string(vout), "imux 9.9.9 ") {
		t.Fatalf("updated imux version: %v %q", err, vout)
	}
	cvcmd := exec.Command(corePath, "-V")
	cvout, err := cvcmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(cvout)) != "tmux 3.8 (imux-core 9.9.9)" {
		t.Fatalf("updated imux-core -V: %v %q", err, cvout)
	}
	for _, old := range []string{imuxPath + ".old", corePath + ".old"} {
		if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s still exists after next start: %v", filepath.Base(old), err)
		}
	}

	// 3. Rollback when replacing imux fails after imux-core was replaced.
	if err := os.WriteFile(imuxPath, oldBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corePath, oldBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(imuxPath+".old", 0o755); err != nil {
		t.Fatal(err)
	}
	fcmd := exec.Command(imuxPath, "update")
	fout, err := fcmd.CombinedOutput()
	if err == nil || !strings.Contains(string(fout), "The update failed:") {
		t.Fatalf("expected update to fail when imux.old is a directory: err=%v\n%s", err, fout)
	}
	if _, err := os.Stat(corePath + ".old"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s.old left behind after rollback", coreName)
	}
	cvcmd = exec.Command(corePath, "-V")
	cvout, err = cvcmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(cvout)) != "tmux 3.8 (imux-core 0.1.0)" {
		t.Fatalf("imux-core was not rolled back to 0.1.0: %v %q", err, cvout)
	}
}
