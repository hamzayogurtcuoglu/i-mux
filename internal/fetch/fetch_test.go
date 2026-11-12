package fetch

import (
	"archive/tar"
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

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// Entry is a tar entry for tests: a name, its type and mode, its body.
type Entry struct {
	Name string
	Type byte
	Mode int64
	Body string
}

// Tgz writes the entries as a .tar.gz into dir and returns its path.
func Tgz(t *testing.T, dir string, entries []Entry) string {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.Name, Typeflag: e.Type, Mode: e.Mode, Size: int64(len(e.Body))}
		if e.Type == tar.TypeSymlink {
			h.Linkname, h.Size = e.Body, 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if e.Type == tar.TypeReg {
			if _, err := tw.Write([]byte(e.Body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "x.tar.gz")
	if err := os.WriteFile(p, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUntar(t *testing.T) {
	dir := t.TempDir()
	good := Tgz(t, t.TempDir(), []Entry{
		{"micro-2.0.15/", tar.TypeDir, 0o755, ""},
		{"micro-2.0.15/micro", tar.TypeReg, 0o755, "#!/bin/sh\n"},
		{"micro-2.0.15/LICENSE", tar.TypeReg, 0o644, "MIT"},
	})
	if err := Untar(good, dir); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "micro-2.0.15", "micro")); err != nil || fi.Mode()&0o100 == 0 {
		t.Fatalf("the program: %v %v", fi, err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "micro-2.0.15", "LICENSE")); err != nil || fi.Mode()&0o100 != 0 {
		t.Errorf("the license: %v %v", fi, err)
	}
	for _, bad := range []Entry{
		{"../evil", tar.TypeReg, 0o644, "x"},
		{"/etc/evil", tar.TypeReg, 0o644, "x"},
		{"micro-2.0.15/link", tar.TypeSymlink, 0o777, "/etc/passwd"},
	} {
		if err := Untar(Tgz(t, t.TempDir(), []Entry{bad}), t.TempDir()); err == nil {
			t.Errorf("%q unpacked", bad.Name)
		}
	}
}

func TestDownloadAndCheck(t *testing.T) {
	i18n.SetLocale("en")
	body := strings.Repeat("micro", 50000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gone" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "a.part")
	var out bytes.Buffer
	if err := Download(context.Background(), srv.URL+"/a", file, int64(len(body)), &out, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "100%") || !strings.Contains(out.String(), "0.2 of 0.2 MB") {
		t.Errorf("progress %q", out.String())
	}
	h := sha256.Sum256([]byte(body))
	if Check(file, hex.EncodeToString(h[:])) != nil || !errors.Is(Check(file, strings.Repeat("0", 64)), ErrChecksum) {
		t.Error("checksum")
	}
	if err := Download(context.Background(), srv.URL+"/gone", file, 0, nil, false); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("a missing file: %v", err)
	}
}
