// Package limits detects usage-limit messages on agent screens (§10): the
// pattern sets per agent, the reset-time parser and, later, usage stats for
// the limits panel.
package limits

import (
	"fmt"
	"regexp"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// Severity of a pattern.
type Severity string

const (
	SevLimit  Severity = "limit"  // switch-worthy
	SevWarn   Severity = "warn"   // approaching a limit: status bar only
	SevIgnore Severity = "ignore" // looks like a limit but is not (model fallback)
)

// Spec is an uncompiled pattern.
type Spec struct {
	Re       string
	Severity Severity
}

// Pattern is a compiled pattern with an id for logs ("claude.limit.1").
type Pattern struct {
	ID       string
	Re       *regexp.Regexp
	Severity Severity
}

// Defaults are the built-in patterns (§10.1). Matching is case-insensitive
// and per line. Claude separates the reset time with " ∙ ", so no pattern
// depends on a separator.
var Defaults = map[session.AgentID][]Spec{
	session.Claude: {
		{`opus limit reached, now using`, SevIgnore},
		{`(?:5-hour|weekly|opus weekly|usage) limit reached`, SevLimit},
		{`claude (?:ai )?usage limit reached`, SevLimit},
		{`limit reached\s*[–—-]\s*contact an admin`, SevLimit},
		{`you'?ve hit your .*limit`, SevLimit},
		{`approaching (?:5-hour|weekly|opus weekly|opus usage) limit`, SevWarn},
	},
	// Antigravity CLI does not document its quota message (Q31). It runs
	// Gemini models, so Gemini CLI's verified wording is kept next to the
	// usual phrasings.
	session.Antigravity: {
		{`usage limit reached for`, SevLimit},
		{`exhausted your daily quota`, SevLimit},
		{`exceeded your current quota`, SevLimit},
		{`quota (?:exceeded|exhausted|reached|runs? out)`, SevLimit},
		{`you'?ve (?:reached|hit|exhausted|run out of) your .*(?:quota|limit)`, SevLimit},
		{`out of (?:quota|credits)`, SevLimit},
		{`resource_exhausted`, SevLimit},
	},
	session.Copilot: {
		{`exceeded your premium request allowance`, SevLimit},
		{`(?:session|weekly) limit reached`, SevLimit},
	},
	session.Codex: {
		{`you'?ve hit your usage limit`, SevLimit},
	},
	// OpenCode shows its provider's errors; the provider decides the words.
	session.OpenCode: {
		{`rate limit (?:exceeded|reached)`, SevLimit},
		{`too many requests`, SevLimit},
		{`\b429\b.*(?:rate|limit|quota|requests)`, SevLimit},
		{`quota (?:exceeded|exhausted)`, SevLimit},
		{`insufficient (?:credits|quota|balance)`, SevLimit},
		{`out of (?:credits|quota)`, SevLimit},
	},
}

// Set is the compiled patterns of every agent.
type Set map[session.AgentID][]Pattern

// Compile builds the pattern set from the defaults and the user's config:
// per-agent `patterns` are added (or replace the defaults with
// replace_default_patterns = true).
func Compile(cfg *config.Config) (Set, error) {
	set := Set{}
	for _, id := range session.AllAgents {
		ac := cfg.Agent(string(id))
		var specs []Spec
		if !ac.ReplaceDefaultPatterns {
			specs = append(specs, Defaults[id]...)
		}
		for _, p := range ac.Patterns {
			specs = append(specs, Spec{Re: p.Re, Severity: Severity(p.Severity)})
		}
		n := map[Severity]int{}
		for _, sp := range specs {
			re, err := config.CompilePattern(sp.Re)
			if err != nil {
				return nil, fmt.Errorf("agents.%s.patterns: %w", id, err)
			}
			n[sp.Severity]++
			set[id] = append(set[id], Pattern{ID: fmt.Sprintf("%s.%s.%d", id, sp.Severity, n[sp.Severity]), Re: re, Severity: sp.Severity})
		}
	}
	return set, nil
}

// Match returns the pattern that decides line: ignore patterns win over
// limit patterns, which win over warn patterns.
func (s Set) Match(id session.AgentID, line string) (Pattern, bool) {
	var best *Pattern
	rank := map[Severity]int{SevIgnore: 3, SevLimit: 2, SevWarn: 1}
	for i := range s[id] {
		p := &s[id][i]
		if (best == nil || rank[p.Severity] > rank[best.Severity]) && p.Re.MatchString(line) {
			best = p
		}
	}
	if best == nil {
		return Pattern{}, false
	}
	return *best, true
}
