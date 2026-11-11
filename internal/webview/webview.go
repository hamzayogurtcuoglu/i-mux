// Package webview is the live preview (D47): a small HTTP server per
// folder that serves its pages on localhost and reloads them when their
// files change, and the records the CLI finds those servers by. Opening
// the browser is fileops'.
package webview

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

// IsHTML reports a web page file: a click on it in the explorer opens its
// preview.
func IsHTML(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm", ".xhtml":
		return true
	}
	return false
}

// Previewable reports a file the preview shows: a web page or an SVG image.
func Previewable(name string) bool {
	return IsHTML(name) || strings.EqualFold(filepath.Ext(name), ".svg")
}

var winDrive = regexp.MustCompile(`^/[A-Za-z]:/`)

// fileURLPath is the file a file:// URL's path names: on Windows
// /C:/x/a.html is C:\x\a.html.
func fileURLPath(p string) string {
	if filepath.Separator == '\\' && winDrive.MatchString(p) {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

var portOnly = regexp.MustCompile(`^:?[0-9]{1,5}([/?#].*)?$`)

// Target turns what the user gave into a file or a URL: an existing path
// (absolute, ~/…, or relative to dir) stays a path (absolute); a port
// ("3000", ":3000/api") is http://localhost:3000; text without a scheme
// gets http:// (localhost and IP addresses) or https://. Blank stays "".
func Target(s, dir string) (file, u string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if strings.HasPrefix(s, "file://") {
		if pu, err := url.Parse(s); err == nil {
			return fileURLPath(pu.Path), ""
		}
	}
	if strings.Contains(s, "://") {
		return "", s
	}
	for _, scheme := range []string{"about:", "data:"} {
		if strings.HasPrefix(s, scheme) {
			return "", s
		}
	}
	p := s
	if strings.HasPrefix(p, "~/") {
		p = filepath.Join(paths.UserHome(), p[2:])
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	if _, err := os.Stat(p); err == nil {
		return p, ""
	}
	if portOnly.MatchString(s) {
		return "", "http://localhost:" + strings.TrimPrefix(s, ":")
	}
	host := s
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host) != nil {
		return "", "http://" + s
	}
	return "", "https://" + s
}

// Label names a page in messages: a file's name, else the URL's host and
// path ("localhost:3000", "go.dev/doc").
func Label(u string) string {
	pu, err := url.Parse(u)
	if err != nil {
		return u
	}
	if l := pu.Host + strings.TrimSuffix(pu.Path, "/"); l != "" {
		return l
	}
	return u
}

// RootFor is the folder a file is served from: the project when the file
// is in it, else the file's folder (a folder: itself).
func RootFor(file, project string) string {
	if project != "" {
		if rel, err := filepath.Rel(project, file); err == nil && filepath.IsLocal(rel) {
			return project
		}
	}
	if fi, err := os.Stat(file); err == nil && fi.IsDir() {
		return file
	}
	return filepath.Dir(file)
}

// URLPath is file's path on the server of root: escaped, "/" for root,
// with a trailing "/" for a folder.
func URLPath(root, file string) string {
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == "." {
		return "/"
	}
	p := "/" + filepath.ToSlash(rel)
	if fi, err := os.Stat(file); err == nil && fi.IsDir() {
		p += "/"
	}
	return (&url.URL{Path: p}).EscapedPath()
}

// Base and Span are the ports servers take: Base plus a number from the
// folder's path, so a folder keeps its port across restarts (an open page
// reconnects); the next free one when it is taken.
const (
	Base = 43000
	Span = 1000
)

// PortFor is root's port.
func PortFor(root string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(root))
	return Base + int(h.Sum32()%Span)
}

// Info is a running server's record (~/.imux/web/<hash>.json).
type Info struct {
	Root  string `json:"root"`
	Port  int    `json:"port"`
	PID   int    `json:"pid"`
	Token string `json:"token"`
}

// URL is the address of path (URLPath) on the server.
func (i *Info) URL(p string) string { return "http://localhost:" + strconv.Itoa(i.Port) + p }

func recordFile(root string) string {
	h := sha256.Sum256([]byte(root))
	return filepath.Join(paths.Web(), hex.EncodeToString(h[:6])+".json")
}

func readRecord(file string) *Info {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var i Info
	if json.Unmarshal(b, &i) != nil || i.Port == 0 {
		return nil
	}
	return &i
}

// TokenHeader carries a server's token on the CLI's requests: no page can
// send it (it is not a header a page may set without the server's CORS).
const TokenHeader = "X-Imux-Token"

var client = &http.Client{Timeout: 2 * time.Second}

func (i *Info) call(ctx context.Context, method, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:"+strconv.Itoa(i.Port)+path, nil)
	if err != nil {
		return err
	}
	req.Host = "localhost"
	req.Header.Set(TokenHeader, i.Token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: %s", method, path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// Status is what a server says about itself.
type Status struct {
	Root  string `json:"root"`
	PID   int    `json:"pid"`
	Pages int    `json:"pages"`           // pages open
	Shown bool   `json:"shown,omitempty"` // Show: an open page shows it
	Title string `json:"title,omitempty"` // Show: that page's title
}

// Ping asks the server how it is; an error means it does not run.
func (i *Info) Ping(ctx context.Context) (Status, error) {
	var st Status
	err := i.call(ctx, http.MethodGet, "/__imux/ping", &st)
	if err == nil && st.Root != i.Root {
		err = errors.New("another folder's server")
	}
	return st, err
}

// Show makes the page last opened on the server show path; not Shown when
// no page is open (the browser must open it).
func (i *Info) Show(ctx context.Context, path string) (Status, error) {
	var st Status
	err := i.call(ctx, http.MethodPost, "/__imux/open?path="+url.QueryEscape(path), &st)
	return st, err
}

// Find returns root's running server, nil when there is none.
func Find(ctx context.Context, root string) *Info {
	i := readRecord(recordFile(root))
	if i == nil || i.Root != root {
		return nil
	}
	if _, err := i.Ping(ctx); err != nil {
		return nil
	}
	return i
}

// Ensure returns root's server, starting `self __web-serve root` (detached,
// its errors in ~/.imux/web/serve.log) when none runs.
func Ensure(ctx context.Context, self, root string) (*Info, error) {
	if i := Find(ctx, root); i != nil {
		return i, nil
	}
	if err := paths.EnsureDir(paths.Web()); err != nil {
		return nil, err
	}
	logf, err := os.OpenFile(filepath.Join(paths.Web(), "serve.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, paths.FileMode)
	if err != nil {
		return nil, err
	}
	defer logf.Close()
	cmd, err := sysx.StartDetachedCmd(func() *exec.Cmd {
		c := exec.Command(self, "__web-serve", root)
		c.Stdin, c.Stdout, c.Stderr = nil, nil, logf
		return c
	})
	if err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			return nil, fmt.Errorf("the server stopped: %v (see %s)", err, paths.Shorten(logf.Name()))
		case <-time.After(30 * time.Millisecond):
		}
		if i := readRecord(recordFile(root)); i != nil && i.PID == pid {
			if _, err := i.Ping(ctx); err == nil {
				return i, nil
			}
		}
	}
	_ = cmd.Process.Kill()
	return nil, errors.New("the server did not start")
}

// All lists the running servers.
func All(ctx context.Context) []*Info {
	files, _ := filepath.Glob(filepath.Join(paths.Web(), "*.json"))
	var out []*Info
	for _, f := range files {
		if i := readRecord(f); i != nil {
			if _, err := i.Ping(ctx); err == nil {
				out = append(out, i)
				continue
			}
			_ = os.Remove(f) // a server that died
		}
	}
	return out
}

// Stop ends the server and removes its record.
func (i *Info) Stop() {
	if i.PID > 0 {
		_ = sysx.Signal(i.PID, sysx.SIGTERM)
	}
	_ = os.Remove(recordFile(i.Root))
}

// RemoveOld deletes what v0.44's terminal browser (Carbonyl, D44) left in
// ~/.imux/tools.
func RemoveOld() {
	for _, pat := range []string{"carbonyl-*", ".carbonyl-*", "carbonyl.*.part"} {
		old, _ := filepath.Glob(filepath.Join(paths.Tools(), pat))
		for _, p := range old {
			_ = os.RemoveAll(p)
		}
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = io.ReadFull(rand.Reader, b)
	return hex.EncodeToString(b)
}

// inject puts the live reload script into a page: before </body>, else
// </html>, else at its end.
func inject(page []byte) []byte {
	tag := []byte(`<script src="/__imux/live.js"></script>`)
	lower := bytes.ToLower(page)
	for _, end := range []string{"</body>", "</html>"} {
		if i := bytes.LastIndex(lower, []byte(end)); i >= 0 {
			return append(append(append([]byte{}, page[:i]...), tag...), page[i:]...)
		}
	}
	return append(append([]byte{}, page...), tag...)
}
