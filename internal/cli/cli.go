// Package cli implements imux's command-line interface: subcommand dispatch
// with the standard library's flag package, help text and exit codes.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// Exit codes.
const (
	ExitOK      = 0
	ExitError   = 1
	ExitUsage   = 2
	ExitTimeout = 124
)

// Env is the I/O environment of one invocation. Tests replace it.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// command is one subcommand.
type command struct {
	Name    string
	Args    string // argument synopsis for help, e.g. "<agent> [--pane %N]"
	Summary string
	Hidden  bool // internal commands (__exec, __menu, ...)
	Run     func(e *Env, args []string) error
}

var commands = map[string]*command{}

func register(c *command) {
	if _, dup := commands[c.Name]; dup {
		panic("cli: duplicate command " + c.Name)
	}
	commands[c.Name] = c
}

// UsageError makes the process exit with ExitUsage after printing the message.
type UsageError struct{ Msg string }

func (u *UsageError) Error() string { return u.Msg }

// ExitCodeError carries an explicit exit code. Msg may be empty (silent exit).
type ExitCodeError struct {
	Code int
	Msg  string
}

func (x *ExitCodeError) Error() string { return x.Msg }

// Main runs imux with os.Args-style arguments and returns the exit code.
func Main(args []string) int {
	return Run(&Env{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}, args[1:])
}

// Run dispatches args (without the program name) and returns the exit code.
func Run(e *Env, args []string) int {
	setup()
	if len(args) == 0 {
		return finish(e, defaultAction(e, nil))
	}
	name := args[0]
	switch name {
	case "-h", "--help", "help":
		if len(args) > 1 {
			if c, ok := commands[args[1]]; ok {
				printCommandHelp(e.Stdout, c)
				return ExitOK
			}
		}
		printHelp(e.Stdout)
		return ExitOK
	case "-v", "--version":
		name = "version"
	}
	c, ok := commands[name]
	if !ok {
		if isAgentArg(name) || strings.HasPrefix(name, "-") {
			return finish(e, defaultAction(e, args))
		}
		fmt.Fprintf(e.Stderr, "imux: %s\n\n", i18n.T("cli.unknown_command", name))
		printHelp(e.Stderr)
		return ExitUsage
	}
	return finish(e, c.Run(e, args[1:]))
}

func finish(e *Env, err error) int {
	if err == nil {
		return ExitOK
	}
	var ue *UsageError
	if errors.As(err, &ue) {
		fmt.Fprintf(e.Stderr, "imux: %s\n", ue.Msg)
		return ExitUsage
	}
	var xe *ExitCodeError
	if errors.As(err, &xe) {
		if xe.Msg != "" {
			fmt.Fprintf(e.Stderr, "imux: %s\n", xe.Msg)
		}
		return xe.Code
	}
	if errors.Is(err, flag.ErrHelp) {
		return ExitOK
	}
	fmt.Fprintf(e.Stderr, "imux: %v\n", err)
	return ExitError
}

// newFlags returns a FlagSet that reports parse errors as usage errors.
func newFlags(e *Env, c string) *flag.FlagSet {
	fs := flag.NewFlagSet("imux "+c, flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	fs.Usage = func() {
		if cmd, ok := commands[c]; ok {
			printCommandHelp(e.Stderr, cmd)
		}
		fs.PrintDefaults()
	}
	return fs
}

// parseFlags parses flags that may be interleaved with positional arguments.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, &UsageError{Msg: err.Error()}
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		if rest[0] == "--" {
			return append(pos, rest[1:]...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, i18n.T("cli.help.header"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, i18n.T("cli.help.usage"))
	fmt.Fprintln(w, "  imux [agent]")
	fmt.Fprintln(w, "  imux <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, i18n.T("cli.help.commands"))
	names := make([]string, 0, len(commands))
	width := 0
	for n, c := range commands {
		if c.Hidden {
			continue
		}
		names = append(names, n)
		if len(n) > width {
			width = len(n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(w, "  %-*s  %s\n", width, n, i18n.T(commands[n].Summary))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, i18n.T("cli.help.footer"))
}

func printCommandHelp(w io.Writer, c *command) {
	fmt.Fprintf(w, "imux %s %s\n  %s\n", c.Name, c.Args, i18n.T(c.Summary))
}
