// Package auto is the Scheduler (D61, §9.3 "Scheduler"): the agent
// definitions and skills a project has, the times they run at, how each
// agent CLI runs one headless, and what the runs reported.
package auto

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/skills"
)

// Kind is what a schedule runs.
type Kind string

// Kinds of definitions.
const (
	KindAgent Kind = "agent"
	KindSkill Kind = "skill"
)

// Def is an agent definition or a skill that can run on a schedule.
type Def struct {
	ID          string // 12 hex digits: the project and the file
	Kind        Kind
	Name        string
	Description string
	File        string          // absolute: the agent's .md, or the skill's SKILL.md
	Place       string          // the folder it was found in, e.g. ".claude/agents" or "~/.claude/agents"
	Owner       session.AgentID // the CLI whose kind of definition it is ("" for any)
	Personal    bool            // in the user's home: for every project
	Model       string          // the model its front matter names
	Problem     string          // a skill's front-matter problem (skills.Prob...)
	Shared      bool            // a skill in .agents/skills, linked for Claude: every agent sees it
}

// agentPlace is a folder of agent definitions.
type agentPlace struct {
	dir      string // relative to the project root, or to the home when personal
	owner    session.AgentID
	personal bool
}

// agentPlaces are the folders agent CLIs read agent definitions from, and
// i-mux's own (.agents/agents) for any CLI.
var agentPlaces = []agentPlace{
	{".claude/agents", session.Claude, false},
	{".github/agents", session.Copilot, false},
	{".opencode/agent", session.OpenCode, false},
	{".opencode/agents", session.OpenCode, false},
	{".agents/agents", "", false},
	{".claude/agents", session.Claude, true},
	{".copilot/agents", session.Copilot, true},
}

// ID is a definition's id in the project at root: a hash of both, which a
// menu command can carry (D58).
func ID(root, file string) string {
	h := sha256.Sum256([]byte(root + "\x00" + file))
	return hex.EncodeToString(h[:])[:12]
}

// ValidID reports whether id is one ID made.
func ValidID(id string) bool {
	if len(id) != 12 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// List finds the definitions of the project at root: its agents, the
// personal ones in home ("" for none), then its skills; by name within each.
func List(root, home string) []Def {
	var agents []Def
	seen := map[string]bool{}
	for _, pl := range agentPlaces {
		base, place := root, pl.dir
		if pl.personal {
			if home == "" {
				continue
			}
			base, place = home, "~/"+pl.dir
		}
		dir := filepath.Join(base, pl.dir)
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
				continue
			}
			file := filepath.Join(dir, name)
			if seen[file] {
				continue
			}
			fm, _, ok := FrontMatter(file)
			if !ok {
				continue // no front matter: not an agent definition (a README)
			}
			seen[file] = true
			d := Def{
				ID: ID(root, file), Kind: KindAgent, File: file, Place: place, Owner: pl.owner, Personal: pl.personal,
				Name: fm["name"], Description: oneLine(fm["description"]), Model: fm["model"],
			}
			if d.Name == "" {
				d.Name = strings.TrimSuffix(strings.TrimSuffix(name, ".md"), ".agent")
			}
			agents = append(agents, d)
		}
	}
	sort.SliceStable(agents, func(i, j int) bool {
		if agents[i].Personal != agents[j].Personal {
			return !agents[i].Personal
		}
		return strings.ToLower(agents[i].Name) < strings.ToLower(agents[j].Name)
	})
	for _, sk := range skills.List(root) {
		if sk.Problem == skills.ProbNoFile {
			continue
		}
		agents = append(agents, Def{
			ID: ID(root, sk.Path()), Kind: KindSkill, Name: sk.Name, Description: oneLine(sk.Description),
			File: sk.Path(), Place: sk.Place, Problem: sk.Problem, Shared: sk.Shared,
		})
	}
	return agents
}

// Find returns the definition with id among List's.
func Find(defs []Def, id string) (Def, bool) {
	for _, d := range defs {
		if d.ID == id {
			return d, true
		}
	}
	return Def{}, false
}

// maxFile bounds what is read of a definition.
const maxFile = 256 << 10

// FrontMatter reads a Markdown file's YAML front matter (top-level
// "key: value" lines; a "|" or ">" value takes the indented lines under
// it) and the body after it. ok is false without a front matter.
func FrontMatter(path string) (fm map[string]string, body string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), maxFile)
	if !sc.Scan() || strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff")) != "---" {
		return nil, "", false
	}
	fm = map[string]string{}
	var block string // the key whose indented block is being read
	var folded bool
	closed := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "---" {
			closed = true
			break
		}
		if block != "" && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.TrimSpace(line) == "") {
			sep := "\n"
			if folded {
				sep = " "
			}
			if fm[block] != "" {
				fm[block] += sep
			}
			fm[block] += strings.TrimSpace(line)
			continue
		}
		block = ""
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "#") {
			continue // a list item or a nested key
		}
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if strings.HasPrefix(v, "|") || strings.HasPrefix(v, ">") {
			block, folded = k, strings.HasPrefix(v, ">")
			fm[k] = ""
			continue
		}
		fm[k] = unquote(v)
	}
	if !closed {
		return nil, "", false
	}
	var b strings.Builder
	size := 0
	for sc.Scan() {
		size += len(sc.Text()) + 1
		if size > maxFile {
			break
		}
		b.WriteString(sc.Text() + "\n")
	}
	for k, v := range fm {
		fm[k] = strings.TrimSpace(v)
	}
	return fm, strings.TrimSpace(b.String()), true
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		inner := v[1 : len(v)-1]
		if v[0] == '"' {
			inner = strings.NewReplacer(`\n`, "\n", `\"`, `"`, `\\`, `\`).Replace(inner)
		} else {
			inner = strings.ReplaceAll(inner, "''", "'")
		}
		return inner
	}
	return v
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// DefaultCLI is the CLI a definition runs with when none was chosen: its
// own, else the first installed one in order.
func DefaultCLI(d Def, order []session.AgentID, installed func(session.AgentID) bool) session.AgentID {
	if d.Owner != "" && installed(d.Owner) {
		return d.Owner
	}
	for _, id := range order {
		if installed(id) {
			return id
		}
	}
	if d.Owner != "" {
		return d.Owner
	}
	if len(order) > 0 {
		return order[0]
	}
	return session.Claude
}
