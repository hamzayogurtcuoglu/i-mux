package session

import (
	"os"
	"path/filepath"
	"sync"
)

// Env tells providers where the agents keep their stores. Empty fields use
// each agent's default resolution (environment variables, then ~/.<agent>).
type Env struct {
	Home        string // user home; "" = $HOME
	ClaudeHome  string // "" = $CLAUDE_CONFIG_DIR or ~/.claude
	GeminiHome  string // "" = $GEMINI_CLI_HOME/.gemini or ~/.gemini
	CopilotHome string // "" = $COPILOT_HOME or ~/.copilot
	CodexHome   string // "" = $CODEX_HOME or ~/.codex
}

// UserHome returns Env.Home or $HOME.
func (e Env) UserHome() string {
	if e.Home != "" {
		return e.Home
	}
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// Resolve returns the store root for agent id.
func (e Env) Resolve(id AgentID) string {
	home := e.UserHome()
	switch id {
	case Claude:
		if e.ClaudeHome != "" {
			return e.ClaudeHome
		}
		if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
			return v
		}
		return filepath.Join(home, ".claude")
	case Gemini:
		if e.GeminiHome != "" {
			return e.GeminiHome
		}
		if v := os.Getenv("GEMINI_CLI_HOME"); v != "" {
			return filepath.Join(v, ".gemini")
		}
		return filepath.Join(home, ".gemini")
	case Copilot:
		if e.CopilotHome != "" {
			return e.CopilotHome
		}
		if v := os.Getenv("COPILOT_HOME"); v != "" {
			return v
		}
		return filepath.Join(home, ".copilot")
	case Codex:
		if e.CodexHome != "" {
			return e.CodexHome
		}
		if v := os.Getenv("CODEX_HOME"); v != "" {
			return v
		}
		return filepath.Join(home, ".codex")
	}
	return ""
}

// Factory creates a provider for an environment.
type Factory func(env Env) Provider

var (
	regMu     sync.RWMutex
	factories = map[AgentID]Factory{}
)

// Register makes a provider available. Provider packages call it from init;
// import internal/session/all to register all of them.
func Register(id AgentID, f Factory) {
	regMu.Lock()
	defer regMu.Unlock()
	factories[id] = f
}

// Providers returns a provider for every registered agent, in AllAgents order.
func Providers(env Env) []Provider {
	regMu.RLock()
	defer regMu.RUnlock()
	var out []Provider
	for _, id := range AllAgents {
		if f, ok := factories[id]; ok {
			out = append(out, f(env))
		}
	}
	return out
}

// ProviderFor returns the provider for id, or nil if none is registered.
func ProviderFor(id AgentID, env Env) Provider {
	regMu.RLock()
	defer regMu.RUnlock()
	if f, ok := factories[id]; ok {
		return f(env)
	}
	return nil
}
