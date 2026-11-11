// Package fetch downloads a release asset with its progress, checks it
// against a pinned SHA-256 and unpacks it (the micro editor, D45): a
// .tar.gz, or a .zip on Windows.
package fetch

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// ErrChecksum means the download is not the release's file.
var ErrChecksum = errors.New("the download does not match its checksum")

// Download saves url into file, drawing its progress on out: one line
// redrawn when bar (a terminal), else the URL once. size is what the
// release says, for a server that does not.
func Download(ctx context.Context, url, file string, size int64, out io.Writer, bar bool) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = size
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	p := &progress{out: out, bar: bar, total: total}
	if out != nil && !bar {
		fmt.Fprintln(out, "  "+i18n.T("fetch.downloading")+" "+url)
	}
	_, err = io.Copy(io.MultiWriter(f, p), resp.Body)
	p.end()
	if err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// progress draws a download's bar: ━━━━━─────  42%  2.1 of 4.9 MB.
type progress struct {
	out      io.Writer
	bar      bool
	total, n int64
	last     time.Time
}

func (p *progress) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if now := time.Now(); p.bar && p.out != nil && now.Sub(p.last) >= 100*time.Millisecond {
		p.last = now
		p.draw()
	}
	return len(b), nil
}

func (p *progress) end() {
	if p.bar && p.out != nil {
		p.draw()
		fmt.Fprintln(p.out)
	}
}

func (p *progress) draw() {
	pct := 0
	if p.total > 0 {
		pct = int(min(p.n*100/p.total, 100))
	}
	const width = 30
	done := pct * width / 100
	mb := func(n int64) float64 { return float64(n) / (1 << 20) }
	fmt.Fprintf(p.out, "\r  %s \x1b[36m%s\x1b[38;5;238m%s\x1b[0m %3d%%  %s\x1b[K", i18n.T("fetch.downloading"),
		strings.Repeat("━", done), strings.Repeat("─", width-done), pct, i18n.T("fetch.progress", mb(p.n), mb(p.total)))
}

// Check reports whether file's SHA-256 is want.
func Check(file, want string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return ErrChecksum
	}
	return nil
}

// Untar unpacks the .tar.gz file into dir: folders and regular files, with
// their modes. Any other entry, or a path that would leave dir, is an error.
func Untar(file, dir string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(strings.TrimSuffix(h.Name, "/"))
		if name == "." {
			continue
		}
		if !filepath.IsLocal(name) {
			return fmt.Errorf("unsafe path in %s: %q", filepath.Base(file), h.Name)
		}
		dst := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := write(tr, dst, os.FileMode(h.Mode).Perm()|0o600); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected entry in %s: %q", filepath.Base(file), h.Name)
		}
	}
}

// Unpack unpacks file into dir: a .zip (Windows' releases) with Unzip, else
// a .tar.gz with Untar.
func Unpack(file, dir string) error {
	if strings.HasSuffix(strings.ToLower(strings.TrimSuffix(file, ".part")), ".zip") {
		return Unzip(file, dir)
	}
	return Untar(file, dir)
}

// Unzip unpacks the .zip file into dir: folders and regular files. Any
// other entry, or a path that would leave dir, is an error.
func Unzip(file, dir string) error {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		name := filepath.FromSlash(strings.TrimSuffix(zf.Name, "/"))
		if name == "." || name == "" {
			continue
		}
		if !filepath.IsLocal(name) {
			return fmt.Errorf("unsafe path in %s: %q", filepath.Base(file), zf.Name)
		}
		dst := filepath.Join(dir, name)
		mode := zf.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case mode.IsRegular():
			r, err := zf.Open()
			if err != nil {
				return err
			}
			err = write(r, dst, mode.Perm()|0o600)
			r.Close()
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected entry in %s: %q", filepath.Base(file), zf.Name)
		}
	}
	return nil
}

func write(r io.Reader, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
