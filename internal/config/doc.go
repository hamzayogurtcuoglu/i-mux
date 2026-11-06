package config

import (
	"fmt"
	"strings"
)

//go:generate go run ./gendoc ../../docs/CONFIG.md

// Doc renders docs/CONFIG.md from the commented default configuration: a
// table of every key with its default and description, then the whole file.
// A test fails when the committed file is stale; regenerate it with
// `go generate ./internal/config` (or `make generate`).
func Doc() string {
	var b strings.Builder
	b.WriteString("# Configuration\n\n")
	b.WriteString("<!-- Generated from internal/config/default.toml by `go generate ./internal/config`. Do not edit. -->\n\n")
	b.WriteString("ajan reads `~/.ajan/config.toml` (or `$AJAN_HOME/config.toml`). Every key is optional; the\n")
	b.WriteString("values below are the built-in defaults. `ajan config init` writes this file with comments,\n")
	b.WriteString("`ajan config show` prints the effective configuration, and `ajan config path` its location.\n")
	b.WriteString("Unknown keys produce a warning; invalid values and regular expressions are rejected with the\n")
	b.WriteString("line number.\n\n")
	b.WriteString("| Key | Default | Description |\n|---|---|---|\n")
	section := ""
	var comment []string
	for _, line := range strings.Split(Template, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
			comment = nil
		case strings.HasPrefix(t, "#"):
			comment = append(comment, strings.TrimSpace(strings.TrimPrefix(t, "#")))
		case strings.HasPrefix(t, "["):
			section = strings.Trim(t, "[]")
			comment = nil
		default:
			key, val, ok := strings.Cut(t, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			val, trailing, _ := strings.Cut(val, "#")
			val = strings.TrimSpace(val)
			desc := strings.Join(comment, " ")
			if tr := strings.TrimSpace(trailing); tr != "" {
				desc = strings.TrimSpace(desc + " " + tr)
			}
			name := key
			if section != "" {
				name = section + "." + key
			}
			if strings.HasPrefix(section, "agents.") {
				if d, ok := agentKeyDocs[key]; ok {
					desc = strings.TrimSpace(d + " " + strings.TrimSpace(trailing))
				}
				// After Claude, only what differs per agent.
				if section != "agents.claude" && key != "new_args" && key != "resume_args" && key != "home" {
					comment = nil
					continue
				}
			}
			fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", name, cell(val), cell(desc))
			comment = nil
		}
	}
	b.WriteString("\nAgent ids are `claude`, `gemini`, `copilot` and `codex`. Launch templates use the placeholders\n")
	b.WriteString("`{prompt}` (the handoff, at most once), `{id}` (the session to resume) and `{uuid}` (a new id).\n")
	b.WriteString("A placeholder with an empty value is dropped together with the flag before it.\n\n")
	b.WriteString("## The default file\n\n```toml\n")
	b.WriteString(strings.TrimRight(Template, "\n"))
	b.WriteString("\n```\n")
	return b.String()
}

func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// agentKeyDocs describes the per-agent keys ([agents.<id>]).
var agentKeyDocs = map[string]string{
	"bin":                      `Binary to run; "" finds it on PATH (an absolute path or a name).`,
	"args":                     `Arguments inserted before the template, e.g. ["--model", "opus"].`,
	"new_args":                 "Template for a new session; the handoff goes in {prompt}.",
	"resume_args":              "Template for resuming a native session ({id}) with a delta handoff.",
	"home":                     "Where the agent keeps its sessions.",
	"resume":                   "Resume the agent's earlier session when switching back to it.",
	"patterns":                 `Extra limit patterns: [{ re = 'my limit text', severity = "limit" }] (limit, warn or ignore; case-insensitive).`,
	"replace_default_patterns": "Use only `patterns`, not the built-in ones.",
}
