// Package tmux runs tmux commands on ajan's private socket (`tmux -L ajan`).
//
// Several commands are batched into one exec with `;` separators, and their
// output is split with delimiter lines (Sections). Failures are classified
// into typed errors (no server, no session, no pane, no client).
//
// Setting AJAN_TMUX_RECORD=<file> appends every invocation to that file as one
// JSON line; tests use it to assert which tmux commands ajan ran.
package tmux

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultSocket is the socket name used with `tmux -L`.
const DefaultSocket = "ajan"

// Typed errors. Use errors.Is.
var (
	ErrNoServer  = errors.New("tmux: no server running")
	ErrNoSession = errors.New("tmux: session not found")
	ErrNoPane    = errors.New("tmux: pane not found")
	ErrNoClient  = errors.New("tmux: client not found")
	ErrNotFound  = errors.New("tmux: binary not found")
)

// Error is a failed tmux invocation.
type Error struct {
	Args   []string
	Stderr string
	Kind   error // one of the typed errors, or nil
	Err    error // the exec error
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	if e.Kind != nil && msg == "" {
		return e.Kind.Error()
	}
	return "tmux: " + msg
}

// Unwrap returns the typed error, so errors.Is works.
func (e *Error) Unwrap() error { return e.Kind }

// classify maps tmux's stderr to a typed error.
func classify(stderr string) error {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "no server running"),
		strings.Contains(s, "error connecting to"),
		strings.Contains(s, "server exited"),
		strings.Contains(s, "lost server"):
		return ErrNoServer
	case strings.Contains(s, "can't find session"), strings.Contains(s, "session not found"),
		strings.Contains(s, "no such session"):
		return ErrNoSession
	case strings.Contains(s, "can't find pane"), strings.Contains(s, "no such pane"),
		strings.Contains(s, "can't find window"), strings.Contains(s, "no such window"):
		return ErrNoPane
	case strings.Contains(s, "can't find client"), strings.Contains(s, "no current client"):
		return ErrNoClient
	}
	return nil
}

// Runner executes the tmux binary with args. Tests use a fake.
type Runner interface {
	Run(ctx context.Context, bin string, args []string) (stdout, stderr []byte, err error)
}

// ExecRunner runs tmux as a child process.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, bin string, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Stdin = nil
	err := cmd.Run()
	return out.Bytes(), errb.Bytes(), err
}

// Cmd is one tmux command with its arguments, e.g. {"set", "-p", "@x", "1"}.
type Cmd []string

// Client talks to one tmux server.
type Client struct {
	Bin     string // tmux binary ("tmux")
	Socket  string // -L name
	Config  string // -f file used when the server starts; "" = tmux's default
	Runner  Runner
	Record  string        // AJAN_TMUX_RECORD file; "" = off
	Timeout time.Duration // per exec; 0 = 5 s

	recMu sync.Mutex
}

// Socket returns the socket name: $AJAN_TMUX_SOCKET or "ajan".
func Socket() string {
	if s := os.Getenv("AJAN_TMUX_SOCKET"); s != "" {
		return s
	}
	return DefaultSocket
}

// New returns a client for ajan's socket with config file conf.
func New(conf string) *Client {
	bin := os.Getenv("AJAN_TMUX_BIN")
	if bin == "" {
		bin = "tmux"
	}
	return &Client{
		Bin:    bin,
		Socket: Socket(),
		Config: conf,
		Runner: ExecRunner{},
		Record: os.Getenv("AJAN_TMUX_RECORD"),
	}
}

// BaseArgs returns the arguments that select the socket and config.
func (c *Client) BaseArgs() []string {
	a := []string{"-L", c.Socket}
	if c.Config != "" {
		a = append(a, "-f", c.Config)
	}
	return a
}

// Argv returns the full argument vector (without the binary) for cmds,
// joined with ";" separators.
func (c *Client) Argv(cmds ...Cmd) []string {
	args := c.BaseArgs()
	for i, cmd := range cmds {
		if i > 0 {
			args = append(args, ";")
		}
		for _, a := range cmd {
			args = append(args, EscapeArg(a))
		}
	}
	return args
}

// EscapeArg protects an argument from being read as a command separator:
// tmux strips a trailing ";" from an argument and ends the command there, so
// it is sent as `\;`, which tmux turns back into a literal ";".
func EscapeArg(a string) string {
	if strings.HasSuffix(a, ";") {
		return a[:len(a)-1] + `\;`
	}
	return a
}

// Run executes cmds in one tmux invocation and returns stdout.
func (c *Client) Run(ctx context.Context, cmds ...Cmd) (string, error) {
	args := c.Argv(cmds...)
	c.record(args)
	to := c.Timeout
	if to <= 0 {
		to = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	r := c.Runner
	if r == nil {
		r = ExecRunner{}
	}
	out, errb, err := r.Run(ctx, c.Bin, args)
	if err != nil {
		if notFound(err) {
			return "", &Error{Args: args, Kind: ErrNotFound, Err: err}
		}
		return string(out), &Error{Args: args, Stderr: string(errb), Kind: classify(string(errb)), Err: err}
	}
	return string(out), nil
}

// notFound reports whether err means the tmux binary could not be started.
func notFound(err error) bool {
	var ee *exec.Error
	return errors.As(err, &ee) || errors.Is(err, fs.ErrNotExist)
}

// Do is Run with a background context.
func (c *Client) Do(cmds ...Cmd) (string, error) { return c.Run(context.Background(), cmds...) }

// record appends one invocation to the recorder file.
func (c *Client) record(args []string) {
	if c.Record == "" {
		return
	}
	line, err := json.Marshal(struct {
		T    int64    `json:"t"`
		Pid  int      `json:"pid"`
		Args []string `json:"args"`
	}{time.Now().UnixMilli(), os.Getpid(), args})
	if err != nil {
		return
	}
	c.recMu.Lock()
	defer c.recMu.Unlock()
	f, err := os.OpenFile(c.Record, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// Recorded is one invocation read back from a recorder file.
type Recorded struct {
	T    int64    `json:"t"`
	Pid  int      `json:"pid"`
	Args []string `json:"args"`
}

// ReadRecord parses a recorder file.
func ReadRecord(path string) ([]Recorded, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Recorded
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" {
			continue
		}
		var r Recorded
		if err := json.Unmarshal([]byte(line), &r); err == nil {
			out = append(out, r)
		}
	}
	return out, nil
}

// --- server and environment ------------------------------------------------

// Running reports whether the server answers.
func (c *Client) Running(ctx context.Context) bool {
	_, err := c.Run(ctx, Cmd{"list-sessions", "-F", "#{session_name}"})
	return err == nil
}

// Inside reports whether the process runs inside a pane of this client's
// server, using $TMUX ("<socket path>,<pid>,<session index>").
func (c *Client) Inside(tmuxEnv string) bool {
	if tmuxEnv == "" {
		return false
	}
	sock, _, _ := strings.Cut(tmuxEnv, ",")
	return filepath.Base(sock) == c.Socket
}

// --- output parsing --------------------------------------------------------

// Delim starts a delimiter line printed between batched commands' output:
// `display -p "\x1eajan:<key>"`.
const Delim = "\x1eajan:"

// DelimCmd returns the command that prints the delimiter line for key.
func DelimCmd(key string) Cmd { return Cmd{"display-message", "-p", Delim + key} }

// Sections splits batched output at delimiter lines. Output before the first
// delimiter is returned under the key "".
func Sections(out string) map[string]string {
	res := map[string]string{}
	key := ""
	var b strings.Builder
	flush := func() {
		if prev, ok := res[key]; ok {
			res[key] = prev + b.String()
		} else if b.Len() > 0 || key != "" {
			res[key] = b.String()
		}
		b.Reset()
	}
	for len(out) > 0 {
		line, rest, found := strings.Cut(out, "\n")
		out = rest
		if k, ok := strings.CutPrefix(line, Delim); ok {
			flush()
			key = k
			continue
		}
		b.WriteString(line)
		if found {
			b.WriteByte('\n')
		}
	}
	flush()
	return res
}

// Rows splits format output into lines of tab-separated fields. Each row has
// exactly n fields: missing ones are "", extra tabs stay in the last field.
// Empty lines are skipped.
func Rows(out string, n int) [][]string {
	var rows [][]string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\t", n)
		for len(f) < n {
			f = append(f, "")
		}
		rows = append(rows, f)
	}
	return rows
}

// --- version ---------------------------------------------------------------

// Version is a parsed `tmux -V`.
type Version struct {
	Major, Minor int
	Suffix       string // "a" in 3.2a
	Raw          string
}

// AtLeast reports whether v ≥ major.minor.
func (v Version) AtLeast(major, minor int) bool {
	return v.Major > major || v.Major == major && v.Minor >= minor
}

func (v Version) String() string {
	if v.Raw != "" {
		return v.Raw
	}
	return fmt.Sprintf("%d.%d%s", v.Major, v.Minor, v.Suffix)
}

// ParseVersion parses `tmux -V` output: "tmux 3.2a", "tmux next-3.5",
// "tmux 3.8". Development builds ("tmux master", "tmux openbsd-7.6") count as
// newer than any release. It reports false if nothing was recognized.
func ParseVersion(s string) (Version, bool) {
	s = strings.TrimSpace(s)
	raw := strings.TrimPrefix(s, "tmux ")
	v := Version{Raw: raw}
	if raw == "" {
		return v, false
	}
	if raw == "master" || strings.HasPrefix(raw, "openbsd-") {
		v.Major, v.Minor = 99, 0
		return v, true
	}
	t := strings.TrimPrefix(raw, "next-")
	maj, rest, ok := strings.Cut(t, ".")
	if !ok {
		return v, false
	}
	m, err := strconv.Atoi(maj)
	if err != nil {
		return v, false
	}
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i == 0 {
		return v, false
	}
	n, err := strconv.Atoi(rest[:i])
	if err != nil {
		return v, false
	}
	v.Major, v.Minor, v.Suffix = m, n, rest[i:]
	if strings.HasPrefix(raw, "next-") {
		v.Suffix = "-next"
	}
	return v, true
}

// BinVersion runs `tmux -V`.
func (c *Client) BinVersion(ctx context.Context) (Version, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	r := c.Runner
	if r == nil {
		r = ExecRunner{}
	}
	out, errb, err := r.Run(ctx, c.Bin, []string{"-V"})
	if err != nil {
		if notFound(err) {
			return Version{}, ErrNotFound
		}
		return Version{}, &Error{Args: []string{"-V"}, Stderr: string(errb), Err: err}
	}
	v, ok := ParseVersion(string(out))
	if !ok {
		return v, fmt.Errorf("tmux: unrecognized version %q", strings.TrimSpace(string(out)))
	}
	return v, nil
}

// --- quoting for tmux command strings -----------------------------------------

// Quote returns s as one word of a tmux command string (menu items, bindings
// and hooks are parsed by tmux's command parser). Inside single quotes
// nothing is expanded; a single quote itself is written as '\” (close,
// escaped quote, reopen).
func Quote(s string) string {
	if s != "" && isSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ShQuote quotes s for sh (run-shell passes its command to /bin/sh -c).
func ShQuote(s string) string {
	if s != "" && isSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// ShJoin quotes and joins argv for sh.
func ShJoin(argv ...string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = ShQuote(a)
	}
	return strings.Join(q, " ")
}

func isSafe(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_-./:%@=,+", r):
		default:
			return false
		}
	}
	return true
}

// EscapeFormat makes text safe inside a tmux format (menu item names, status
// lines, display-message): "#" starts a format, so it is doubled.
func EscapeFormat(s string) string { return strings.ReplaceAll(s, "#", "##") }
