package limits

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/jsonl"
)

// Tokens are summed usage counters.
type Tokens struct {
	Input, Output, CacheRead, CacheWrite int64
}

// ClaudeWindow is Claude's usage within a rolling window (the 5-hour limit).
type ClaudeWindow struct {
	From, To time.Time
	Replies  int // assistant API messages, deduped by message.id
	Tokens
	Files int // session files read
}

var (
	tsKey    = []byte(`"timestamp":"`)
	usageKey = []byte(`"usage"`)
)

type claudeUsageLine struct {
	Message struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// lineTime reads the top-level "timestamp" without decoding the line.
func lineTime(line []byte) (time.Time, bool) {
	i := bytes.Index(line, tsKey)
	if i < 0 {
		return time.Time{}, false
	}
	rest := line[i+len(tsKey):]
	j := bytes.IndexByte(rest, '"')
	if j < 0 {
		return time.Time{}, false
	}
	t := session.ParseTime(string(rest[:j]))
	return t, !t.IsZero()
}

// ClaudeUsage sums Claude's usage over [now-window, now] from the session
// files modified in that window. Each file is read backwards until its
// timestamps leave the window; usage is counted once per message.id.
func ClaudeUsage(env session.Env, now time.Time, window time.Duration) ClaudeWindow {
	from := now.Add(-window)
	w := ClaudeWindow{From: from, To: now}
	root := filepath.Join(env.Resolve(session.Claude), "projects")
	dirs, err := os.ReadDir(root)
	if err != nil {
		return w
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		for _, f := range session.ListFiles(filepath.Join(root, d.Name()), session.HasSuffix(".jsonl")) {
			if f.ModTime.Before(from) {
				continue
			}
			w.Files++
			scanClaudeFile(f.Path, from, now, seen, &w)
		}
	}
	return w
}

func scanClaudeFile(path string, from, to time.Time, seen map[string]bool, w *ClaudeWindow) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return
	}
	rv := jsonl.NewReverse(f, fi.Size(), 0)
	for rv.Scan() {
		line := rv.Bytes()
		t, ok := lineTime(line)
		if !ok {
			continue
		}
		if t.Before(from) {
			return // older lines are outside the window
		}
		if t.After(to) || !bytes.Contains(line, usageKey) {
			continue
		}
		var l claudeUsageLine
		if json.Unmarshal(line, &l) != nil || l.Message.Usage == nil || l.Message.Model == "<synthetic>" {
			continue
		}
		id := l.Message.ID
		if id != "" {
			if seen[id] {
				continue
			}
			seen[id] = true
		}
		u := l.Message.Usage
		w.Replies++
		w.Input += u.Input
		w.Output += u.Output
		w.CacheRead += u.CacheRead
		w.CacheWrite += u.CacheWrite
	}
}
