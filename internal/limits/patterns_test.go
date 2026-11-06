package limits

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

func TestDefaultPatterns(t *testing.T) {
	set, err := Compile(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		agent session.AgentID
		line  string
		want  Severity // "" = no match
	}{
		{"claude", "5-hour limit reached ∙ resets 3pm", SevLimit},
		{"claude", "  Weekly limit reached ∙ resets Nov 8, 9am", SevLimit},
		{"claude", "Opus weekly limit reached ∙ resets 3pm", SevLimit},
		{"claude", "Usage limit reached ∙ resets 3:45pm (Europe/Istanbul)", SevLimit},
		{"claude", "Claude usage limit reached. Your limit will reset at 3pm|1762354800", SevLimit},
		{"claude", "Claude AI usage limit reached|1762354800", SevLimit},
		{"claude", "Limit reached – contact an admin to increase your limits", SevLimit},
		{"claude", "You've hit your weekly limit", SevLimit},
		{"claude", "Approaching 5-hour limit ∙ resets 3pm", SevWarn},
		{"claude", "approaching Opus weekly limit", SevWarn},
		{"claude", "Claude Opus limit reached, now using Sonnet 4", SevIgnore},
		{"claude", "I'll check the rate limit middleware", ""},
		{"claude", "> fix the usage limit counter in billing.go", ""},
		{"gemini", "Usage limit reached for gemini-2.5-pro.", SevLimit},
		{"gemini", "You have exhausted your daily quota on this model.", SevLimit},
		{"gemini", "[API Error: RESOURCE_EXHAUSTED]", SevLimit},
		{"gemini", "Quota exceeded for quota metric", SevLimit},
		{"copilot", "You have exceeded your premium request allowance.", SevLimit},
		{"copilot", "Weekly limit reached", SevLimit},
		{"codex", "■ You've hit your usage limit. Upgrade to Pro or try again in 2 days 3 hours.", SevLimit},
		{"codex", "you have hit the usage limit of the API", ""},
	}
	for _, c := range cases {
		p, ok := set.Match(c.agent, c.line)
		got := Severity("")
		if ok {
			got = p.Severity
			if !strings.HasPrefix(p.ID, string(c.agent)+"."+string(p.Severity)+".") {
				t.Errorf("pattern id %q", p.ID)
			}
		}
		if got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.agent, c.line, got, c.want)
		}
	}
}

func TestConfigPatterns(t *testing.T) {
	cfg := config.Default()
	cfg.Agents["gemini"].Patterns = []config.Pattern{{Re: `my custom stop`, Severity: "limit"}, {Re: `almost out`, Severity: "warn"}}
	set, err := Compile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := set.Match("gemini", "MY CUSTOM STOP here"); !ok || p.Severity != SevLimit {
		t.Fatal("added pattern")
	}
	if _, ok := set.Match("gemini", "Quota exceeded"); !ok {
		t.Fatal("defaults kept when adding")
	}
	cfg.Agents["gemini"].ReplaceDefaultPatterns = true
	set, _ = Compile(cfg)
	if _, ok := set.Match("gemini", "Quota exceeded"); ok {
		t.Fatal("defaults should be replaced")
	}
	if p, ok := set.Match("gemini", "almost out of requests"); !ok || p.Severity != SevWarn {
		t.Fatal("replacement warn pattern")
	}
	cfg.Agents["gemini"].Patterns = []config.Pattern{{Re: `(`, Severity: "limit"}}
	if _, err := Compile(cfg); err == nil {
		t.Fatal("invalid regex accepted")
	}
}
