package editor

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
	"runtime"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/fetch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestMicroAssets(t *testing.T) {
	for _, p := range [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}} {
		a, ok := MicroFor(p[0], p[1])
		if !ok || len(a.Sum) != 64 || a.Size < 4<<20 || !strings.HasPrefix(MicroURL(a), "https://github.com/zyedidia/micro/releases/download/v2.0.15/micro-2.0.15-") {
			t.Errorf("%v: %+v %s", p, a, MicroURL(a))
		}
	}
	if a, _ := MicroFor("windows", "amd64"); !strings.HasSuffix(a.Name, ".zip") {
		t.Errorf("windows: %+v", a)
	}
}

// InstallMicro downloads the archive, checks it, unpacks micro into
// ~/.imux/tools and runs it; a download that is not the release's leaves
// nothing.
func TestInstallMicro(t *testing.T) {
	if !CanGetMicro() {
		t.Skip("no micro for this system")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the fake micro is a shell script; fetch's tests cover the .zip")
	}
	i18n.SetLocale("en")
	testutil.Isolate(t)
	LookPath = func(string) (string, error) { return "", errors.New("no") }
	t.Cleanup(func() { LookPath = defaultLookPath })
	body := microTgz(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()
	t.Setenv("IMUX_MICRO_URL", srv.URL)
	old := microSum
	defer func() { microSum = old }()

	microSum = func(MicroAsset) string { return strings.Repeat("0", 64) }
	if err := InstallMicro(context.Background(), nil, false); !errors.Is(err, fetch.ErrChecksum) || OurMicro() != "" {
		t.Fatalf("a wrong download: %v, micro %q", err, OurMicro())
	}
	sum := sha256.Sum256(body)
	microSum = func(MicroAsset) string { return hex.EncodeToString(sum[:]) }
	if err := InstallMicro(context.Background(), nil, false); err != nil {
		t.Fatal(err)
	}
	if OurMicro() == "" || !HasMicro() {
		t.Fatal("not installed")
	}
}

// microTgz is a release archive with a micro that answers -version.
func microTgz(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, e := range []struct {
		name string
		typ  byte
		mode int64
		body string
	}{
		{"micro-2.0.15/", tar.TypeDir, 0o755, ""},
		{"micro-2.0.15/micro", tar.TypeReg, 0o755, "#!/bin/sh\necho Version: 2.0.15\n"},
		{"micro-2.0.15/LICENSE", tar.TypeReg, 0o644, "MIT"},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: e.typ, Mode: e.mode, Size: int64(len(e.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
