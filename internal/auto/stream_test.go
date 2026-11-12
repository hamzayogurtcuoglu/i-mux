package auto

import (
	"strings"
	"testing"
)

// The fixtures follow the streams the CLIs wrote for a probe (Claude Code
// 2.1, Copilot CLI's JSONL, Antigravity's stream-json): synthetic content.
const claudeStream = `{"type":"system","subtype":"init","cwd":"/p","session_id":"s-1","model":"claude-sonnet-5-5","tools":["Bash"]}
{"type":"assistant","message":{"content":[{"type":"text","text":"Checking the tags."},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"git ls-remote --tags origin\nwc -l","description":"List tags"}}]},"parent_tool_use_id":null}
{"type":"user","message":{"content":[{"tool_use_id":"t1","type":"tool_result","content":"abc\trefs/tags/v2.3\n\ndef\trefs/tags/v2.2\n","is_error":false}]}}
{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"hm"},{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/p/docs/a.md"}}]}}
{"type":"user","message":{"content":[{"tool_use_id":"t2","type":"tool_result","content":[{"type":"text","text":"Error: no such file"}],"is_error":true}]}}
{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning","utilization":0.99}}
{"type":"assistant","message":{"content":[{"type":"text","text":"## v2.3 is out\nThe pipeline started."}]}}
{"type":"result","subtype":"success","is_error":false,"duration_ms":7123,"result":"## v2.3 is out\nThe pipeline started.","session_id":"s-1"}
`

const copilotStream = `{"type":"session.start","data":{"sessionId":"c-1","selectedModel":"claude-opus-5.5"},"id":"1"}
{"type":"user.message","data":{"content":"P"}}
{"type":"assistant.message","data":{"content":"","toolRequests":[{"name":"bash"}]}}
{"type":"tool.execution_start","data":{"toolCallId":"x1","toolName":"bash","arguments":{"command":"echo hi","description":"Echo"}}}
{"type":"tool.shell_output","data":{"toolCallId":"x1","stream":"stdout","text":"hi\n"}}
{"type":"tool.execution_complete","data":{"toolCallId":"x1","success":true,"result":{"content":"hi\n<shellId: 0 completed with exit code 0>"}}}
{"type":"tool.execution_start","data":{"toolCallId":"x2","toolName":"view","arguments":{"path":"/p/README.md"}}}
{"type":"tool.execution_complete","data":{"toolCallId":"x2","success":false,"error":{"message":"Path does not exist"}}}
{"type":"assistant.message_delta","data":{"deltaContent":"Done"}}
{"type":"assistant.message","data":{"content":"Done: hi","toolRequests":[]}}
{"type":"result","timestamp":"2026-11-12T12:06:45.875Z","sessionId":"c-1","exitCode":0,"usage":{"premiumRequests":1}}
`

const agyStream = `{"event":"init","conversation_id":"g-1","init":{"cwd":"/p","tools":["run_command"]}}
{"event":"step_update","step_update":{"step_index":0,"state":"DONE","step_type":"user_input"}}
{"event":"step_update","step_update":{"step_index":1,"state":"DONE","step_type":"agent_response"}}
{"event":"step_update","step_update":{"step_index":2,"state":"ACTIVE","step_type":"tool","tool_name":"run_command","tool_info":{"name":"run_command","parameters":{"CommandLine":"echo imux-probe"}}}}
{"event":"step_update","step_update":{"step_index":2,"state":"DONE","step_type":"tool","tool_name":"run_command","tool_info":{"name":"run_command","parameters":{"CommandLine":"echo imux-probe"},"output":"Created At: 2026-11-12T15:06:47+03:00\nCompleted At: 2026-11-12T15:06:48+03:00\nCommand exited with code 0.\n\nCommand output:\nimux-probe\r\n"}}}
{"event":"step_update","step_update":{"step_index":4,"state":"DONE","step_type":"tool","tool_name":"run_command","tool_info":{"name":"run_command","parameters":{"CommandLine":"make test"},"output":"Command exited with code 2.\n\nCommand output:\nFAIL x\n"}}}
{"event":"step_update","step_update":{"step_index":5,"state":"ACTIVE","step_type":"agent_response","text_delta":"do"}}
{"event":"step_update","step_update":{"step_index":5,"state":"ACTIVE","step_type":"agent_response","text_delta":"ne"}}
{"event":"step_update","step_update":{"step_index":5,"state":"DONE","step_type":"agent_response","text_delta":"\n"}}
{"event":"result","result":{"conversation_id":"g-1","status":"SUCCESS","response":"done\n","duration_seconds":13.9}}
`

// kinds is an output's items in short: "T:text", "B:Bash cmd", "R:first +2", "!R:failed", "E:error", "N:note", "P:plain".
func kinds(o *Output) []string {
	var out []string
	for _, it := range o.Items {
		var s string
		switch it.Kind {
		case ItemText:
			s = "T:" + it.Text
		case ItemTool:
			s = "B:" + it.Name + " " + it.Text
		case ItemResult:
			s = "R:" + it.Text
			if it.More > 0 {
				s += " +" + string(rune('0'+it.More))
			}
			if it.Failed {
				s = "!" + s
			}
		case ItemNote:
			s = "N:" + it.Text
		case ItemError:
			s = "E:" + it.Text
		case ItemPlain:
			s = "P:" + it.Text
		}
		out = append(out, s)
	}
	return out
}

func TestParseOutput(t *testing.T) {
	for _, c := range []struct {
		name, raw           string
		items               []string
		report, sess, model string
	}{
		{"claude", claudeStream, []string{
			"T:Checking the tags.", "B:Bash git ls-remote --tags origin …", "R:abc\trefs/tags/v2.3 +1",
			"B:Read docs/a.md", "!R:Error: no such file", "T:## v2.3 is out\nThe pipeline started.",
		}, "## v2.3 is out\nThe pipeline started.", "s-1", "claude-sonnet-5-5"},
		{"copilot", copilotStream, []string{
			"B:bash echo hi", "R:hi +1", "B:view README.md", "!R:Path does not exist", "T:Done: hi",
		}, "Done: hi", "c-1", "claude-opus-5.5"},
		{"agy", agyStream, []string{
			"B:run_command echo imux-probe", "R:imux-probe", "B:run_command make test", "!R:FAIL x", "T:done\n",
		}, "done\n", "g-1", ""},
	} {
		o := ParseOutput(c.raw, "/p")
		if got := strings.Join(kinds(o), "|"); got != strings.Join(c.items, "|") {
			t.Errorf("%s items:\n got %q\nwant %q", c.name, kinds(o), c.items)
		}
		if !o.Streamed || !o.Done || o.Failed || o.Report != c.report || o.Session != c.sess || o.Model != c.model {
			t.Errorf("%s: %+v", c.name, *o)
		}
		// Fed in pieces, as a growing log is read: the same.
		p := &Output{Dir: "/p"}
		for i := 0; i < len(c.raw); i += 7 {
			p.Feed([]byte(c.raw[i:min(i+7, len(c.raw))]))
			if i == len(c.raw)/2 && p.Done {
				t.Errorf("%s: done half way", c.name)
			}
		}
		p.Flush()
		if strings.Join(kinds(p), "|") != strings.Join(kinds(o), "|") || p.Report != o.Report {
			t.Errorf("%s fed in pieces: %q", c.name, kinds(p))
		}
	}
}

func TestParseOutputFailures(t *testing.T) {
	for _, c := range []struct {
		name, raw, err string
		items          []string
	}{
		{"claude error", `{"type":"result","subtype":"error_during_execution","is_error":true,"result":""}`, "error during execution", nil},
		{"claude limit", `{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":1794592800}}` + "\n" +
			`{"type":"result","subtype":"success","is_error":true,"result":"Claude AI usage limit reached\nmore"}`, "Claude AI usage limit reached", []string{"E:usage limit reached until "}},
		{"copilot", `{"type":"session.error","data":{"errorType":"auth","message":"Not logged in"}}` + "\n" +
			`{"type":"result","sessionId":"c-2","exitCode":1}`, "Not logged in", []string{"E:Not logged in"}},
		{"agy", `{"event":"result","result":{"conversation_id":"","status":"ERROR","response":"","error":"Error: empty prompt.\nUsage: agy"}}`, "Error: empty prompt.", nil},
	} {
		o := ParseOutput(c.raw, "")
		if !o.Failed || o.Error != c.err {
			t.Errorf("%s: failed %v error %q", c.name, o.Failed, o.Error)
		}
		got := kinds(o)
		for i, want := range c.items {
			if i >= len(got) || !strings.HasPrefix(got[i], want) {
				t.Errorf("%s items %q", c.name, got)
			}
		}
	}
}

// TestParseOutputPlain: a text CLI's output, or a warning among events,
// is kept as lines; tool arguments without a known name show their first.
func TestParseOutputPlain(t *testing.T) {
	o := ParseOutput("line one\n\x1b[1mline two\x1b[0m\r\n{not json\n", "")
	if o.Streamed || o.Done || strings.Join(kinds(o), "|") != "P:line one|P:line two|P:{not json" {
		t.Errorf("%q %+v", kinds(o), *o)
	}
	o = ParseOutput("warning: old config\n"+copilotStream, "")
	if !o.Streamed || kinds(o)[0] != "P:warning: old config" {
		t.Errorf("%q", kinds(o))
	}
	if got := (&Output{}).toolArg(map[string]any{"zeta": "z", "alpha": " a ", "n": 3.0}); got != "a" {
		t.Errorf("toolArg %q", got)
	}
	if got := (&Output{Dir: "/p/"}).toolArg(map[string]any{"pattern": "TODO", "path": "/p/src"}); got != "src" {
		t.Errorf("toolArg path %q", got)
	}
}
