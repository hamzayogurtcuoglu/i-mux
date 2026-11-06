// Package antigravity reads what imux can use from Antigravity CLI (agy),
// Google's successor to Gemini CLI for personal accounts.
//
// Antigravity keeps conversations as undocumented protobuf files, so imux
// does not parse them: a handoff from an Antigravity pane is rebuilt from the
// pane's scrollback (ARCHITECTURE §7.6). What Antigravity does keep in plain
// text is the agent's plan: brain/<conversation-id>/task.md, a markdown
// checklist. Todos returns it so the handoff still carries the task list.
package antigravity

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// BrainDirs are the directories, relative to the user's home, that may hold
// brain/<id>/task.md: the CLI's own and the one the Antigravity app uses.
var BrainDirs = []string{
	filepath.Join(".gemini", "antigravity-cli", "brain"),
	filepath.Join(".gemini", "antigravity", "brain"),
}

// maxTodos bounds the list taken from one task.md.
const maxTodos = 50

// checklist matches "- [ ] text", "* [x] text", "1. [/] text".
var checklist = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+\[([ xX/~-])\]\s+(.+?)\s*$`)

// Todos returns the checklist of the newest task.md under home's brain
// directories that changed at or after since, or nil when there is none.
func Todos(home string, since time.Time) []session.Todo {
	var newest string
	var newestTime time.Time
	for _, d := range BrainDirs {
		matches, _ := filepath.Glob(filepath.Join(home, d, "*", "task.md"))
		for _, m := range matches {
			fi, err := os.Stat(m)
			if err != nil || fi.IsDir() || fi.ModTime().Before(since) {
				continue
			}
			if newest == "" || fi.ModTime().After(newestTime) {
				newest, newestTime = m, fi.ModTime()
			}
		}
	}
	if newest == "" {
		return nil
	}
	return ParseTaskList(newest)
}

// ParseTaskList reads a markdown checklist. "[x]" is done, "[/]" (how
// Antigravity marks the current step) in progress, "[-]" and "[~]" blocked,
// "[ ]" pending; other lines are ignored.
func ParseTaskList(path string) []session.Todo {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []session.Todo
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() && len(out) < maxTodos {
		m := checklist.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		st := session.TodoPending
		switch strings.ToLower(m[1]) {
		case "x":
			st = session.TodoDone
		case "/":
			st = session.TodoInProgress
		case "-", "~":
			st = session.TodoBlocked
		}
		out = append(out, session.Todo{Text: m[2], Status: st})
	}
	return out
}
