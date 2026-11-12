package auto

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// Model is one model a CLI offers.
type Model struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"` // its display name, when the CLI gives one
}

// ModelsFor is how long a CLI's model list is kept.
const ModelsFor = 24 * time.Hour

// modelsTimeout bounds asking a CLI for its models.
var modelsTimeout = 8 * time.Second

type modelCache map[string]struct {
	At     time.Time `json:"at"`
	Bin    string    `json:"bin"`
	Models []Model   `json:"models"`
}

func modelsFile() string { return filepath.Join(paths.Home(), "models.json") }

// Models lists cli's models (binary bin, PATH pathEnv): Antigravity's and
// OpenCode's `models`, the aliases Claude's --help names, Copilot's
// "auto". The list is kept a day; an empty list is not kept.
func Models(ctx context.Context, cli session.AgentID, bin, pathEnv string, now time.Time) []Model {
	cache := modelCache{}
	if b, err := os.ReadFile(modelsFile()); err == nil {
		_ = json.Unmarshal(b, &cache)
	}
	if c, ok := cache[string(cli)]; ok && c.Bin == bin && now.Sub(c.At) < ModelsFor && len(c.Models) > 0 {
		return c.Models
	}
	ms := listModels(ctx, cli, bin, pathEnv)
	if len(ms) > 0 {
		cache[string(cli)] = struct {
			At     time.Time `json:"at"`
			Bin    string    `json:"bin"`
			Models []Model   `json:"models"`
		}{now, bin, ms}
		if b, err := json.Marshal(cache); err == nil {
			_ = paths.WriteFile(modelsFile(), b)
		}
	}
	return ms
}

func listModels(ctx context.Context, cli session.AgentID, bin, pathEnv string) []Model {
	run := func(args ...string) string {
		ctx, cancel := context.WithTimeout(ctx, modelsTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(os.Environ(), "NO_COLOR=1")
		if pathEnv != "" {
			cmd.Env = append(cmd.Env, "PATH="+pathEnv)
		}
		cmd.WaitDelay = time.Second
		out, _ := cmd.Output()
		return string(out)
	}
	switch cli {
	case session.Claude:
		return ClaudeAliases(run("--help"))
	case session.Antigravity, session.OpenCode:
		return ParseModelList(run("models"))
	case session.Copilot:
		return []Model{{ID: "auto"}}
	}
	return nil
}

var (
	quoted     = regexp.MustCompile(`'([a-z][a-z0-9.\-]*)'`)
	nextOption = regexp.MustCompile(`\n\s+-[-a-zA-Z]`)
)

// ClaudeAliases are the model aliases `claude --help` names under --model
// ("e.g. 'fable', 'opus', or 'sonnet'").
func ClaudeAliases(help string) []Model {
	i := strings.Index(help, "--model <model>")
	if i < 0 {
		return nil
	}
	rest := help[i:]
	// The option's description: up to the next option.
	if j := nextOption.FindStringIndex(rest[1:]); j != nil {
		rest = rest[:j[0]+1]
	}
	var out []Model
	seen := map[string]bool{}
	for _, m := range quoted.FindAllStringSubmatch(rest, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, Model{ID: m[1]})
		}
	}
	return out
}

// ParseModelList reads a `models` listing: one model per line, its id then,
// after a tab or two spaces, its name; headers and notes are left out.
func ParseModelList(out string) []Model {
	var ms []Model
	sc := bufio.NewScanner(strings.NewReader(Clean(out)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasSuffix(line, "...") || strings.HasSuffix(line, "…") || strings.HasSuffix(line, ":") {
			continue
		}
		id, name := line, ""
		if k, v, ok := strings.Cut(line, "\t"); ok {
			id, name = k, strings.TrimSpace(v)
		} else if k, v, ok := strings.Cut(line, "  "); ok {
			id, name = k, strings.TrimSpace(v)
		}
		id = strings.TrimSpace(id)
		if id == "" || strings.ContainsAny(id, " \t") {
			continue
		}
		ms = append(ms, Model{ID: id, Name: name})
	}
	return ms
}
