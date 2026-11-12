package auto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ItemKind is what an item of a run's output is.
type ItemKind int

// Items of a run's output.
const (
	ItemText   ItemKind = iota // the agent's words
	ItemTool                   // a tool it called: Name, and what with (Text)
	ItemResult                 // what a tool gave back: its first line (Text), More lines, Failed
	ItemNote                   // the CLI's own notice (the model, a limit)
	ItemError                  // an error the CLI reported
	ItemPlain                  // a line that is no event: a text CLI's output, or a warning
)

// Item is one thing a run's output says.
type Item struct {
	Kind   ItemKind
	Name   string
	Text   string
	More   int  // a result's other lines
	Failed bool // a tool that failed
	step   string
}

// Output is a run's output as the agent CLI streamed it (§9.3
// "Scheduler"): Claude's stream-json, Copilot's JSONL, Antigravity's
// stream-json, or plain text from the others.
type Output struct {
	Items    []Item
	Report   string // the agent's final answer
	Streamed bool   // events of a CLI's JSON stream were read
	Done     bool   // the stream's final event came
	Failed   bool   // the CLI said the run failed
	Error    string // why
	Session  string // its conversation's id, to continue it
	Model    string // the model that ran
	Dir      string // the project: tool paths are shown relative to it

	partial  []byte
	lastText string
}

// ParseOutput reads a whole output.
func ParseOutput(raw, dir string) *Output {
	o := &Output{Dir: dir}
	o.Feed([]byte(raw))
	o.Flush()
	return o
}

// Feed reads more output: its whole lines (the rest waits for its end).
func (o *Output) Feed(b []byte) {
	o.partial = append(o.partial, b...)
	for {
		i := bytes.IndexByte(o.partial, '\n')
		if i < 0 {
			return
		}
		o.line(string(o.partial[:i]))
		o.partial = o.partial[i+1:]
	}
}

// Flush reads the last line, which no newline ended.
func (o *Output) Flush() {
	if len(o.partial) > 0 {
		o.line(string(o.partial))
		o.partial = nil
	}
}

func (o *Output) line(l string) {
	l = strings.TrimRight(l, "\r")
	if t := strings.TrimSpace(l); strings.HasPrefix(t, "{") {
		var m map[string]any
		if json.Unmarshal([]byte(t), &m) == nil && o.event(m) {
			o.Streamed = true
			return
		}
	}
	o.Items = append(o.Items, Item{Kind: ItemPlain, Text: Clean(l)})
}

// event reads one event; it reports whether it was one of a known stream.
func (o *Output) event(m map[string]any) bool {
	if ev, ok := m["event"].(string); ok {
		return o.agy(ev, m)
	}
	t, _ := m["type"].(string)
	switch {
	case t == "":
		return false
	case m["data"] != nil || strings.Contains(t, ".") || t == "result" && m["exitCode"] != nil:
		return o.copilot(t, obj(m["data"]), m)
	}
	return o.claude(t, m)
}

// claude reads Claude Code's stream-json (claude -p --output-format
// stream-json --verbose).
func (o *Output) claude(t string, m map[string]any) bool {
	switch t {
	case "system":
		if str(m["subtype"]) == "init" {
			o.Session = first(str(m["session_id"]), o.Session)
			o.Model = first(str(m["model"]), o.Model)
		}
	case "assistant":
		for _, b := range list(obj(m["message"])["content"]) {
			blk := obj(b)
			switch str(blk["type"]) {
			case "text":
				o.text(str(blk["text"]))
			case "tool_use":
				o.tool(str(blk["name"]), obj(blk["input"]), "")
			}
		}
	case "user":
		for _, b := range list(obj(m["message"])["content"]) {
			if blk := obj(b); str(blk["type"]) == "tool_result" {
				o.result(content(blk["content"]), blk["is_error"] == true, "")
			}
		}
	case "result":
		o.Done = true
		o.Report = str(m["result"])
		o.Session = first(str(m["session_id"]), o.Session)
		if sub := str(m["subtype"]); m["is_error"] == true || sub != "" && sub != "success" {
			o.Failed = true
			o.Error = first(FirstLine(o.Report), strings.ReplaceAll(sub, "_", " "))
		}
	case "rate_limit_event":
		info := obj(m["rate_limit_info"])
		if strings.HasPrefix(str(info["status"]), "rejected") {
			msg := "usage limit reached"
			if at, ok := info["resetsAt"].(float64); ok && at > 0 {
				msg += " until " + time.Unix(int64(at), 0).Format("Jan 2 15:04")
			}
			o.Items = append(o.Items, Item{Kind: ItemError, Text: msg})
		}
	}
	return true
}

// copilot reads GitHub Copilot CLI's JSONL (copilot -p --output-format
// json): its session events.
func (o *Output) copilot(t string, d, m map[string]any) bool {
	switch t {
	case "session.start":
		o.Session = first(str(d["sessionId"]), o.Session)
		o.Model = first(str(d["selectedModel"]), o.Model)
	case "assistant.message":
		if c := str(d["content"]); strings.TrimSpace(c) != "" {
			o.text(c)
		}
	case "tool.execution_start":
		o.tool(str(d["toolName"]), obj(d["arguments"]), str(d["toolCallId"]))
	case "tool.execution_complete":
		res := obj(d["result"])
		text := str(res["content"])
		ok := d["success"] == true
		if !ok {
			text = first(str(obj(d["error"])["message"]), str(d["error"]), text)
		}
		o.result(text, !ok, str(d["toolCallId"]))
	case "session.error":
		msg := first(str(d["message"]), str(d["errorType"]), "error")
		o.Items = append(o.Items, Item{Kind: ItemError, Text: msg})
		o.Error = first(o.Error, msg)
	case "abort":
		o.Items = append(o.Items, Item{Kind: ItemNote, Text: first(str(d["reason"]), "aborted")})
	case "result":
		o.Done = true
		o.Report = o.lastText
		o.Session = first(str(m["sessionId"]), o.Session)
		if code, ok := m["exitCode"].(float64); ok && code != 0 {
			o.Failed = true
			o.Error = first(o.Error, "exit "+strconv.Itoa(int(code)))
		}
	}
	return true
}

// agy reads Antigravity's stream-json (agy -p --output-format
// stream-json): steps that start and end, the answer in deltas.
func (o *Output) agy(ev string, m map[string]any) bool {
	switch ev {
	case "init":
		o.Session = first(str(m["conversation_id"]), o.Session)
		o.Model = first(str(obj(m["init"])["model"]), o.Model)
	case "step_update":
		su := obj(m["step_update"])
		step := fmt.Sprint(su["step_index"])
		switch str(su["step_type"]) {
		case "tool":
			info := obj(su["tool_info"])
			name := first(str(su["tool_name"]), str(info["name"]))
			if !o.hasStep(ItemTool, step) {
				o.tool(name, obj(info["parameters"]), step)
			}
			if st := str(su["state"]); st != "" && st != "ACTIVE" && !o.hasStep(ItemResult, step) {
				text, failed := agyOutput(str(info["output"]))
				o.result(text, failed || st == "ERROR" || st == "FAILED", step)
			}
		case "agent_response":
			if d := str(su["text_delta"]); d != "" {
				o.delta(step, d)
			}
		}
	case "result":
		r := obj(m["result"])
		o.Done = true
		o.Report = first(str(r["response"]), o.lastText)
		o.Session = first(str(r["conversation_id"]), o.Session)
		if st := str(r["status"]); st != "" && st != "SUCCESS" {
			o.Failed = true
			o.Error = first(FirstLine(str(r["error"])), strings.ToLower(st))
		}
	case "error":
		msg := first(str(m["message"]), str(obj(m["error"])["message"]), "error")
		o.Items = append(o.Items, Item{Kind: ItemError, Text: msg})
		o.Error = first(o.Error, msg)
	}
	return true
}

var agyExit = regexp.MustCompile(`Command exited with code (-?\d+)`)

// agyOutput is what one of Antigravity's tools gave: a command's own
// output, not the times around it.
func agyOutput(s string) (string, bool) {
	failed := false
	if m := agyExit.FindStringSubmatch(s); m != nil && m[1] != "0" {
		failed = true
	}
	if _, out, ok := strings.Cut(s, "Command output:"); ok {
		s = out
	}
	return s, failed
}

func (o *Output) hasStep(k ItemKind, step string) bool {
	for i := len(o.Items) - 1; i >= 0; i-- {
		if o.Items[i].Kind == k && o.Items[i].step == step {
			return true
		}
	}
	return false
}

// text adds the agent's words.
func (o *Output) text(s string) {
	s = strings.Trim(Clean(s), "\n")
	if strings.TrimSpace(s) == "" {
		return
	}
	o.lastText = s
	o.Items = append(o.Items, Item{Kind: ItemText, Text: s})
}

// delta adds words to step's text, as they come.
func (o *Output) delta(step, d string) {
	if n := len(o.Items); n > 0 && o.Items[n-1].Kind == ItemText && o.Items[n-1].step == step {
		o.Items[n-1].Text += d
	} else {
		o.Items = append(o.Items, Item{Kind: ItemText, Text: d, step: step})
	}
	last := &o.Items[len(o.Items)-1]
	o.lastText = strings.Trim(last.Text, "\n")
}

// tool adds a tool call: its name and what it was called with.
func (o *Output) tool(name string, args map[string]any, step string) {
	o.Items = append(o.Items, Item{Kind: ItemTool, Name: name, Text: o.toolArg(args), step: step})
}

// result adds what a tool gave back: its first line, and how many more.
func (o *Output) result(s string, failed bool, step string) {
	var lines []string
	for _, l := range strings.Split(Clean(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	it := Item{Kind: ItemResult, Failed: failed, step: step}
	if len(lines) > 0 {
		it.Text, it.More = cut(lines[0], 300), len(lines)-1
	}
	o.Items = append(o.Items, it)
}

// toolArgs are the arguments that say best what a tool was called with.
var toolArgs = []string{
	"command", "CommandLine", "cmd", "file_path", "path", "AbsolutePath", "TargetFile", "notebook_path",
	"url", "Url", "query", "Query", "pattern", "Pattern", "skill", "description", "prompt", "SearchPath", "DirectoryPath",
}

// toolArg is what a tool was called with, in one line: its command, file,
// address or pattern, a path inside the project relative to it.
func (o *Output) toolArg(args map[string]any) string {
	v := ""
	for _, k := range toolArgs {
		if s := str(args[k]); strings.TrimSpace(s) != "" {
			v = s
			break
		}
	}
	if v == "" {
		keys := make([]string, 0, len(args))
		for k := range args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if s := str(args[k]); strings.TrimSpace(s) != "" {
				v = s
				break
			}
		}
	}
	v = strings.TrimSpace(v)
	if o.Dir != "" {
		v = strings.ReplaceAll(v, strings.TrimSuffix(o.Dir, "/")+"/", "")
	}
	if i := strings.IndexByte(v, '\n'); i >= 0 {
		v = strings.TrimSpace(v[:i]) + " …"
	}
	return cut(v, 300)
}

func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// content is a tool result's text: a string, or text blocks.
func content(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	var b strings.Builder
	for _, x := range list(v) {
		if t := str(obj(x)["text"]); t != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(t)
		}
	}
	return b.String()
}

func first(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
