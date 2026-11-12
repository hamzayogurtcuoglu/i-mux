package sidebar

// Icons are the sidebar's button icons in one theme (§9.3 "Buttons"): the
// editor's codicons from a Nerd Font, or emoji, which every font draws at
// full size.
type Icons struct {
	Add, Remove, Discard, Refresh string
	Pull, Push, Sync, Commit      string
	Branch, Open, Closed          string // Open / Closed: a section's or folder's arrow
	Edit, History, Search         string
	Suggest                       string // an agent writes the commit message
	GoTo                          string // Go to File
	More                          string // ⋯ More Actions
	Publish                       string // publish a branch with no upstream
	Run, Stop, Schedule           string // the Scheduler's: run now, stop the run, schedule (D61)
	Skill, Robot                  string // a skill, an agent any CLI runs (the Scheduler's rows)
}

var buttonIcons = map[string]Icons{
	// Codicons (Nerd Fonts v3): add, remove, discard, refresh, arrow-down,
	// arrow-up, sync, check, git branch, chevron-down, chevron-right, edit,
	// history, search, sparkle (Nerd Fonts 3.2+), go-to-file, ellipsis,
	// cloud-upload, play, debug-stop, watch, extensions, hubot.
	IconsNerd: {
		Add: "\uea60", Remove: "\ueb3b", Discard: "\ueae2", Refresh: "\ueb37",
		Pull: "\uea9a", Push: "\ueaa1", Sync: "\uea77", Commit: "\ueab2",
		Branch: "\ue725", Open: "\ueab4", Closed: "\ueab6",
		Edit: "\uea73", History: "\uea82", Search: "\uea6d",
		Suggest: "\uec10", GoTo: "\uea94", More: "\uea7c", Publish: "\ueac3",
		Run: "\ueb2c", Stop: "\uead7", Schedule: "\ueb7c", Skill: "\ueae6", Robot: "\ueb08",
	},
	IconsEmoji: {
		Add: "➕", Remove: "➖", Discard: "⏪", Refresh: "🔄",
		Pull: "⏬", Push: "⏫", Sync: "🔃", Commit: "✅",
		Branch: "🌿", Open: "▾", Closed: "▸",
		Edit: "📝", History: "📜", Search: "🔍",
		Suggest: "✨", GoTo: "📄", More: "⋯", Publish: "📤",
		Run: "🚀", Stop: "🛑", Schedule: "⏰", Skill: "🧩", Robot: "🤖",
	},
}

// icons returns the button icons of the model's theme.
func (m Model) icons() Icons {
	if ic, ok := buttonIcons[m.Icons]; ok {
		return ic
	}
	return buttonIcons[IconsEmoji]
}

// fold is the arrow of a section or folder.
func (ic Icons) fold(folded bool) string {
	if folded {
		return ic.Closed
	}
	return ic.Open
}

// button is a button cell: the icon with a space on each side, so it takes
// at least 3 columns to see and to hit.
func button(icon, color string, a Action) Cell {
	return Cell{W: DisplayWidth(icon) + 2, Text: " " + color + icon + reset + " ", Action: a}
}

// width sums the cells' widths.
func width(cells []Cell) int {
	n := 0
	for _, c := range cells {
		n += c.W
	}
	return n
}

// ButtonIcons are the button icons of theme (the popups' titles).
func ButtonIcons(theme string) Icons {
	if ic, ok := buttonIcons[theme]; ok {
		return ic
	}
	return buttonIcons[IconsEmoji]
}
