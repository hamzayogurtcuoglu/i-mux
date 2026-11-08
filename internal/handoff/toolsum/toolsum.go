// Package toolsum turns agent tool calls into one-line summaries and file
// activity, with a generic, ordered rule list keyed on the tool name and its
// argument names (§8.4). The first matching rule wins.
package toolsum

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// MaxLine caps a summary line (runes).
const MaxLine = 160

// Kind classifies a tool call's effect on files.
type Kind uint8

// Kinds.
const (
	KindNone Kind = iota
	KindEdited
	KindCreated
	KindRead
	KindDeleted
)

// Summary is the result for one tool call.
type Summary struct {
	Line  string // "" when the call is not shown
	Skip  bool   // report_intent and similar
	Todos bool   // captured as the task list
	Files []File // files touched
}

// File is one touched file.
type File struct {
	Path string // display form (relative to cwd when inside it)
	Kind Kind
}

var (
	reTodo     = regexp.MustCompile(`(?i)todo|plan|task`)
	reShell    = regexp.MustCompile(`(?i)bash|shell|exec_command|run_shell`)
	reEdit     = regexp.MustCompile(`(?i)edit|replace|str_replace|multi_?edit`)
	reWrite    = regexp.MustCompile(`(?i)write|create`)
	reRead     = regexp.MustCompile(`(?i)read|view|cat|open`)
	reSearch   = regexp.MustCompile(`(?i)grep|rg|search|glob|find`)
	reList     = regexp.MustCompile(`(?i)list|^ls$|_ls$|^ls_`)
	reWeb      = regexp.MustCompile(`(?i)web_?search|google_web_search`)
	reMCP      = regexp.MustCompile(`^mcp__([^_].*?)__(.+)$`)
	patchHdrRe = regexp.MustCompile(`(?m)^\*\*\* (Update|Add|Delete) File: (.+?)\s*$|^\*\*\* Move to: (.+?)\s*$`)
)

var pathKeys = []string{"file_path", "absolute_path", "path", "filename", "notebook_path"}

// Summarize applies the rules to one call. cwd shortens paths inside it.
func Summarize(tc session.ToolCall, cwd string) Summary {
	s := summarize(tc, cwd)
	if s.Line != "" {
		if tc.Failed {
			s.Line = clean.OneLine(s.Line, MaxLine-len(" (failed)")) + " (failed)"
		} else {
			s.Line = clean.OneLine(s.Line, MaxLine)
		}
	}
	return s
}

func summarize(tc session.ToolCall, cwd string) Summary {
	name := tc.Name
	args := tc.Args
	// 1. report_intent
	if strings.EqualFold(name, "report_intent") {
		return Summary{Skip: true}
	}
	// 2. task lists
	if reTodo.MatchString(name) && hasListArg(args) {
		return Summary{Todos: true}
	}
	// 3. apply_patch
	if patch := findPatch(args); patch != "" || strings.Contains(strings.ToLower(name), "apply_patch") {
		files := patchFiles(patch, cwd)
		if len(files) == 0 {
			return Summary{Line: "applied a patch"}
		}
		parts := make([]string, 0, len(files))
		for _, f := range files {
			switch f.Kind {
			case KindCreated:
				parts = append(parts, "created "+f.Path)
			case KindDeleted:
				parts = append(parts, "deleted "+f.Path)
			default:
				parts = append(parts, "edited "+f.Path)
			}
		}
		return Summary{Line: strings.Join(parts, ", "), Files: files}
	}
	// 4. shell commands
	if cmd := commandArg(args); cmd != "" {
		if readsHandoff(cmd) {
			return Summary{Skip: true}
		}
		return Summary{Line: "$ " + firstLine(cmd, 120)}
	} else if reShell.MatchString(name) {
		if s := firstScalar(args, 120); s != "" {
			return Summary{Line: "$ " + s}
		}
	}
	p := pathArg(args)
	dp := display(p, cwd)
	// 5. edits
	if p != "" && reEdit.MatchString(name) {
		return Summary{Line: "edited " + dp, Files: []File{{dp, KindEdited}}}
	}
	// 6. writes
	if p != "" && reWrite.MatchString(name) {
		return Summary{Line: "wrote " + dp, Files: []File{{dp, KindCreated}}}
	}
	// 7. reads; reading imux's handoff file is how the agent got its
	// context, not work on the project
	if p != "" && reRead.MatchString(name) {
		if session.IsHandoffFile(p) {
			return Summary{Skip: true}
		}
		return Summary{Line: "read " + dp, Files: []File{{dp, KindRead}}}
	}
	// 11 (before 8, which would also match "search"). web search
	if q := strArg(args, "query"); q != "" && reWeb.MatchString(name) {
		return Summary{Line: fmt.Sprintf("web search %q", firstLine(q, 100))}
	}
	// 8. search
	if reSearch.MatchString(name) {
		if q := firstNonEmpty(strArg(args, "pattern"), strArg(args, "query"), strArg(args, "regex")); q != "" {
			line := fmt.Sprintf("searched %q", firstLine(q, 100))
			if p != "" {
				line += " in " + dp
			}
			return Summary{Line: line}
		}
	}
	// 9. listing
	if p != "" && reList.MatchString(name) {
		return Summary{Line: "listed " + dp}
	}
	// 10. URLs
	if u := strArg(args, "url"); u != "" {
		return Summary{Line: "fetched " + firstLine(u, 140)}
	}
	// 12. MCP tools
	if m := reMCP.FindStringSubmatch(name); m != nil {
		return Summary{Line: fmt.Sprintf("mcp %s.%s(%s)", m[1], m[2], firstScalar(args, 60))}
	}
	// 13. fallback
	return Summary{Line: fmt.Sprintf("%s(%s)", name, firstScalar(args, 60))}
}

func hasListArg(args map[string]any) bool {
	for _, k := range []string{"todos", "plan"} {
		if _, ok := args[k].([]any); ok {
			return true
		}
	}
	return false
}

// findPatch returns an apply_patch body found in any string argument or
// argv element.
func findPatch(args map[string]any) string {
	for _, k := range sortedKeys(args) {
		switch v := args[k].(type) {
		case string:
			if strings.Contains(v, "*** Begin Patch") {
				return v
			}
		case []any:
			for _, e := range v {
				if s, ok := e.(string); ok && strings.Contains(s, "*** Begin Patch") {
					return s
				}
			}
		}
	}
	return ""
}

// patchFiles lists files from patch headers.
func patchFiles(patch, cwd string) []File {
	var out []File
	for _, m := range patchHdrRe.FindAllStringSubmatch(patch, -1) {
		switch {
		case m[1] == "Add":
			out = append(out, File{display(m[2], cwd), KindCreated})
		case m[1] == "Update":
			out = append(out, File{display(m[2], cwd), KindEdited})
		case m[1] == "Delete":
			out = append(out, File{display(m[2], cwd), KindDeleted})
		case m[3] != "":
			out = append(out, File{display(m[3], cwd), KindEdited})
		}
	}
	return out
}

// commandArg returns a shell command from "command" or "cmd" (string or
// argv; [sh, -c|-lc, X] yields X).
// readsHandoff reports whether a shell command only prints imux's handoff
// file (cat, sed -n, head ...).
func readsHandoff(cmd string) bool {
	f := strings.Fields(strings.TrimSpace(cmd))
	if len(f) < 2 {
		return false
	}
	switch filepath.Base(f[0]) {
	case "cat", "sed", "head", "tail", "less", "more", "bat", "nl":
	default:
		return false
	}
	for _, w := range f[1:] {
		if session.IsHandoffFile(strings.Trim(w, `"'`)) {
			return true
		}
	}
	return false
}

func commandArg(args map[string]any) string {
	for _, k := range []string{"command", "cmd"} {
		switch v := args[k].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		case []any:
			var argv []string
			for _, e := range v {
				if s, ok := e.(string); ok {
					argv = append(argv, s)
				}
			}
			if len(argv) == 0 {
				continue
			}
			if len(argv) == 3 && (argv[1] == "-c" || argv[1] == "-lc") {
				return argv[2]
			}
			return strings.Join(argv, " ")
		}
	}
	return ""
}

func pathArg(args map[string]any) string {
	for _, k := range pathKeys {
		if s, ok := args[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func strArg(args map[string]any, k string) string {
	s, _ := args[k].(string)
	return s
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

var preferredKeys = []string{"description", "query", "pattern", "path", "file_path", "name", "prompt", "url", "id"}

// firstScalar returns the first short scalar argument (preferred keys
// first, then alphabetical), cut to max runes.
func firstScalar(args map[string]any, max int) string {
	keys := append([]string{}, preferredKeys...)
	keys = append(keys, sortedKeys(args)...)
	for _, k := range keys {
		v, ok := args[k]
		if !ok {
			continue
		}
		switch x := v.(type) {
		case string:
			if strings.TrimSpace(x) != "" {
				return firstLine(x, max)
			}
		case float64, bool, int, int64:
			return fmt.Sprint(x)
		}
	}
	return ""
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i]) + " …"
	}
	return clean.OneLine(s, max)
}

// display shows p relative to cwd when it lies inside it.
func display(p, cwd string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	if cwd != "" && filepath.IsAbs(p) {
		if rel, err := filepath.Rel(cwd, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return rel
		}
	}
	return strings.TrimPrefix(p, "./")
}

// MaxReadSample caps FileActivity.Read.
const MaxReadSample = 8

// Activity computes file activity over a transcript.
func Activity(entries []session.Entry, cwd string) session.FileActivity {
	var fa session.FileActivity
	created := map[string]bool{}
	reads := map[string]bool{}
	for _, e := range entries {
		for _, tc := range e.Tools {
			if tc.Failed {
				continue
			}
			for _, f := range Summarize(tc, cwd).Files {
				switch f.Kind {
				case KindCreated:
					if !created[f.Path] {
						created[f.Path] = true
						fa.Created = append(fa.Created, f.Path)
					}
					fa.Edited = remove(fa.Edited, f.Path)
				case KindEdited:
					if created[f.Path] {
						continue
					}
					fa.Edited = append(remove(fa.Edited, f.Path), f.Path)
				case KindDeleted:
					fa.Edited = remove(fa.Edited, f.Path)
					fa.Created = remove(fa.Created, f.Path)
				case KindRead:
					if !reads[f.Path] {
						reads[f.Path] = true
						fa.ReadCount++
						if len(fa.Read) < MaxReadSample {
							fa.Read = append(fa.Read, f.Path)
						}
					}
				}
			}
		}
	}
	return fa
}

// LatestTodos returns the task list from the newest todo/plan tool call.
func LatestTodos(entries []session.Entry) []session.Todo {
	for i := len(entries) - 1; i >= 0; i-- {
		tools := entries[i].Tools
		for j := len(tools) - 1; j >= 0; j-- {
			if reTodo.MatchString(tools[j].Name) && hasListArg(tools[j].Args) {
				return session.TodosFromArgs(tools[j].Args)
			}
		}
	}
	return nil
}

func remove(list []string, s string) []string {
	out := list[:0]
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}
