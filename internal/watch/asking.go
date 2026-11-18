package watch

import (
	"regexp"
	"strings"
)

// askingLines is how many of a screen's last non-blank lines are checked
// for a question.
const askingLines = 12

// askingPatterns are the questions agent CLIs ask before they go on: a
// permission prompt, a numbered choice, a yes/no prompt (§9.3 "Sidebar v4",
// "Needs you"). They are checked only on a screen that stopped changing.
var askingPatterns = []*regexp.Regexp{
	// Claude Code, Copilot: "Do you want to proceed?", "Do you want to make
	// this edit to a.go?", "Do you want to run this command?"
	regexp.MustCompile(`(?i)\bdo you want to (proceed|make|create|run|allow|use|fetch|edit|delete|overwrite|continue)\b.*\?`),
	// Codex: "Would you like to run the following command?"
	regexp.MustCompile(`(?i)\bwould you like to (run|make|apply|proceed|allow|continue)\b`),
	// A numbered choice whose first option is Yes ("❯ 1. Yes").
	regexp.MustCompile(`^\s*[❯›>▸]?\s*1[.)]\s+Yes\b`),
	// Yes/no prompts.
	regexp.MustCompile(`\((?i:y/n)\)|\[(?:y/N|Y/n|y/n)\]`),
	// "Allow this action?", "Allow once" / "Allow always" choices.
	regexp.MustCompile(`(?i)\ballow (this|once|always|for this session)\b`),
}

// Asking reports whether the bottom of a screen asks the user something.
func Asking(screen string) bool {
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(screen, " \t\r\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > askingLines {
		lines = lines[len(lines)-askingLines:]
	}
	for _, l := range lines {
		for _, re := range askingPatterns {
			if re.MatchString(l) {
				return true
			}
		}
	}
	return false
}
