// Package skills finds a project's Agent Skills (`<name>/SKILL.md` folders)
// in every place an agent CLI reads them, tells which agents see each one,
// and keeps them in one place for all of them (D32, §9.3 "Skills"):
// `.agents/skills/` holds the skill, and a link in `.claude/skills/` shows it
// to Claude Code, the one CLI that does not read `.agents/skills/`.
package skills

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// Home is where i-mux keeps a project's skills (relative to its root).
const Home = ".agents/skills"

// ClaudeDir is Claude Code's project skills folder; i-mux links skills there.
const ClaudeDir = ".claude/skills"

// File is a skill's main file.
const File = "SKILL.md"

// Place is a project folder of skills and the agents that read it.
type Place struct {
	Dir    string // relative to the project root
	Agents []session.AgentID
}

// Places are the project folders the agent CLIs read skills from, i-mux's
// own first (§21: the CLIs' documentation, 2026-11-09).
var Places = []Place{
	{Home, []session.AgentID{session.Antigravity, session.Codex, session.Copilot, session.OpenCode}},
	{ClaudeDir, []session.AgentID{session.Claude, session.Copilot, session.OpenCode}},
	{".github/skills", []session.AgentID{session.Copilot}},
	{".opencode/skills", []session.AgentID{session.OpenCode}},
	{".opencode/skill", []session.AgentID{session.OpenCode}},
	{".agent/skills", []session.AgentID{session.Antigravity}},
	{".codex/skills", []session.AgentID{session.Codex}},
}

// Problems a skill can have; the first one found is reported.
const (
	ProbNone       = ""
	ProbNoFile     = "no_file"        // the folder has no SKILL.md
	ProbNoHeader   = "no_header"      // SKILL.md does not start with a --- block
	ProbNoName     = "no_name"        // the block has no name
	ProbNoDesc     = "no_description" // ... or no description
	ProbNameFolder = "name_folder"    // name and folder differ
	ProbBadName    = "bad_name"       // not lowercase letters, digits and hyphens (≤ 64)
	ProbLongDesc   = "long_description"
)

const (
	maxDescription = 1024 // characters the standard allows
	maxName        = 64
	maxHeader      = 600 // lines read looking for the front matter's end
	keepDesc       = 400 // characters of the description kept for the list
)

// Skill is one skill of the project.
type Skill struct {
	Name        string // the name in SKILL.md, else the folder's
	Folder      string // the folder's name
	Description string
	Dir         string // its folder (absolute): the first place it is in
	Place       string // that place (relative), e.g. ".agents/skills"
	Shared      bool   // in .agents/skills and linked for Claude: every agent sees it
	Agents      map[session.AgentID]bool
	Problem     string
}

// Path is its SKILL.md.
func (s Skill) Path() string { return filepath.Join(s.Dir, File) }

// Root is the project folder of dir: the closest folder above it (itself
// included) with a .git, else dir.
func Root(dir string) string {
	for d := dir; ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		up := filepath.Dir(d)
		if up == d {
			return dir
		}
		d = up
	}
}

// List finds the skills of the project at root, by name.
func List(root string) []Skill {
	byFolder := map[string]*Skill{}
	var order []string
	for _, pl := range Places {
		ents, err := os.ReadDir(filepath.Join(root, pl.Dir))
		if err != nil {
			continue
		}
		for _, e := range ents {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			dir := filepath.Join(root, pl.Dir, name)
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				continue // a file, or a link to nothing
			}
			sk := byFolder[name]
			if sk == nil {
				sk = &Skill{Folder: name, Dir: dir, Place: pl.Dir, Agents: map[session.AgentID]bool{}}
				sk.read()
				byFolder[name] = sk
				order = append(order, name)
			}
			if sk.Problem == ProbNoFile {
				continue // nothing an agent can load
			}
			for _, a := range pl.Agents {
				sk.Agents[a] = true
			}
		}
	}
	out := make([]Skill, 0, len(order))
	for _, n := range order {
		sk := byFolder[n]
		sk.Shared = sk.Place == Home && linked(root, n)
		out = append(out, *sk)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// linked reports whether Claude's folder has the skill (a link or a copy).
func linked(root, folder string) bool {
	st, err := os.Stat(filepath.Join(root, ClaudeDir, folder, File))
	return err == nil && !st.IsDir()
}

// read fills the name, description and problem from SKILL.md.
func (s *Skill) read() {
	s.Name = s.Folder
	f, err := os.Open(s.Path())
	if err != nil {
		s.Problem = ProbNoFile
		return
	}
	defer f.Close()
	h, ok := header(bufio.NewScanner(f))
	if !ok {
		s.Problem = ProbNoHeader
		return
	}
	if h["name"] != "" {
		s.Name = h["name"]
	}
	desc := h["description"]
	s.Description = cut(desc, keepDesc)
	switch {
	case h["name"] == "":
		s.Problem = ProbNoName
	case desc == "":
		s.Problem = ProbNoDesc
	case !ValidName(h["name"]):
		s.Problem = ProbBadName
	case h["name"] != s.Folder:
		s.Problem = ProbNameFolder
	case len([]rune(desc)) > maxDescription:
		s.Problem = ProbLongDesc
	}
}

// header reads the YAML front matter's top-level "key: value" lines (a
// value may continue on indented lines, or be a > or | block).
func header(sc *bufio.Scanner) (map[string]string, bool) {
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() || strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff")) != "---" {
		return nil, false
	}
	h := map[string]string{}
	key := ""
	for n := 0; sc.Scan() && n < maxHeader; n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "---" {
			return h, true
		}
		if line != "" && (line[0] == ' ' || line[0] == '\t') {
			if key != "" {
				h[key] = strings.TrimSpace(h[key] + " " + strings.TrimSpace(line))
			}
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			key = ""
			continue
		}
		key = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if v == ">" || v == "|" || v == ">-" || v == "|-" {
			v = ""
		}
		h[key] = unquote(v)
	}
	return nil, false
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ValidName reports a name the skills standard takes: lowercase letters,
// digits and single hyphens, not at the ends, at most 64 characters.
func ValidName(n string) bool {
	if n == "" || len(n) > maxName || n[0] == '-' || n[len(n)-1] == '-' || strings.Contains(n, "--") {
		return false
	}
	for _, r := range n {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

// Slug makes a skill name of free text: "Sürüm notları yaz" →
// "surum-notlari-yaz" ("" when nothing is left).
func Slug(text string) string {
	tr := strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u",
		"Ç", "c", "Ğ", "g", "İ", "i", "Ö", "o", "Ş", "s", "Ü", "u", "ä", "a", "ß", "ss", "é", "e", "è", "e", "á", "a", "ñ", "n")
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(tr.Replace(text)) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r):
			dash = true
		}
		if b.Len() >= maxName {
			break
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > maxName {
		s = strings.Trim(s[:maxName], "-")
	}
	return s
}

// Sync links every skill in .agents/skills that Claude's folder lacks: a
// relative link, so it works in every clone. An entry already there (a
// skill of the same name, a file) is left alone. It returns the names linked.
func Sync(root string) []string {
	ents, err := os.ReadDir(filepath.Join(root, Home))
	if err != nil {
		return nil
	}
	var done []string
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if st, err := os.Stat(filepath.Join(root, Home, name, File)); err != nil || st.IsDir() {
			continue
		}
		link := filepath.Join(root, ClaudeDir, name)
		if _, err := os.Lstat(link); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Join(root, ClaudeDir), 0o755); err != nil {
			return done
		}
		if os.Symlink(filepath.Join("..", "..", Home, name), link) == nil {
			done = append(done, name)
		}
	}
	return done
}

// ErrExists is returned when a skill of that name is already in .agents/skills.
var ErrExists = errors.New("a skill with this name already exists")

// Share moves a skill found elsewhere (.claude/skills, .github/skills ...)
// into .agents/skills and links it back for Claude, so every agent sees it.
func Share(root, dir string) (string, error) {
	name := filepath.Base(dir)
	dst := filepath.Join(root, Home, name)
	if filepath.Clean(dir) == dst {
		Sync(root)
		return dst, nil
	}
	if _, err := os.Lstat(dst); err == nil {
		return "", ErrExists
	}
	if err := os.MkdirAll(filepath.Join(root, Home), 0o755); err != nil {
		return "", err
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	real := dir
	if st.Mode()&os.ModeSymlink != 0 {
		if real, err = filepath.EvalSymlinks(dir); err != nil {
			return "", err
		}
	}
	if err := os.Rename(real, dst); err != nil {
		return "", err
	}
	if real != dir {
		_ = os.Remove(dir) // the link that pointed at it
	}
	Sync(root)
	return dst, nil
}

// Links are the links in the project's skill places that point at dir: the
// ones to take away with it.
func Links(root, dir string) []string {
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, pl := range Places {
		p := filepath.Join(root, pl.Dir, filepath.Base(dir))
		if p == dir {
			continue
		}
		if st, err := os.Lstat(p); err != nil || st.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if got, err := filepath.EvalSymlinks(p); err == nil && got == want {
			out = append(out, p)
		}
	}
	return out
}

// Create writes a new skill's SKILL.md from the template and returns its
// path. The name must be valid and free.
func Create(root, name, description string) (string, error) {
	if !ValidName(name) {
		return "", errors.New("invalid skill name")
	}
	dir := filepath.Join(root, Home, name)
	if _, err := os.Lstat(dir); err == nil {
		return "", ErrExists
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, File)
	if err := os.WriteFile(p, []byte(Template(name, description)), 0o644); err != nil {
		return "", err
	}
	Sync(root)
	return p, nil
}

// Template is a new skill's SKILL.md.
func Template(name, description string) string {
	if description == "" {
		description = "What this skill does, and when an agent should use it. Agents read only this line to decide."
	}
	title := strings.ReplaceAll(name, "-", " ")
	if r := []rune(title); len(r) > 0 {
		title = string(unicode.ToUpper(r[0])) + string(r[1:])
	}
	return "---\nname: " + name + "\ndescription: " + oneLine(description) + "\n---\n\n# " + title + `

## When to use

- 

## Steps

1. 

## Checks

- 
`
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// WritePrompt is what an agent is asked to write a new skill with. It is
// model-facing, so English (like the handoff).
func WritePrompt(root, ask string) string {
	return "Create an Agent Skill for this project. What it should do: " + oneLine(ask) + `

Write it to ` + filepath.Join(root, Home) + `/<name>/SKILL.md, where <name> is a short lowercase-hyphenated name you choose (letters, digits, hyphens; the same as the folder).
SKILL.md starts with YAML front matter holding "name" and "description", then the instructions in Markdown:
- description: one or two sentences, under 1024 characters, saying what the skill does and when to use it. Agents read only this line to decide whether to load the skill, so name the situations and words that should trigger it.
- body: the concrete steps, commands, file paths and checks for this repository. Look at the code first so they are right; keep it short and practical.
- Long reference material or helper scripts go next to SKILL.md (references/, scripts/), mentioned from it.
Only create files under that folder. i-mux shows the skill to every agent (it links it into .claude/skills for Claude Code). When you are done, say the skill's name in one line.`
}

// ImprovePrompt asks an agent to improve an existing skill.
func ImprovePrompt(path string) string {
	return "Improve the Agent Skill at " + path + `.
Check it against this repository's code and fix what is wrong or out of date. Make the description (front matter, under 1024 characters) say clearly what the skill does and when to use it, since agents read only that line to decide whether to load it. Keep the name and the folder. Keep the body short, concrete and correct. Say in one line what you changed.`
}
