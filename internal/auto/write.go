package auto

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// ErrExists is returned when an agent with the name is already there.
var ErrExists = errors.New("an agent with this name already exists")

// Exists reports whether the project at root has an agent named name in
// any of its agent folders.
func Exists(root, name string) bool {
	for _, d := range List(root, "") {
		if d.Kind == KindAgent && (d.Name == name || strings.TrimSuffix(strings.TrimSuffix(filepath.Base(d.File), ".md"), ".agent") == name) {
			return true
		}
	}
	return false
}

// Template is a blank agent definition named name (Claude Code's format,
// which i-mux runs with any CLI).
func Template(name, description string) string {
	if description == "" {
		description = "What this agent does and when it runs."
	}
	return "---\nname: " + name + "\ndescription: " + description + `
---

You run in this repository with nobody watching, often on a schedule.

1. Check what you are here for (the commands, files or pages to look at).
2. Do what is needed, without asking questions.
3. Report in a few lines what you found or did; if there was nothing new,
   reply with exactly ` + NothingNew + `.
`
}

// Create writes the blank agent named name in the project's
// .claude/agents and returns its path.
func Create(root, name string) (string, error) {
	if Exists(root, name) {
		return "", ErrExists
	}
	p := AgentFile(root, session.Claude, name)
	if err := paths.EnsureDir(filepath.Dir(p)); err != nil {
		return "", err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "", ErrExists
		}
		return "", err
	}
	_, err = f.WriteString(Template(name, ""))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return p, err
}

// WritePrompt asks the agent CLI cli to write an agent for the project at
// root that does what ask says ("": it asks first, in lang).
func WritePrompt(root string, cli session.AgentID, ask, lang string) string {
	dir := filepath.Join(root, AgentDir(cli))
	file := "<name>.md"
	format := `starts with YAML front matter holding "name" (the same as the file's name) and "description", then the agent's instructions in Markdown.`
	switch cli {
	case session.Claude:
		format = `starts with Claude Code's subagent front matter ("name", "description"; "tools" and "model" only when they are needed), then the agent's instructions (its system prompt) in Markdown.`
	case session.Copilot:
		file = "<name>.agent.md"
		format = `starts with GitHub Copilot's custom agent front matter ("name", "description"; "tools" only when needed), then the agent's instructions in Markdown.`
	case session.OpenCode:
		format = `starts with OpenCode's agent front matter ("description", "mode: primary"), then the agent's instructions in Markdown.`
	}
	guide := "Create an agent for this project (" + root + ") that i-mux runs on a schedule or by hand, with nobody watching.\n\nHow to write it:\n" +
		"- Save it as " + dir + "/" + file + ", where <name> is a short lowercase-hyphenated name you choose.\n" +
		"- The file " + format + "\n" +
		"- The instructions are the whole job, step by step: the exact commands, files, URLs and checks for this repository. Look at the code and the tools it uses first so they are right.\n" +
		"- It runs non-interactively: it must never ask questions or wait for an answer; it decides itself.\n" +
		"- It ends with a short report whose first line is the news (i-mux shows it in its sidebar), or with exactly " + NothingNew + " when there is nothing new. i-mux gives it its last report, so it can tell what is new.\n" +
		"- Only create that one file. When you are done, say the agent's name in one line.\n"
	ask = strings.TrimSpace(strings.ReplaceAll(ask, "\r\n", "\n"))
	if ask == "" {
		if lang == "" {
			lang = "English"
		}
		return guide + "\nFirst ask me, in " + lang + " and in one short question, what the agent should do, and wait for my answer before you write anything."
	}
	return guide + "\nWhat the agent should do:\n" + ask
}
