package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/fetch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.27.0", "0.26.1", true},
		{"0.26.1", "0.26.1", false},
		{"0.26.1", "0.27.0", false},
		{"1.0.0", "0.99.99", true},
		{"v0.10.0", "0.9.9", true},
		{"0.27.0", "dev", false},
		{"x", "0.1.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/v0.27.0", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer srv.Close()
	old := LatestURL
	defer func() { LatestURL = old }()
	LatestURL = srv.URL + "/releases/latest"
	if v, err := Latest(context.Background()); err != nil || v != "0.27.0" {
		t.Fatalf("latest %q %v", v, err)
	}
	LatestURL = srv.URL + "/down"
	if _, err := Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "504") {
		t.Fatalf("an outage: %v", err)
	}
}

func TestInstallCmd(t *testing.T) {
	c := InstallCmd("0.27.0", "/Users/me/.local/bin")
	if c[0] != "sh" || !strings.Contains(c[2], "--version") || c[4] != "https://github.com/"+Repo+"/releases/download/v0.27.0/install.sh" ||
		c[5] != "v0.27.0" || c[6] != "/Users/me/.local/bin" {
		t.Fatalf("%q", c)
	}
	if !Homebrew("/opt/homebrew/Cellar/imux/0.26.1/bin") || Homebrew("/Users/me/.local/bin") {
		t.Fatal("homebrew")
	}
}

func makeTestArchive(t *testing.T, name string, files map[string][]byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	if strings.HasSuffix(name, ".zip") {
		zw := zip.NewWriter(&buf)
		for k, v := range files {
			h := &zip.FileHeader{Name: k, Method: zip.Deflate}
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

func TestInstallArchiveAndRollback(t *testing.T) {
	imuxName := "imux" + sysx.ExeSuffix
	coreName := "imux-core" + sysx.ExeSuffix
	archName := ArchiveName("9.9.9")

	oldImux, oldCore := []byte("old-imux-bin"), []byte("old-core-bin")
	newImux, newCore := []byte("new-imux-9.9.9"), []byte("new-core-9.9.9")

	var archBytes []byte
	var sumsText string
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
	prevBase := BaseURL
	BaseURL = srv.URL
	defer func() { BaseURL = prevBase }()

	dir := t.TempDir()
	seedOld := func() {
		t.Helper()
		for _, p := range []string{imuxName, coreName, imuxName + ".old", coreName + ".old"} {
			_ = os.RemoveAll(filepath.Join(dir, p))
		}
		if err := os.WriteFile(filepath.Join(dir, imuxName), oldImux, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, coreName), oldCore, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	hasContent := func(name string, want []byte) bool {
		b, err := os.ReadFile(filepath.Join(dir, name))
		return err == nil && bytes.Equal(b, want)
	}

	// 1. Both replaced; *.old kept until CleanOldIn runs on the next start.
	seedOld()
	var sum string
	archBytes, sum = makeTestArchive(t, archName, map[string][]byte{
		imuxName:  newImux,
		coreName:  newCore,
		"LICENSE": []byte("MIT"),
	})
	sumsText = sum + "  " + archName + "\n"
	var out bytes.Buffer
	if err := InstallArchive(context.Background(), "9.9.9", dir, &out); err != nil {
		t.Fatalf("InstallArchive: %v", err)
	}
	if !hasContent(imuxName, newImux) || !hasContent(coreName, newCore) {
		t.Fatalf("binaries not replaced")
	}
	if !hasContent(imuxName+".old", oldImux) || !hasContent(coreName+".old", oldCore) {
		t.Fatalf("*.old files not kept after replace")
	}
	CleanOldIn(dir)
	for _, old := range []string{imuxName + ".old", coreName + ".old"} {
		if _, err := os.Stat(filepath.Join(dir, old)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s still exists after CleanOldIn: %v", old, err)
		}
	}

	// 2. Corrupt checksum: nothing in dir changes.
	seedOld()
	sumsText = strings.Repeat("0", 64) + "  " + archName + "\n"
	if err := InstallArchive(context.Background(), "9.9.9", dir, nil); !errors.Is(err, fetch.ErrChecksum) {
		t.Fatalf("corrupt checksum err = %v", err)
	}
	if !hasContent(imuxName, oldImux) || !hasContent(coreName, oldCore) {
		t.Fatalf("corrupt archive changed binaries")
	}

	// 3. Archive missing imux-core: nothing in dir changes.
	seedOld()
	archBytes, sum = makeTestArchive(t, archName, map[string][]byte{imuxName: newImux})
	sumsText = sum + "  " + archName + "\n"
	if err := InstallArchive(context.Background(), "9.9.9", dir, nil); err == nil || !strings.Contains(err.Error(), "archive has no") {
		t.Fatalf("incomplete archive err = %v", err)
	}
	if !hasContent(imuxName, oldImux) || !hasContent(coreName, oldCore) {
		t.Fatalf("incomplete archive changed binaries")
	}

	// 4. Rollback: imux-core is replaced first, then replacing imux fails
	// (imux.old is a directory); imux-core is restored from imux-core.old.
	seedOld()
	archBytes, sum = makeTestArchive(t, archName, map[string][]byte{
		imuxName: newImux,
		coreName: newCore,
	})
	sumsText = sum + "  " + archName + "\n"
	if err := os.Mkdir(filepath.Join(dir, imuxName+".old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := InstallArchive(context.Background(), "9.9.9", dir, nil); err == nil {
		t.Fatalf("expected failure when %s.old is a directory", imuxName)
	}
	if !hasContent(coreName, oldCore) || !hasContent(imuxName, oldImux) {
		t.Fatalf("rollback failed: core=%v imux=%v", hasContent(coreName, oldCore), hasContent(imuxName, oldImux))
	}
	if _, err := os.Stat(filepath.Join(dir, coreName+".old")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s.old left behind after rollback", coreName)
	}
	for _, b := range []string{coreName, imuxName} {
		if _, err := os.Stat(filepath.Join(dir, b+".new")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s.new left behind after rollback", b)
		}
	}
}
