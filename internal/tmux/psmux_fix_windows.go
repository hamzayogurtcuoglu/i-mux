//go:build windows

package tmux

// Work-arounds for psmux behaviors that differ from tmux: pane borders show
// only #{pane_title}, split-window has no -b, and resize-pane -x/-y acts on
// the active pane and takes a share of the window, not cells.

import (
	"context"
	"strconv"
	"strings"
)

var labelOpts = map[string]bool{"@imux_label": true, "@imux_name": true, "@imux_flag": true}

// labelPanes lists the panes whose label options cmds change.
func labelPanes(cmds []Cmd) []string {
	var ps []string
	for _, cmd := range cmds {
		if cmd[0] != "set-option" && cmd[0] != "set" {
			continue
		}
		t := valueOf(cmd, "-t")
		if t == "" || !hasFlag(cmd, 'p') {
			continue
		}
		for _, a := range cmd[1:] {
			if labelOpts[a] {
				ps = append(ps, t)
				break
			}
		}
	}
	return ps
}

// refreshTitles writes imux's border label as the panes' titles:
// " name · label flag ", as pane-border-format does on tmux.
func (c *Client) refreshTitles(ctx context.Context, route string, panes []string) {
	seen := map[string]bool{}
	for _, p := range panes {
		if seen[p] {
			continue
		}
		seen[p] = true
		o, err := c.execRoute(ctx, route, []Cmd{{"display-message", "-p", "-t", p,
			"#{@imux_name}\t#{@imux_label}\t#{@imux_flag}\t#{pane_current_command}"}})
		f := strings.Split(strings.TrimRight(o, "\r\n"), "\t")
		if err != nil || len(f) != 4 {
			continue
		}
		title := f[1]
		if title == "" {
			title = f[3]
		}
		if f[0] != "" {
			title = f[0] + " · " + title
		}
		if f[2] != "" {
			title += " " + f[2]
		}
		_, _ = c.execRoute(ctx, route, []Cmd{{"select-pane", "-t", p, "-T", title}})
	}
}

// splitBefore is a split-window -b: psmux has no -b, so the new pane is
// moved before its target afterwards and given its size again.
type splitBefore struct {
	target     string // the pane split (psmux id; "" = the active one)
	size       string // -l value ("" = none)
	horizontal bool
}

// takeBefore removes -b from a split-window and says what to do after it.
func takeBefore(cmd Cmd) (Cmd, *splitBefore) {
	if cmd[0] != "split-window" && cmd[0] != "splitw" {
		return cmd, nil
	}
	i := indexOf(cmd, "-b")
	if i < 0 || i > flagsEnd(cmd) {
		return cmd, nil
	}
	out := append(append(Cmd(nil), cmd[:i]...), cmd[i+1:]...)
	return out, &splitBefore{target: valueOf(out, "-t"), size: valueOf(out, "-l"), horizontal: indexOf(out, "-h") > 0}
}

// flagsEnd is the index of the first word after cmd's flags.
func flagsEnd(cmd Cmd) int {
	i := 1
	for i < len(cmd) {
		a := cmd[i]
		if a == "--" || len(a) < 2 || a[0] != '-' {
			return i
		}
		if len(a) == 2 && valueFlag(cmd[0], a[1]) {
			i += 2
			continue
		}
		i++
	}
	return i
}

// moveBefore puts a new pane before (left of, or above) the pane it split.
func (c *Client) moveBefore(ctx context.Context, session, pane string, b *splitBefore) {
	target := b.target
	if target == "" {
		out, err := c.execRoute(ctx, session, []Cmd{{"display-message", "-p", "#{pane_id}"}})
		if err != nil {
			return
		}
		target = strings.TrimSpace(out)
	}
	cmds := []Cmd{{"swap-pane", "-d", "-s", pane, "-t", target}}
	if b.size != "" {
		axis := "-y"
		if b.horizontal {
			axis = "-x"
		}
		cmds = append(cmds, Cmd{"resize-pane", "-t", pane, axis, b.size})
	}
	_, _ = c.execRoute(ctx, session, c.targetedResizes(ctx, session, cmds))
}

// targetedResizes makes resize-pane -t P -x/-y act on P: psmux resizes the
// active pane whatever -t says, so P is made active for the resize and the
// active pane is given back afterwards.
func (c *Client) targetedResizes(ctx context.Context, route string, cmds []Cmd) []Cmd {
	var out []Cmd
	for _, cmd := range cmds {
		if (cmd[0] != "resize-pane" && cmd[0] != "resizep") || indexOf(cmd, "-t") < 0 ||
			(indexOf(cmd, "-x") < 0 && indexOf(cmd, "-y") < 0) {
			out = append(out, cmd)
			continue
		}
		t := valueOf(cmd, "-t")
		info, err := c.execRoute(ctx, route, []Cmd{{"display-message", "-p", "-t", t, "#{window_id}\t#{window_width}\t#{window_height}"}})
		f := strings.Split(strings.TrimSpace(info), "\t")
		if err != nil || len(f) != 3 {
			out = append(out, cmd)
			continue
		}
		active, _ := c.execRoute(ctx, route, []Cmd{{"list-panes", "-t", f[0], "-F", "#{?pane_active,#{pane_id},}"}})
		prev := strings.TrimSpace(active)
		out = append(out, Cmd{"select-pane", "-t", t}, percentResize(cmd, f[1], f[2]))
		if prev != "" && prev != t {
			out = append(out, Cmd{"select-pane", "-t", prev})
		}
	}
	return out
}

// percentResize turns resize-pane's -x / -y cells into the share of the
// window psmux takes the number as.
func percentResize(cmd Cmd, w, h string) Cmd {
	out := append(Cmd(nil), cmd...)
	for i := 1; i+1 < len(out); i++ {
		total := ""
		switch out[i] {
		case "-x":
			total = w
		case "-y":
			total = h
		default:
			continue
		}
		cells, err1 := strconv.Atoi(out[i+1])
		t, err2 := strconv.Atoi(total)
		if err1 != nil || err2 != nil || t <= 1 {
			continue
		}
		pct := (cells*100 + (t-1)/2) / (t - 1)
		out[i+1] = strconv.Itoa(max(1, min(99, pct)))
	}
	return out
}

// askNewPane makes the creating command print the new pane's id first.
