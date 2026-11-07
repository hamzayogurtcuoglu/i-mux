// Package all registers every session provider. Import it for side effects.
package all

import (
	_ "github.com/hamzayogurtcuoglu/i-mux/internal/session/antigravity" // Antigravity CLI
	_ "github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"      // Claude Code
	_ "github.com/hamzayogurtcuoglu/i-mux/internal/session/codex"       // Codex CLI
	_ "github.com/hamzayogurtcuoglu/i-mux/internal/session/copilot"     // Copilot CLI
)
