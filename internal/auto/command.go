package auto

import (
	"fmt"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// NothingNew is the reply of a run with nothing to report: no notification.
const NothingNew = "IMUX_NOTHING_NEW"

// Native reports whether cli runs def as its own agent (--agent): a
// Claude Code agent with Claude, a Copilot one with Copilot, an OpenCode
// one with OpenCode. Otherwise the prompt carries the instructions.
func Native(d Def, cli session.AgentID) bool {
	return d.Kind == KindAgent && d.Owner == cli && (cli == session.Claude || cli == session.Copilot || cli == session.OpenCode)
}

// PromptInput is what a run's prompt says.
type PromptInput struct {
	Def    Def
	CLI    session.AgentID
	When   string // the schedule's time as the UI says it ("every 5 min"); "" by hand
	Dir    string // the project
	Prev   *Run   // the last run with a report (nil: the first)
	Lang   string // the UI's language for the report ("Turkish")
	Now    time.Time
	Manual bool
}

// Prompt is the run's prompt (English: the model reads it): the
// instructions when the CLI does not run the definition natively, then how
// to run unattended, the last report and how to answer.
func Prompt(in PromptInput) string {
	var b strings.Builder
	d := in.Def
	if !Native(d, in.CLI) {
		switch d.Kind {
		case KindSkill:
			fmt.Fprintf(&b, "Use the Agent Skill %q at %s: read it and follow it.\n\n", d.Name, d.File)
		default:
			_, body, _ := FrontMatter(d.File)
			fmt.Fprintf(&b, "You are the %q agent (%s). Your instructions:\n\n%s\n\n---\n\n", d.Name, d.File, body)
		}
	}
	what := "agent"
	if d.Kind == KindSkill {
		what = "skill"
	}
	if in.Manual || in.When == "" {
		fmt.Fprintf(&b, "i-mux runs this %s now in %s, started by hand, with nobody watching: do the task now without asking questions; decide yourself.\n", what, in.Dir)
	} else {
		fmt.Fprintf(&b, "i-mux runs this %s on a schedule (%s) in %s, with nobody watching: do the task now without asking questions; decide yourself.\n", what, in.When, in.Dir)
	}
	if p := in.Prev; p != nil && p.Report != "" {
		fmt.Fprintf(&b, "\nYour report from the last run (%s):\n%s\n", p.Start.In(in.Now.Location()).Format("2006-01-02 15:04"), p.Report)
	} else {
		b.WriteString("\nThis is the first run.\n")
	}
	lang := in.Lang
	if lang == "" {
		lang = "English"
	}
	fmt.Fprintf(&b, "\nWhen you are done, reply with a short report for the user in %s. Its first line is shown in i-mux's sidebar: make it the news. If there is nothing new to report, reply with exactly %s and nothing else.", lang, NothingNew)
	return b.String()
}

// Invocation is a run's command line.
type Invocation struct {
	Argv    []string // Argv[0] is the binary
	OutFile string   // the reply is read from this file instead of stdout (Codex)
	Session string   // the session id i-mux gave it (Claude)
}

// Command builds the headless command line of cli (binary bin) for def:
// the headless flag first, as the ✨ commit messages (D29) do.
func Command(cli session.AgentID, bin string, d Def, model, prompt string, readOnly bool, outFile, uuid string) (Invocation, error) {
	native := Native(d, cli)
	opt := func(flag, v string) []string {
		if v == "" {
			return nil
		}
		return []string{flag, v}
	}
	agentName := ""
	if native {
		agentName = d.Name
	}
	switch cli {
	case session.Claude:
		a := []string{bin, "-p", "--session-id", uuid}
		a = append(a, opt("--agent", agentName)...)
		a = append(a, opt("--model", model)...)
		if readOnly {
			a = append(a, "--permission-mode", "dontAsk")
		} else {
			a = append(a, "--dangerously-skip-permissions")
		}
		return Invocation{Argv: append(a, prompt), Session: uuid}, nil
	case session.Copilot:
		a := []string{bin, "-p", prompt}
		a = append(a, opt("--agent", agentName)...)
		a = append(a, opt("--model", model)...)
		if !readOnly {
			a = append(a, "--yolo")
		}
		return Invocation{Argv: append(a, "-s", "--no-color")}, nil
	case session.Antigravity:
		a := []string{bin, "-p", prompt}
		a = append(a, opt("--model", model)...)
		if readOnly {
			a = append(a, "--mode", "plan")
		} else {
			a = append(a, "--dangerously-skip-permissions")
		}
		return Invocation{Argv: a}, nil
	case session.Codex:
		a := []string{bin, "exec", "--skip-git-repo-check", "--color", "never", "--output-last-message", outFile}
		a = append(a, opt("-m", model)...)
		if readOnly {
			a = append(a, "--sandbox", "read-only")
		} else {
			a = append(a, "--dangerously-bypass-approvals-and-sandbox")
		}
		return Invocation{Argv: append(a, prompt), OutFile: outFile}, nil
	case session.OpenCode:
		a := []string{bin, "run"}
		a = append(a, opt("--agent", agentName)...)
		a = append(a, opt("-m", model)...)
		return Invocation{Argv: append(a, prompt)}, nil
	}
	return Invocation{}, fmt.Errorf("%s cannot run on a schedule", cli)
}
