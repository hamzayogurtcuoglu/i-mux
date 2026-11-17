// Package aider reads Aider's conversations (ARCHITECTURE §7.8, D81): the
// Markdown chat history Aider appends to in the repository's root,
// `.aider.chat.history.md`. Every run of Aider starts a part with
// "# aider chat started at <local time>"; that part is one session, named by
// its start time.
package aider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

func init() {
	session.Register(session.Aider, func(env session.Env) session.Provider { return &Provider{Env: env} })
}

// HistoryName is the chat history file Aider writes by default.
const HistoryName = ".aider.chat.history.md"

// IDLayout names a session by its start time.
const IDLayout = "20060102T150405"

// maxRead bounds how much of a history file is read: its end, where the
// newest runs are.
var maxRead int64 = 32 << 20

const (
	headerPrefix = "# aider chat started at "
	headerLayout = "2006-01-02 15:04:05"
)

// Provider reads Aider's chat histories.
type Provider struct {
	Env session.Env
}

// Agent implements session.Provider.
func (p *Provider) Agent() session.AgentID { return session.Aider }

// HistoryFile is the chat history that Aider started in dir writes: dir's
// own, else the one at the root of dir's git repository ("" when neither
// exists).
func HistoryFile(dir string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	for d := dir; ; {
		if fi, err := os.Stat(filepath.Join(d, HistoryName)); err == nil && fi.Mode().IsRegular() {
			return filepath.Join(d, HistoryName)
		}
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return "" // the repository's root: Aider writes nowhere higher
		}
		up := filepath.Dir(d)
		if up == d {
			return ""
		}
		d = up
	}
}

// run is one part of a history file.
type run struct {
	start      time.Time
	begin, end int // byte offsets in the data read
}

// read returns the end of file (at most maxRead bytes, starting at a line)
// and its modification time.
func read(file string) ([]byte, os.FileInfo, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	off := int64(0)
	if fi.Size() > maxRead {
		off = fi.Size() - maxRead
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, nil, err
	}
	b, err := io.ReadAll(io.LimitReader(f, maxRead))
	if err != nil {
		return nil, nil, err
	}
	if off > 0 {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return b, fi, nil
}

// runs splits a history into its runs, oldest first.
func runs(b []byte) []run {
	var out []run
	for off := 0; off < len(b); {
		end := bytes.IndexByte(b[off:], '\n')
		next := len(b)
		if end >= 0 {
			next = off + end + 1
		}
		line := strings.TrimRight(string(b[off:next]), "\r\n")
		if strings.HasPrefix(line, headerPrefix) {
			if t, err := time.ParseInLocation(headerLayout, strings.TrimSpace(strings.TrimPrefix(line, headerPrefix)), time.Local); err == nil {
				if n := len(out); n > 0 {
					out[n-1].end = off
				}
				out = append(out, run{start: t, begin: off, end: len(b)})
			}
		}
		off = next
	}
	return out
}

// refs turns a history file's runs into refs, newest first.
func refs(file string, b []byte, fi os.FileInfo) []session.Ref {
	rs := runs(b)
	out := make([]session.Ref, 0, len(rs))
	for i := len(rs) - 1; i >= 0; i-- {
		r := rs[i]
		upd := fi.ModTime()
		if i+1 < len(rs) {
			upd = rs[i+1].start
		}
		if upd.Before(r.start) {
			upd = r.start
		}
		out = append(out, session.Ref{
			Agent: session.Aider, ID: r.start.Format(IDLayout), Path: file, CWD: filepath.Dir(file),
			StartedAt: r.start, UpdatedAt: upd, Size: int64(r.end - r.begin),
		})
	}
	return out
}

// Find implements session.Provider. Aider keeps no list of its
// conversations: they are found for a folder (q.CWD) only.
func (p *Provider) Find(_ context.Context, q session.Query) ([]session.Ref, error) {
	file := HistoryFile(q.CWD)
	if file == "" {
		return nil, nil
	}
	b, fi, err := read(file)
	if err != nil {
		return nil, nil
	}
	var out []session.Ref
	for _, r := range refs(file, b, fi) {
		if q.ID != "" && r.ID != q.ID {
			continue
		}
		if !q.Since.IsZero() && r.UpdatedAt.Before(q.Since) {
			continue
		}
		out = append(out, r)
		if q.Limit > 0 && len(out) == q.Limit {
			break
		}
	}
	return out, nil
}

// Load implements session.Provider: the run ref.ID of ref.Path (the newest
// run when the id is empty).
func (p *Provider) Load(_ context.Context, ref session.Ref, opt session.LoadOptions) (*session.Session, error) {
	opt = opt.WithDefaults()
	file := ref.Path
	if file == "" {
		file = HistoryFile(ref.CWD)
	}
	if file == "" {
		return nil, errors.New("aider: no chat history")
	}
	b, fi, err := read(file)
	if err != nil {
		return nil, err
	}
	rs := runs(b)
	all := refs(file, b, fi)
	for i, r := range all {
		if ref.ID != "" && r.ID != ref.ID {
			continue
		}
		part := rs[len(rs)-1-i]
		s := Parse(b[part.begin:part.end], r, opt)
		s.FilterSince(opt.Since)
		return s, nil
	}
	return nil, errors.New("aider: no run " + ref.ID + " in " + file)
}

// Preview implements session.Previewer.
func (p *Provider) Preview(ctx context.Context, ref session.Ref) (string, error) {
	s, err := p.Load(ctx, ref, session.LoadOptions{})
	if err != nil {
		return "", err
	}
	return s.FirstUserText(), nil
}

var (
	reModel   = regexp.MustCompile(`^(?:Main )?[Mm]odel: (\S+)`)
	reApplied = regexp.MustCompile(`^Applied edit to (.+?)\s*$`)
	reAdded   = regexp.MustCompile(`^Added (.+?) to the chat\b`)
	reCommit  = regexp.MustCompile(`^Commit ([0-9a-f]{7,40}) (.+?)\s*$`)
	reRunning = regexp.MustCompile(`^Running (.+?)\s*$`)
	reTokens  = regexp.MustCompile(`^Tokens: ([\d.]+[kKmM]?) sent,(?:.*?)\s([\d.]+[kKmM]?) received`)
	reError   = regexp.MustCompile(`(?i)\berror\b|Error:|exception`)
)

// Parse reads one run (from its header on) into a session. It never fails:
// lines it does not know are the assistant's text. A run longer than the
// head and tail windows keeps both ends (Session.Gap).
func Parse(part []byte, ref session.Ref, opt session.LoadOptions) *session.Session {
	opt = opt.WithDefaults()
	s := &session.Session{Ref: ref}
	if int64(len(part)) > opt.HeadBytes+opt.TailBytes {
		head := part[:opt.HeadBytes]
		if i := bytes.LastIndexByte(head, '\n'); i >= 0 {
			head = head[:i+1]
		}
		tail := part[int64(len(part))-opt.TailBytes:]
		if i := bytes.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:]
		}
		part = append(append(append([]byte{}, head...), '\n'), tail...)
		s.Gap = true
	}
	var (
		cur    *session.Entry // the entry being read (user or assistant)
		notice []string       // Aider's own output being read
		first  = true
	)
	flush := func() {
		if cur == nil {
			return
		}
		cur.Text = strings.TrimSpace(cur.Text)
		if cur.Role == session.RoleUser {
			switch {
			case first && session.IsHandoff(cur.Text):
				cur.Kind = session.KindHandoff
				s.Chained = session.ParseChained(cur.Text)
			case strings.HasPrefix(cur.Text, "/"):
				cur.Kind = session.KindSlashCommand
			}
			first = false
		}
		if cur.Text != "" || len(cur.Tools) > 0 {
			s.Entries = append(s.Entries, *cur)
		}
		cur = nil
	}
	tool := func(name string, args map[string]any) {
		if cur == nil || cur.Role != session.RoleAssistant {
			flush()
			cur = &session.Entry{Role: session.RoleAssistant}
		}
		cur.Tools = append(cur.Tools, session.ToolCall{Name: name, Args: args})
	}
	endNotice := func() {
		if len(notice) == 0 {
			return
		}
		text := strings.TrimSpace(strings.Join(notice, "\n"))
		notice = nil
		line, _, _ := strings.Cut(text, "\n")
		switch {
		case strings.HasPrefix(line, "Aider v"):
		case reModel.MatchString(line):
			s.Model = reModel.FindStringSubmatch(line)[1]
		case reApplied.MatchString(line):
			tool("edit", map[string]any{"file_path": reApplied.FindStringSubmatch(line)[1]})
		case reAdded.MatchString(line):
			if f := reAdded.FindStringSubmatch(line)[1]; !session.IsHandoffFile(f) {
				tool("read", map[string]any{"file_path": f})
			}
		case reCommit.MatchString(line):
			m := reCommit.FindStringSubmatch(line)
			tool("commit", map[string]any{"message": m[1][:7] + " " + m[2]})
		case reRunning.MatchString(line):
			tool("bash", map[string]any{"command": reRunning.FindStringSubmatch(line)[1]})
		case reTokens.MatchString(line):
			m := reTokens.FindStringSubmatch(line)
			s.Usage.InputTokens += tokens(m[1])
			s.Usage.OutputTokens += tokens(m[2])
			s.Usage.Replies++
		default:
			if e := (session.APIError{Code: "error", Text: text}); e.LimitLike() || reError.MatchString(line) {
				s.Errors = append(s.Errors, e)
			}
		}
	}
	for _, raw := range strings.Split(string(part), "\n") {
		l := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(l, headerPrefix):
		case l == "####" || strings.HasPrefix(l, "#### "):
			endNotice()
			if cur == nil || cur.Role != session.RoleUser {
				flush()
				cur = &session.Entry{Role: session.RoleUser}
			} else {
				cur.Text += "\n"
			}
			cur.Text += strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(l, "####"), " "), " ")
		case l == ">" || strings.HasPrefix(l, "> "):
			endNotice()
			if cur != nil && cur.Role == session.RoleUser {
				flush()
			}
			notice = []string{strings.TrimPrefix(strings.TrimPrefix(l, ">"), " ")}
		case strings.TrimSpace(l) == "":
			endNotice()
			if cur != nil && cur.Role == session.RoleAssistant {
				cur.Text += "\n"
			}
		case len(notice) > 0:
			notice = append(notice, l)
		default:
			if cur == nil || cur.Role != session.RoleAssistant {
				flush()
				cur = &session.Entry{Role: session.RoleAssistant}
			}
			cur.Text += l + "\n"
		}
	}
	endNotice()
	flush()
	return s
}

// tokens reads Aider's token counts: "512", "2.3k", "12k", "1.1M".
func tokens(s string) int64 {
	mul := 1.0
	switch {
	case strings.HasSuffix(s, "k"), strings.HasSuffix(s, "K"):
		mul, s = 1e3, s[:len(s)-1]
	case strings.HasSuffix(s, "m"), strings.HasSuffix(s, "M"):
		mul, s = 1e6, s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f*mul + 0.5)
}
