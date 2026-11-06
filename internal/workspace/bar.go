package workspace

import (
	"strings"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Agent-bar ranges (§9.3). tmux limits a range argument to 15 bytes.
const (
	RangePrefix   = "imux:"
	RangeSessions = "imux:sessions"
	RangeMenu     = "imux:menu"
	RangeCur      = "imux:cur"
	MaxRangeBytes = 15
)

// shortNames are used when the bar would not fit (Q29).
var shortNames = map[session.AgentID]string{"claude": "Cl", "antigravity": "Ag", "gemini": "Ge", "copilot": "Cp", "codex": "Cx"}

// limitMarkerWidth is the room kept for a "⛔15:00 " marker per agent.
const limitMarkerWidth = 8

// BarAgent is one agent shown on the bar.
type BarAgent struct {
	ID        session.AgentID
	Installed bool
}

// AgentBar renders status-format[1] (§9.3): the agent bar for the active
// pane, highlighted by tmux formats alone (no imux process on focus
// changes). With clickable, every region is a `range=user|imux:<x>`.
//
// Overflow (Q29): when the client is narrower than the full bar, entries of
// agents that are not installed are dropped; when still too narrow, short
// names are used.
func AgentBar(agents []BarAgent, clickable bool) string {
	full, wFull := barVariant(agents, clickable, true, false)
	mid, wMid := barVariant(agents, clickable, false, false)
	short, _ := barVariant(agents, clickable, false, true)
	return "#[align=left]" + "#{?#{e|<:#{client_width}," + itoa(wFull) + "}," +
		"#{?#{e|<:#{client_width}," + itoa(wMid) + "}," + short + "," + mid + "}," + full + "}"
}

// barVariant renders one layout and returns it with its estimated width.
func barVariant(agents []BarAgent, clickable, withMissing, short bool) (string, int) {
	var b strings.Builder
	width := 0
	text := func(s string) {
		b.WriteString(s)
		width += displayWidth(s)
	}
	region := func(rng string, body func()) {
		if clickable {
			b.WriteString("#[range=user|" + rng + "]")
		}
		body()
		if clickable {
			b.WriteString("#[norange]")
		}
	}
	sep := func() { text("│") }
	text(" i-mux ")
	for _, a := range agents {
		info := agent.MustGet(a.ID)
		name := info.Short
		if short {
			name = shortNames[a.ID]
		}
		if !a.Installed {
			if !withMissing {
				continue
			}
			sep()
			region(RangePrefix+string(a.ID), func() {
				b.WriteString("#[dim]")
				text(" " + name + " " + i18n.T("menu.not_installed") + " ")
				b.WriteString("#[nodim]")
			})
			continue
		}
		sep()
		id := string(a.ID)
		region(RangePrefix+id, func() {
			// Active pane's agent: reverse video with a ▶ marker.
			b.WriteString("#{?#{==:#{@imux_agent}," + id + "},#[reverse] ▶ " + name + " #[noreverse], " + name + " }")
			width += displayWidth(" " + name + " ")
			b.WriteString("#{?@imux_lim_" + id + ",#{@imux_lim_" + id + "} ,}")
			width += limitMarkerWidth
		})
	}
	sep()
	region(RangeSessions, func() {
		if short {
			text(" ⧉ ")
		} else {
			text(" ⧉ " + i18n.T("bar.sessions") + " ")
		}
	})
	sep()
	region(RangeMenu, func() { text(" ☰ ") })
	return b.String(), width + 2
}

// displayWidth estimates terminal columns: one per rune, two for emoji
// presentation characters (⛔).
func displayWidth(s string) int {
	n := utf8.RuneCountInString(s)
	for _, r := range s {
		if r == '⛔' {
			n++
		}
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

// barAgents lists the agents in config order with their install state.
func (w *WS) barAgents() []BarAgent {
	var out []BarAgent
	for _, a := range agent.Ordered(w.Cfg) {
		out = append(out, BarAgent{ID: a.ID, Installed: w.Installed(a.ID)})
	}
	return out
}

func init() {
	agentBar = func(w *WS, v tmux.Version) string {
		if !w.Cfg.Tmux.AgentBar {
			return ""
		}
		clickable := w.Cfg.Tmux.Mouse && v.AtLeast(tmux.ClickMinVersion[0], tmux.ClickMinVersion[1])
		return AgentBar(w.barAgents(), clickable)
	}
}
