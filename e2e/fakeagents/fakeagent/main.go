// Command fakeagent is fake.sh in Go: the fake agent CLI of imux's end-to-end
// tests on Windows, where a shell script cannot be an agent's program (T90.9).
// It never touches the network and never runs a real agent. The agent is
// FAKE_AGENT, else the program's name: fake-claude.exe is claude; nvim.exe is
// the fake Neovim of e2e/fakeagents/nvim.
//
// It logs and prints what fake.sh does, byte for byte on the same input
// (TestSameAsFakeSh), so read fake.sh for the files it writes. A test's
// control script ($IMUX_FAKE_LOG/A.sh, FAKE_SCRIPT) runs in sh (IMUX_E2E_SH,
// else sh on PATH) with $log, $n, $agent and the arguments; its exit ends the
// fake with that status, as when fake.sh sources it.
package main

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() { os.Exit(run(os.Args[1:])) }

func agentName() string {
	if a := os.Getenv("FAKE_AGENT"); a != "" {
		return a
	}
	exe := filepath.Base(os.Args[0])
	exe = strings.TrimSuffix(exe, filepath.Ext(exe))
	return strings.TrimPrefix(exe, "fake-")
}

func logDir() string {
	if l := os.Getenv("IMUX_FAKE_LOG"); l != "" {
		return l
	}
	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		tmp = os.TempDir()
	}
	return filepath.Join(tmp, "imux-fake")
}

func appendLine(path, line string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = io.WriteString(f, line+"\n")
}

func appendFile(path, s string) { // appends s as it is
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = io.WriteString(f, s)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func readTrim(p string) (string, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", false
	}
	return strings.TrimRight(string(b), "\n"), true // $(cat f)
}

// opt is fake.sh's opt: the value after name.
func opt(name string, args []string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1], true
		}
	}
	return "", false
}

// jstr is fake.sh's jstr: s as the inside of a JSON string.
func jstr(s string) string {
	s = strings.TrimSuffix(s, "\n")
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\t", `\t`, "\n", `\n`)
	return r.Replace(s)
}

// jpath is a path in fake.sh's JSON, where it is written as it is: the same
// on macOS and Linux, and Windows' backslashes doubled so the line parses.
func jpath(p string) string { return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(p) }

func joinArgs(a []string) string { return strings.Join(a, " ") }

func nulArgs(args []string) string {
	var b strings.Builder
	for _, a := range args {
		b.WriteString(a)
		b.WriteByte(0)
	}
	return b.String()
}

func uuid() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func nowMs() string { return strconv.FormatInt(time.Now().Unix(), 10) + "000" }

// claim creates prefix-n.argv for the first free n, as fake.sh's noclobber loop.
func claim(prefix string) int {
	for n := 1; ; n++ {
		f, err := os.OpenFile(prefix+"-"+strconv.Itoa(n)+".argv", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return n
		}
		if !errors.Is(err, os.ErrExist) {
			return n
		}
	}
}

func run(args []string) int {
	agent := agentName()
	log := logDir()
	_ = os.MkdirAll(log, 0o755)
	if agent == "nvim" {
		return nvim(log, args)
	}
	out := os.Stdout

	if len(args) == 1 && args[0] == "--version" {
		v := os.Getenv("FAKE_VERSION")
		if v == "" {
			v = "9.9.9"
		}
		fmt.Fprintf(out, "%s %s-fake\n", agent, v)
		appendLine(filepath.Join(log, "version-probes"), agent+" --version")
		return 0
	}
	if len(args) > 0 && (args[0] == "update" || args[0] == "upgrade") {
		appendLine(filepath.Join(log, "updates"), agent+" "+joinArgs(args))
		fmt.Fprintf(out, "%s is up to date\n", agent)
		return 0
	}
	if len(args) == 1 && args[0] == "models" {
		fmt.Fprint(out, "fake-model-1\tFake Model One\nfake-model-2\tFake Model Two\n")
		appendLine(filepath.Join(log, "version-probes"), agent+" models")
		return 0
	}
	if len(args) == 1 && args[0] == "--help" {
		fmt.Fprintf(out, "Usage: %s [options] [prompt]\n", agent)
		fmt.Fprintln(out, "  --session-id <uuid>  -i, --prompt-interactive <prompt>  -r, --resume <id>  resume <id>")
		appendLine(filepath.Join(log, "version-probes"), agent+" --help")
		return 0
	}
	if agent == "opencode" && len(args) == 2 && args[0] == "api" && args[1] == "experimental.usage.limits" {
		appendLine(filepath.Join(log, "opencode-api"), "opencode api experimental.usage.limits")
		if b, err := os.ReadFile(filepath.Join(log, "opencode.go-usage")); err == nil {
			_, _ = out.Write(b)
		} else {
			fmt.Fprintln(out, `{"location":{},"data":null}`)
		}
		return 0
	}

	ocdir := opencodeDir()
	if agent == "opencode" && len(args) >= 2 && args[0] == "session" && args[1] == "export" {
		id := args[len(args)-1]
		f := filepath.Join(ocdir, "fake-sessions", id+".msgs")
		msgs, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Session not found: %s\n", id)
			return 1
		}
		dir, _ := readTrim(filepath.Join(ocdir, "fake-sessions", id+".dir"))
		created, _ := os.ReadFile(filepath.Join(ocdir, "fake-sessions", id+".created"))
		if len(created) > 13 {
			created = created[:13]
		}
		fmt.Fprintf(out, `{"info":{"id":"%s","model":{"id":"fake-model","providerID":"fake"},"time":{"created":%s,"updated":%s},"location":{"directory":"%s"}},"messages":[`,
			id, created, nowMs(), jpath(dir))
		lines := strings.Split(strings.TrimSuffix(string(msgs), "\n"), "\n")
		fmt.Fprint(out, strings.Join(lines, ","))
		fmt.Fprint(out, "]}\n")
		return 0
	}

	headless := false
	switch agent + ":" + first(args) {
	case "claude:-p", "copilot:-p", "antigravity:-p", "codex:exec", "opencode:run":
		headless = true
	}
	if headless {
		return runHeadless(agent, log, args)
	}

	n := claim(filepath.Join(log, agent))
	ns := strconv.Itoa(n)
	if len(args) > 0 {
		_ = os.WriteFile(filepath.Join(log, agent+"-"+ns+".argv"), []byte(nulArgs(args)), 0o644)
	}
	pwd, _ := os.Getwd()
	env := fmt.Sprintf("IMUX_PANE=%s\nIMUX_AGENT=%s\nTMUX_PANE=%s\nPATH=%s\nPWD=%s\nE2E_ENV_PROBE=%s\n",
		os.Getenv("IMUX_PANE"), os.Getenv("IMUX_AGENT"), os.Getenv("TMUX_PANE"), os.Getenv("PATH"), pwd, os.Getenv("E2E_ENV_PROBE"))
	_ = os.WriteFile(filepath.Join(log, agent+"-"+ns+".env"), []byte(env), 0o644)

	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		pane = "none"
	}
	req := fmt.Sprintf("fake %s request %d from pane %s", agent, n, pane)
	reply := fmt.Sprintf("fake %s reply %d", agent, n)
	last := ""
	if len(args) > 0 {
		last = args[len(args)-1]
	}
	if exists(filepath.Join(log, agent+".handoff-as-request")) && strings.HasPrefix(last, "[i-mux handoff") {
		// fake.sh's awk: backslashes and quotes escaped, lines joined by \n.
		r := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(strings.TrimSuffix(last, "\n"))
		req = strings.ReplaceAll(r, "\n", `\n`)
	}
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	writeSession(agent, log, ns, args, pwd, req, reply, ts, ocdir)

	fmt.Fprintf(out, "fake %s ready (run %d, pane %s)\n", agent, n, pane)
	fmt.Fprintf(out, "argc %d · last argument %d bytes\n", len(args), len(last))

	vars := []string{"log=" + log, "n=" + ns, "agent=" + agent}
	if ctl := filepath.Join(log, agent+".sh"); exists(ctl) {
		if code, exited := shell(`. "$FAKE_CONTROL"`, append(vars, "FAKE_CONTROL="+ctl), args); exited {
			return code
		}
	}
	if s := os.Getenv("FAKE_SCRIPT"); s != "" {
		if code, exited := shell(`eval "$FAKE_SCRIPT"`, vars, args); exited {
			return code
		}
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.HasPrefix(line, "/exit ") {
			code, _ := strconv.Atoi(strings.TrimPrefix(line, "/exit "))
			return code
		}
		appendLine(filepath.Join(log, agent+"-"+ns+".stdin"), line)
	}
	if s := os.Getenv("FAKE_EXIT_STATUS"); s != "" {
		code, _ := strconv.Atoi(s)
		return code
	}
	return 0
}

func first(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return a[0]
}

func opencodeDir() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode")
}

// shell runs body in sh as fake.sh sources a control script: same terminal,
// $log, $n, $agent and the arguments. exited reports that the script called
// exit (then code is its status), not that it reached its end.
func shell(body string, vars, args []string) (code int, exited bool) {
	sh := os.Getenv("IMUX_E2E_SH")
	if sh == "" {
		sh = "sh"
	}
	marker, err := os.CreateTemp("", "fakeagent-*")
	if err != nil {
		return 1, true
	}
	marker.Close()
	_ = os.Remove(marker.Name())
	defer os.Remove(marker.Name())
	script := "log=\"$1\"; n=\"$2\"; agent=\"$3\"; shift 3\n" + body + "\n: >\"$FAKE_DONE\"\n"
	cmd := exec.Command(sh, append([]string{"-c", script, "sh", varOf(vars, "log"), varOf(vars, "n"), varOf(vars, "agent")}, args...)...)
	cmd.Env = append(os.Environ(), append(vars, "FAKE_DONE="+filepath.ToSlash(marker.Name()))...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()
	if exists(marker.Name()) {
		return 0, false
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), true
	}
	if err != nil {
		return 1, true
	}
	return 0, true
}

func varOf(vars []string, name string) string {
	for _, v := range vars {
		if strings.HasPrefix(v, name+"=") {
			return strings.TrimPrefix(v, name+"=")
		}
	}
	return ""
}

func runHeadless(agent, log string, args []string) int {
	n := claim(filepath.Join(log, agent+"-headless"))
	ns := strconv.Itoa(n)
	_ = os.WriteFile(filepath.Join(log, agent+"-headless-"+ns+".argv"), []byte(nulArgs(args)), 0o644)
	if agent == "claude" || agent == "codex" {
		b, _ := io.ReadAll(os.Stdin)
		_ = os.WriteFile(filepath.Join(log, agent+"-headless-"+ns+".stdin"), b, 0o644)
	}
	msg := fmt.Sprintf("feat: fake %s commit message %d", agent, n)
	if m, ok := readTrim(filepath.Join(log, agent+".headless-reply")); ok {
		msg = m
	}
	out := os.Stdout
	of, _ := opt("--output-format", args)
	switch {
	case agent == "codex":
		path := ""
		for i := 1; i < len(args); i++ {
			if args[i-1] == "--output-last-message" {
				path = args[i]
			}
		}
		fmt.Fprintln(os.Stderr, "codex: thinking")
		_ = os.WriteFile(path, []byte(msg+"\n"), 0o644)
	case agent == "claude" && of == "stream-json":
		sid, ok := opt("--session-id", args)
		if !ok {
			sid = "fake-session"
		}
		text := jstr(msg)
		fmt.Fprintf(out, `{"type":"system","subtype":"init","session_id":"%s","model":"fake-model"}`+"\n", sid)
		fmt.Fprintf(out, `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t%d","name":"Bash","input":{"command":"echo fake step %d"}}]},"session_id":"%s"}`+"\n", n, n, sid)
		fmt.Fprintf(out, `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t%d","content":"fake step %d","is_error":false}]},"session_id":"%s"}`+"\n", n, n, sid)
		if p, ok := readTrim(filepath.Join(log, agent+".headless-pause")); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(p), 64); err == nil {
				time.Sleep(time.Duration(f * float64(time.Second)))
			}
		}
		fmt.Fprintf(out, `{"type":"assistant","message":{"content":[{"type":"text","text":"%s"}]},"session_id":"%s"}`+"\n", text, sid)
		fmt.Fprintf(out, `{"type":"result","subtype":"success","is_error":false,"result":"%s","session_id":"%s","duration_ms":5}`+"\n", text, sid)
	case exists(filepath.Join(log, agent+".headless-plain")):
		fmt.Fprintln(out, msg)
	default:
		fmt.Fprintf(out, "Here is a commit message:\n\n```\n%s\n```\n", msg)
	}
	return 0
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

func writeSession(agent, log, n string, args []string, pwd, req, reply, ts, ocdir string) {
	if exists(filepath.Join(log, agent+".nosession")) {
		return
	}
	home, _ := os.UserHomeDir()
	envOr := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	session := ""
	jp := jpath(pwd)
	switch agent {
	case "claude":
		id, ok := opt("--session-id", args)
		if !ok {
			if id, ok = opt("--resume", args); !ok {
				id = uuid()
			}
		}
		dir := filepath.Join(envOr("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude")), "projects", nonAlnum.ReplaceAllString(pwd, "-"))
		_ = os.MkdirAll(dir, 0o755)
		session = filepath.Join(dir, id+".jsonl")
		appendFile(session, fmt.Sprintf(`{"type":"user","sessionId":"%s","cwd":"%s","timestamp":"%s","uuid":"u%s","message":{"role":"user","content":"%s"}}`+"\n"+
			`{"type":"assistant","sessionId":"%s","cwd":"%s","timestamp":"%s","uuid":"a%s","message":{"id":"msg-%s-%s","role":"assistant","model":"fake-model","content":[{"type":"text","text":"%s"}]}}`+"\n",
			id, jp, ts, n, req, id, jp, ts, n, n, id, reply))
	case "codex":
		root := envOr("CODEX_HOME", filepath.Join(home, ".codex"))
		var id string
		if first(args) == "resume" && len(args) > 1 {
			id = args[1]
			matches, _ := filepath.Glob(filepath.Join(root, "sessions", "*", "*", "*", "rollout-*-"+id+".jsonl"))
			if len(matches) > 0 {
				session = matches[0]
			}
		} else {
			id = uuid()
		}
		if session == "" {
			now := time.Now().UTC()
			dir := filepath.Join(root, "sessions", now.Format("2006"), now.Format("01"), now.Format("02"))
			_ = os.MkdirAll(dir, 0o755)
			session = filepath.Join(dir, "rollout-"+now.Format("2006-01-02T15-04-05")+"-"+id+".jsonl")
			appendFile(session, fmt.Sprintf(`{"timestamp":"%s","type":"session_meta","payload":{"id":"%s","cwd":"%s"}}`+"\n", ts, id, jp))
		}
		appendFile(session, fmt.Sprintf(`{"timestamp":"%s","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"%s"}]}}`+"\n"+
			`{"timestamp":"%s","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"%s"}]}}`+"\n", ts, req, ts, reply))
	case "antigravity":
		id, ok := opt("--conversation", args)
		if !ok {
			id = uuid()
		}
		root := filepath.Join(home, ".gemini", "antigravity-cli")
		dir := filepath.Join(root, "brain", id)
		_ = os.MkdirAll(filepath.Join(dir, ".system_generated", "logs"), 0o755)
		session = filepath.Join(dir, ".system_generated", "logs", "transcript_full.jsonl")
		step := 0
		if b, err := os.ReadFile(session); err == nil {
			step = strings.Count(string(b), "\n")
		}
		appendFile(session, fmt.Sprintf(`{"step_index":%d,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"%s","content":"<USER_REQUEST>\n%s\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nThe current local time is: %s.\n</ADDITIONAL_METADATA>"}`, step, ts, req, ts)+"\n")
		appendFile(session, fmt.Sprintf(`{"step_index":%d,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"%s","content":"%s","tool_calls":[{"name":"run_command","args":{"CommandLine":"fake test %s","WaitMsBeforeAsync":0}}],"input_tokens":100,"output_tokens":10}`, step+1, ts, reply, n)+"\n")
		appendFile(session, fmt.Sprintf(`{"step_index":%d,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"%s","content":"ok"}`, step+2, ts)+"\n")
		appendFile(filepath.Join(root, "history.jsonl"), fmt.Sprintf(`{"display":"%s","timestamp":%s,"workspace":"%s"}`, req, nowMs(), jp)+"\n")
		_ = os.WriteFile(filepath.Join(dir, "task.md"), []byte(fmt.Sprintf("# Task\n\n- [x] fake antigravity step done %s\n- [/] fake antigravity step in progress %s\n", n, n)), 0o644)
	case "opencode":
		id, ok := opt("--session", args)
		if !ok {
			id = "ses_" + uuid()
		}
		sdir := filepath.Join(ocdir, "fake-sessions")
		_ = os.MkdirAll(sdir, 0o755)
		appendFile(filepath.Join(ocdir, "opencode.db"), "")
		session = filepath.Join(sdir, id+".msgs")
		if !exists(filepath.Join(sdir, id+".created")) {
			_ = os.WriteFile(filepath.Join(sdir, id+".created"), []byte(nowMs()+"\n"), 0o644)
		}
		_ = os.WriteFile(filepath.Join(sdir, id+".dir"), []byte(pwd), 0o644)
		ms := nowMs()
		appendFile(session, fmt.Sprintf(`{"type":"user","time":{"created":%s},"text":"%s"}`+"\n", ms, req))
		appendFile(session, fmt.Sprintf(`{"type":"assistant","time":{"created":%s},"content":[{"type":"text","text":"%s"},{"type":"tool","name":"bash","state":{"status":"completed","input":{"command":"fake test %s"}}}]}`+"\n", ms, reply, n))
	case "copilot":
		id, ok := opt("--resume", args)
		if !ok {
			id = uuid()
		}
		dir := filepath.Join(envOr("COPILOT_HOME", filepath.Join(home, ".copilot")), "session-state", id)
		_ = os.MkdirAll(dir, 0o755)
		session = filepath.Join(dir, "events.jsonl")
		if ws := filepath.Join(dir, "workspace.yaml"); !exists(ws) {
			_ = os.WriteFile(ws, []byte(fmt.Sprintf("id: %s\ncwd: %s\ncreated_at: %s\n", id, pwd, ts)), 0o644)
		}
		appendFile(session, fmt.Sprintf(`{"type":"user.message","timestamp":"%s","data":{"content":"%s"}}`+"\n"+
			`{"type":"assistant.message","timestamp":"%s","data":{"content":"%s"}}`+"\n", ts, req, ts, reply))
	}
	if session != "" {
		_ = os.WriteFile(filepath.Join(log, agent+"-"+n+".session"), []byte(session+"\n"), 0o644)
	}
}

// nvim is e2e/fakeagents/nvim: it logs how it was started.
func nvim(log string, args []string) int {
	_ = os.WriteFile(filepath.Join(log, "nvim.argv"), []byte(nulArgs(args)), 0o644)
	_ = os.WriteFile(filepath.Join(log, "nvim.env"), []byte("NVIM_APPNAME="+os.Getenv("NVIM_APPNAME")+"\n"), 0o644)
	fmt.Println("fake nvim " + joinArgs(args))
	return 0
}
