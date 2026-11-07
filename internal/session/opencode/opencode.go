// Package opencode reads OpenCode's sessions (ARCHITECTURE §7.7): it lists
// them from OpenCode's SQLite database with the sqlite3 CLI and loads one
// through `opencode session export`, OpenCode's own JSON.
package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

func init() {
	session.Register(session.OpenCode, func(env session.Env) session.Provider { return &Provider{Env: env} })
}

// Provider reads OpenCode's store.
type Provider struct {
	Env session.Env
}

// Agent implements session.Provider.
func (p *Provider) Agent() session.AgentID { return session.OpenCode }

// DB is the path of OpenCode's database.
func (p *Provider) DB() string { return filepath.Join(p.Env.Resolve(session.OpenCode), "opencode.db") }

// Timeout bounds every external command.
var Timeout = 10 * time.Second

// LookPath is exec.LookPath; tests replace it.
var LookPath = exec.LookPath

// bin is the opencode binary (Env.OpenCodeBin, else the PATH).
func (p *Provider) bin() string {
	if p.Env.OpenCodeBin != "" {
		return p.Env.OpenCodeBin
	}
	if b, err := LookPath("opencode"); err == nil {
		return b
	}
	return ""
}

// row is one session_v2 row.
type row struct {
	ID          string `json:"id"`
	Directory   string `json:"directory"`
	Title       string `json:"title"`
	TimeCreated int64  `json:"time_created"`
	TimeUpdated int64  `json:"time_updated"`
}

// query runs a read-only SQL query with the sqlite3 CLI (JSON rows).
func (p *Provider) query(ctx context.Context, sql string, args ...string) ([]byte, error) {
	sq, err := LookPath("sqlite3")
	if err != nil {
		return nil, errors.New("sqlite3 not installed")
	}
	db := p.DB()
	if _, err := os.Stat(db); err != nil {
		return nil, err
	}
	for _, a := range args {
		sql = strings.Replace(sql, "?", "'"+strings.ReplaceAll(a, "'", "''")+"'", 1)
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	// file: URI with mode=ro: never writes, never takes OpenCode's lock.
	out, err := exec.CommandContext(ctx, sq, "-readonly", "-json", "file:"+db+"?mode=ro", sql).Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Find implements session.Provider: top-level sessions, newest first.
func (p *Provider) Find(ctx context.Context, q session.Query) ([]session.Ref, error) {
	sql := "select id, directory, title, time_created, time_updated from session_v2 where parent_id is null and time_archived is null"
	var args []string
	if q.ID != "" {
		sql += " and id = ?"
		args = append(args, q.ID)
	}
	sql += " order by time_updated desc limit 500"
	out, err := p.query(ctx, sql, args...)
	if err != nil {
		if q.ID != "" && strings.HasPrefix(q.ID, "ses") && p.bin() != "" {
			// Without sqlite3 a known id is still loadable.
			return []session.Ref{{Agent: session.OpenCode, ID: q.ID, Path: p.DB(), CWD: q.CWD}}, nil
		}
		return nil, nil
	}
	var rows []row
	if len(bytes.TrimSpace(out)) > 0 {
		if err := json.Unmarshal(out, &rows); err != nil {
			return nil, fmt.Errorf("opencode: %w", err)
		}
	}
	var refs []session.Ref
	for _, r := range rows {
		if q.CWD != "" && !session.SameDir(r.Directory, q.CWD) {
			continue
		}
		upd := time.UnixMilli(r.TimeUpdated)
		if !q.Since.IsZero() && upd.Before(q.Since) {
			continue
		}
		refs = append(refs, session.Ref{
			Agent: session.OpenCode, ID: r.ID, Path: p.DB(), CWD: r.Directory,
			StartedAt: time.UnixMilli(r.TimeCreated), UpdatedAt: upd,
		})
		if q.Limit > 0 && len(refs) >= q.Limit {
			break
		}
	}
	return refs, nil
}

// export is the JSON of `opencode session export`.
type export struct {
	Info struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Model struct {
			ID         string `json:"id"`
			ProviderID string `json:"providerID"`
		} `json:"model"`
		Tokens struct {
			Input  int64 `json:"input"`
			Output int64 `json:"output"`
			Cache  struct {
				Read  int64 `json:"read"`
				Write int64 `json:"write"`
			} `json:"cache"`
		} `json:"tokens"`
		Time struct {
			Created int64 `json:"created"`
			Updated int64 `json:"updated"`
		} `json:"time"`
		Location struct {
			Directory string `json:"directory"`
		} `json:"location"`
	} `json:"info"`
	Messages []message `json:"messages"`
}

type message struct {
	Type string `json:"type"`
	Time struct {
		Created int64 `json:"created"`
	} `json:"time"`
	Text    string `json:"text"`
	Content []struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		Name  string `json:"name"`
		State struct {
			Status string         `json:"status"`
			Input  map[string]any `json:"input"`
		} `json:"state"`
	} `json:"content"`
	Model struct {
		ID         string `json:"id"`
		ProviderID string `json:"providerID"`
	} `json:"model"`
	Error json.RawMessage `json:"error"`
}

// Export runs `opencode session export --standalone id`.
func (p *Provider) Export(ctx context.Context, id string) ([]byte, error) {
	b := p.bin()
	if b == "" {
		return nil, errors.New("opencode not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, b, "session", "export", "--standalone", id)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("opencode session export: %s", msg)
	}
	return out, nil
}

// argAliases map OpenCode's argument names to the summarizer's.
var argAliases = map[string]string{"filePath": "file_path", "path": "file_path"}

// Load implements session.Provider.
func (p *Provider) Load(ctx context.Context, ref session.Ref, opt session.LoadOptions) (*session.Session, error) {
	raw, err := p.Export(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	s, err := Parse(raw, ref)
	if err != nil {
		return nil, err
	}
	s.FilterSince(opt.Since)
	return s, nil
}

// Parse turns an export into a session.
func Parse(raw []byte, ref session.Ref) (*session.Session, error) {
	var ex export
	if err := json.Unmarshal(raw, &ex); err != nil {
		return nil, fmt.Errorf("opencode: export: %w", err)
	}
	ref.Agent = session.OpenCode
	if ref.ID == "" {
		ref.ID = ex.Info.ID
	}
	if ex.Info.Location.Directory != "" {
		ref.CWD = ex.Info.Location.Directory
	}
	if ex.Info.Time.Created > 0 {
		ref.StartedAt = time.UnixMilli(ex.Info.Time.Created)
	}
	if ex.Info.Time.Updated > 0 {
		ref.UpdatedAt = time.UnixMilli(ex.Info.Time.Updated)
	}
	s := &session.Session{Ref: ref}
	if ex.Info.Model.ID != "" {
		s.Model = modelName(ex.Info.Model.ProviderID, ex.Info.Model.ID)
	}
	s.Usage.InputTokens = ex.Info.Tokens.Input
	s.Usage.OutputTokens = ex.Info.Tokens.Output
	s.Usage.CacheReadTokens = ex.Info.Tokens.Cache.Read
	s.Usage.CacheWriteTokens = ex.Info.Tokens.Cache.Write
	first := true
	for _, m := range ex.Messages {
		at := time.UnixMilli(m.Time.Created)
		switch m.Type {
		case "user":
			e := session.Entry{Role: session.RoleUser, Kind: session.KindMessage, At: at, Text: m.Text}
			if first && session.IsHandoff(m.Text) {
				e.Kind = session.KindHandoff
				s.Chained = session.ParseChained(m.Text)
			}
			first = false
			s.Entries = append(s.Entries, e)
		case "assistant":
			s.Usage.Replies++
			if m.Model.ID != "" {
				s.Model = modelName(m.Model.ProviderID, m.Model.ID)
			}
			e := session.Entry{Role: session.RoleAssistant, Kind: session.KindMessage, At: at}
			var texts []string
			for _, c := range m.Content {
				switch c.Type {
				case "text":
					if t := strings.TrimSpace(c.Text); t != "" {
						texts = append(texts, t)
					}
				case "tool":
					args := map[string]any{}
					for k, v := range c.State.Input {
						args[k] = v
						if a, ok := argAliases[k]; ok {
							if _, taken := c.State.Input[a]; !taken {
								args[a] = v
							}
						}
					}
					e.Tools = append(e.Tools, session.ToolCall{Name: c.Name, Args: args, Failed: c.State.Status == "error"})
					if strings.EqualFold(c.Name, "todowrite") {
						if todos := parseTodos(c.State.Input); todos != nil {
							s.Todos = todos
						}
					}
				}
			}
			e.Text = strings.Join(texts, "\n\n")
			if len(m.Error) > 0 && string(m.Error) != "null" {
				s.Errors = append(s.Errors, session.APIError{At: at, Code: "error", Text: errorText(m.Error)})
			}
			if e.Text != "" || len(e.Tools) > 0 {
				s.Entries = append(s.Entries, e)
			}
		}
	}
	return s, nil
}

func modelName(provider, id string) string {
	if provider == "" {
		return id
	}
	return provider + "/" + id
}

// errorText reads an error object's message ({message} or {data:{message}}).
func errorText(raw json.RawMessage) string {
	var e struct {
		Name    string `json:"name"`
		Message string `json:"message"`
		Data    struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return string(raw)
	}
	for _, t := range []string{e.Message, e.Data.Message, e.Name} {
		if t != "" {
			return t
		}
	}
	return string(raw)
}

// parseTodos reads todowrite's {todos: [{content, status}]}.
func parseTodos(in map[string]any) []session.Todo {
	list, ok := in["todos"].([]any)
	if !ok {
		return nil
	}
	var out []session.Todo
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		text, _ := m["content"].(string)
		st, _ := m["status"].(string)
		if text != "" {
			out = append(out, session.Todo{Text: text, Status: session.ParseTodoStatus(st)})
		}
	}
	return out
}

// Preview implements session.Previewer: the first user message, from the
// database (cheap) or the export.
func (p *Provider) Preview(ctx context.Context, ref session.Ref) (string, error) {
	out, err := p.query(ctx, "select data from session_message where session_id = ? and type = 'user' order by seq limit 1", ref.ID)
	if err == nil {
		var rows []struct {
			Data string `json:"data"`
		}
		if json.Unmarshal(out, &rows) == nil && len(rows) == 1 {
			var d struct {
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(rows[0].Data), &d) == nil {
				if c := session.ParseChained(d.Text); c != nil && c.OriginalRequest != "" {
					return c.OriginalRequest, nil
				}
				return d.Text, nil
			}
		}
	}
	s, err := p.Load(ctx, ref, session.LoadOptions{})
	if err != nil {
		return "", err
	}
	return s.FirstUserText(), nil
}

// Tokens sums the tokens of the sessions updated since t (limits panel).
func (p *Provider) Tokens(ctx context.Context, since time.Time) (in, out int64, n int, err error) {
	q, err := p.query(ctx, "select coalesce(sum(tokens_input),0) as i, coalesce(sum(tokens_output),0) as o, count(*) as n from session_v2 where time_updated >= ?", strconv.FormatInt(since.UnixMilli(), 10))
	if err != nil {
		return 0, 0, 0, err
	}
	var rows []struct {
		I, O int64
		N    int
	}
	if json.Unmarshal(q, &rows) != nil || len(rows) == 0 {
		return 0, 0, 0, errors.New("opencode: tokens")
	}
	return rows[0].I, rows[0].O, rows[0].N, nil
}
