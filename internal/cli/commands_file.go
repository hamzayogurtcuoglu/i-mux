package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/editor"
	"github.com/hamzayogurtcuoglu/i-mux/internal/fileops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:   "__file",
		Args:   "<op> [--client C] [--pane P]",
		Hidden: true,
		Run:    runFile,
	})
}

// fileCtx is what an explorer menu's operation works on.
type fileCtx struct {
	ctx    context.Context
	w      *workspace.WS
	client string
	pane   string // the sidebar's target pane
	path   string // sidebar.OptFile
	root   string // sidebar.OptFileRoot
	name   string // sidebar.OptFileName (a prompt's answer)
	clip   string // sidebar.OptFileClip
	cmp    string // sidebar.OptCompare
	sys    *fileops.System
}

// runFile does what an explorer menu item asks (§9.3 "Explorer menus and
// hover"). The path comes from the options the sidebar set.
func runFile(e *Env, args []string) error {
	fs := newFlags(e, "__file")
	clientF := fs.String("client", "", "client for messages, prompts and popups")
	paneF := fs.String("pane", "", "the target pane (terminal, side, add)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__file <op>")}
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	f := &fileCtx{ctx: context.Background(), w: w, client: *clientF, pane: *paneF, sys: &fileops.System{}}
	out, err := w.T.Run(f.ctx,
		tmux.DelimCmd("file"), tmux.Cmd{"show-options", "-gqv", sidebar.OptFile},
		tmux.DelimCmd("root"), tmux.Cmd{"show-options", "-gqv", sidebar.OptFileRoot},
		tmux.DelimCmd("name"), tmux.Cmd{"show-options", "-gqv", sidebar.OptFileName},
		tmux.DelimCmd("clip"), tmux.Cmd{"show-options", "-gqv", sidebar.OptFileClip},
		tmux.DelimCmd("cmp"), tmux.Cmd{"show-options", "-gqv", sidebar.OptCompare})
	if err != nil {
		return err
	}
	sec := tmux.Sections(out)
	val := func(k string) string { return strings.TrimRight(sec[k], "\n") }
	f.path, f.root, f.name, f.clip, f.cmp = val("file"), val("root"), val("name"), val("clip"), val("cmp")
	if f.path == "" || f.root == "" {
		return errors.New("no file chosen")
	}
	err = f.do(pos[0])
	if err != nil {
		f.say(i18n.T("file.failed", err.Error()))
	}
	return err
}

// isDir reports whether the menu's path is a folder.
func (f *fileCtx) isDir() bool {
	fi, err := os.Stat(f.path)
	return err == nil && fi.IsDir()
}

// dir is the folder new files and pastes go to: the path, or its folder.
func (f *fileCtx) dir() string {
	if f.isDir() {
		return f.path
	}
	return filepath.Dir(f.path)
}

func (f *fileCtx) rel(p string) string {
	r, err := filepath.Rel(f.root, p)
	if err != nil {
		return p
	}
	return r
}

func (f *fileCtx) say(msg string) { f.w.Message(f.ctx, f.client, f.pane, msg) }

func (f *fileCtx) self(args ...string) string {
	return tmux.ShJoin(append([]string{f.w.Self}, args...)...)
}

// popup runs an imux command in a popup in folder dir.
func (f *fileCtx) popup(dir string, args ...string) error {
	return f.popupSized(dir, "90%", "90%", args...)
}

// popupSized is popup w wide and h high.
func (f *fileCtx) popupSized(dir, w, h string, args ...string) error {
	c := tmux.Cmd{"display-popup"}
	if f.client != "" {
		c = append(c, "-c", f.client)
	}
	c = append(c, "-E", "-w", w, "-h", h, "-d", tmux.EscapeFormat(dir), f.self(args...))
	return f.w.T.Spawn(c)
}

// prompt asks on the client for a name; the answer goes to OptFileName,
// then `imux __file <then>` runs.
func (f *fileCtx) prompt(label, initial, then string) error {
	c := tmux.Cmd{"command-prompt"}
	if f.client != "" {
		c = append(c, "-t", f.client)
	}
	// -I is a comma-separated list (one per prompt): a name with a comma
	// would be cut, so it starts empty then.
	if initial != "" && !strings.Contains(initial, ",") {
		c = append(c, "-I", initial)
	}
	// The template turns "%1" into the answer, so no pane id goes in it
	// (%14 would become "<answer>4"); naming needs no pane.
	tpl := "set-option -g " + sidebar.OptFileName + " \"%%%\" ; " + tmux.RunShell(f.w.Self, f.again(then)...)
	_, err := f.w.T.Run(f.ctx, append(c, "-p", label, tpl))
	return err
}

// reveal makes the sidebars open the folders above p and select it.
func (f *fileCtx) reveal(p string) {
	f.signal("reveal", p)
}

func (f *fileCtx) signal(op, p string) {
	v := fmt.Sprintf("%d:%s:%s", time.Now().UnixNano(), op, p)
	_, _ = f.w.T.Run(f.ctx, tmux.Cmd{"set-option", "-g", sidebar.OptReveal, v})
	sidebar.RefreshAll(f.ctx, f.w.T)
}

func (f *fileCtx) do(op string) error {
	ctx := f.ctx
	switch op {
	case "open":
		return f.popup(f.root, "__edit", f.rel(f.path))
	case "view":
		return f.popup(f.root, "__view", f.rel(f.path))
	case "side":
		// The editor in a new pane beside the target.
		c := tmux.Cmd{"split-window", "-h"}
		if f.pane != "" {
			c = append(c, "-t", f.pane)
		}
		_, err := f.w.T.Run(ctx, append(c, "-c", tmux.EscapeFormat(f.root), f.self("__edit", f.path)))
		return err
	case "preview":
		return webOpen(ctx, f.w, f.path, f.pane, f.client, "", false)
	case "preview-window":
		return webOpen(ctx, f.w, f.path, f.pane, f.client, "window", false)
	case "preview-tab":
		return webOpen(ctx, f.w, f.path, f.pane, f.client, "tab", false)
	case "vscode":
		code := editor.VSCode()
		if code == "" {
			return errors.New("VS Code not found")
		}
		return startDetached(code, "-g", f.path)
	case "add":
		return f.addToAgent()
	case "reveal":
		return f.sys.RevealFile(ctx, f.path)
	case "terminal":
		c := tmux.Cmd{"split-window", "-v", "-l", "30%"}
		if f.pane != "" {
			c = append(c, "-t", f.pane)
		}
		_, err := f.w.T.Run(ctx, append(c, "-c", tmux.EscapeFormat(f.dir())))
		return err
	case "find":
		return f.popup(f.dir(), "__search")
	case "compare-select":
		_, err := f.w.T.Run(ctx, tmux.Cmd{"set-option", "-g", sidebar.OptCompare, f.path})
		if err == nil {
			f.say(i18n.T("file.compare_selected", filepath.Base(f.path)))
		}
		return err
	case "compare":
		if f.cmp == "" {
			return errors.New("no file selected for compare")
		}
		c := tmux.Cmd{"display-popup"}
		if f.client != "" {
			c = append(c, "-c", f.client)
		}
		return f.w.T.Spawn(append(c, "-E", "-w", "95%", "-h", "90%", "-d", tmux.EscapeFormat(f.root),
			f.self("__diff", "--compare", f.cmp, f.path)))
	case "cut", "copy":
		_, err := f.w.T.Run(ctx, tmux.Cmd{"set-option", "-g", sidebar.OptFileClip, op + ":" + f.path})
		if err == nil {
			msg := i18n.T("file.copy_done", filepath.Base(f.path))
			if op == "cut" {
				msg = i18n.T("file.cut_done", filepath.Base(f.path))
			}
			f.say(msg)
		}
		return err
	case "paste":
		kind, src, ok := strings.Cut(f.clip, ":")
		if !ok || src == "" {
			return errors.New("nothing to paste")
		}
		to, err := fileops.Paste(src, f.dir(), kind == "cut")
		if err != nil {
			return err
		}
		if kind == "cut" {
			_, _ = f.w.T.Run(ctx, tmux.Cmd{"set-option", "-gu", sidebar.OptFileClip})
		}
		f.reveal(to)
		return nil
	case "copy-path", "copy-relative-path":
		text := f.path
		if op == "copy-relative-path" {
			text = f.rel(f.path)
		}
		// The tmux buffer, sent on to the terminal's clipboard (OSC 52:
		// over SSH too), and the system clipboard.
		_, _ = f.w.T.Run(ctx, tmux.Cmd{"set-buffer", "-w", "--", text})
		_ = f.sys.CopyText(ctx, text)
		f.say(i18n.T("file.copied", text))
		return nil
	case "new-file", "new-folder":
		label := "file.new_file_prompt"
		if op == "new-folder" {
			label = "file.new_folder_prompt"
		}
		return f.prompt(i18n.T(label), "", op+"-done")
	case "new-file-done", "new-folder-done":
		p, err := fileops.Create(f.dir(), f.name, op == "new-folder-done")
		if err != nil {
			return err
		}
		f.reveal(p)
		if op == "new-file-done" {
			return f.popup(f.root, "__edit", f.rel(p))
		}
		return nil
	case "rename":
		return f.prompt(i18n.T("file.rename_prompt"), filepath.Base(f.path), "rename-done")
	case "rename-done":
		p, err := fileops.Rename(f.path, f.name)
		if err != nil {
			return err
		}
		_, _ = f.w.T.Run(ctx, tmux.Cmd{"set-option", "-g", sidebar.OptFile, p})
		f.reveal(p)
		return nil
	case "delete":
		name := filepath.Base(f.path)
		m := &workspace.Menu{
			Title: i18n.T("file.delete_title", name),
			Items: []workspace.MenuItem{
				{Name: i18n.T("file.delete_text"), Disabled: true},
				workspace.Separator(),
				{Name: i18n.T("file.trash"), Key: "y", Cmd: tmux.RunShell(f.w.Self, f.again("trash")...)},
				{Name: i18n.T("click.cancel"), Key: "n"},
			},
		}
		return f.w.Show(ctx, m, f.client, "")
	case "trash":
		if err := f.sys.Trash(ctx, f.path); err != nil {
			m := &workspace.Menu{
				Title: i18n.T("file.trash_failed_title"),
				Items: []workspace.MenuItem{
					{Name: i18n.T("file.trash_failed", firstLine(err.Error())), Disabled: true},
					workspace.Separator(),
					{Name: i18n.T("file.delete_permanently"), Key: "d", Cmd: tmux.RunShell(f.w.Self, f.again("delete-permanently")...)},
					{Name: i18n.T("click.cancel"), Key: "n"},
				},
			}
			return f.w.Show(ctx, m, f.client, "")
		}
		f.done()
		return nil
	case "delete-permanently":
		if err := os.RemoveAll(f.path); err != nil {
			return err
		}
		f.done()
		return nil
	case "collapse":
		f.signal("collapse", f.root)
		return nil
	// The explorer title's ⋯ (D63).
	case "refresh":
		f.signal("refresh", f.root)
		return nil
	case "goto":
		return f.popupSized(f.root, "80%", "70%", "__goto", "--run")
	case "show-hidden", "hide-hidden":
		// Saved as a Settings row is: every sidebar reads it again.
		v := "on"
		if op == "hide-hidden" {
			v = "off"
		}
		if err := setSetting("hidden_files", v); err != nil {
			return err
		}
		sidebar.RefreshAll(ctx, f.w.T)
		return nil
	}
	return &UsageError{Msg: i18n.T("cli.usage", "__file <op>")}
}

// again is the arguments of another __file operation for the same client.
func (f *fileCtx) again(op string) []string {
	args := []string{"__file", op}
	if f.client != "" {
		args = append(args, "--client", f.client)
	}
	return args
}

// done redraws the sidebars after a file went.
func (f *fileCtx) done() {
	_, _ = f.w.T.Run(f.ctx, tmux.Cmd{"set-option", "-gu", sidebar.OptCompare})
	sidebar.RefreshAll(f.ctx, f.w.T)
}

// addToAgent pastes the path into the target agent, as a dropped file
// arrives, and goes to it.
func (f *fileCtx) addToAgent() error {
	if f.pane == "" {
		return errors.New("no agent")
	}
	out, err := f.w.T.Run(f.ctx, tmux.Cmd{"display-message", "-p", "-t", f.pane, "#{@imux_agent}"})
	if err != nil {
		return err
	}
	agent := strings.TrimSpace(out)
	if agent == "" {
		return errors.New("no agent")
	}
	text := ImageText(session.AgentID(agent), []string{f.path})
	_, err = f.w.T.Run(f.ctx,
		tmux.Cmd{"set-buffer", "-b", "imux-file", "--", text},
		tmux.Cmd{"paste-buffer", "-p", "-d", "-b", "imux-file", "-t", f.pane},
		tmux.Cmd{"select-pane", "-t", f.pane})
	return err
}

// startDetached starts a program without waiting for it.
func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
