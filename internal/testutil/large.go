package testutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// GenClaudeSession writes a synthetic Claude Code session of about size
// bytes to w: user/assistant turns with tool calls and results, usage on
// every assistant line, and long tool outputs like real sessions have. All
// content is generated; nothing comes from real sessions.
func GenClaudeSession(w io.Writer, size int64, cwd, sessionID string) error {
	bw := bufio.NewWriterSize(w, 1<<20)
	var written int64
	t := time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC)
	n := 0
	emit := func(m map[string]any) error {
		n++
		t = t.Add(2 * time.Second)
		m["uuid"] = fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
		m["sessionId"] = sessionID
		m["timestamp"] = t.Format(time.RFC3339Nano)
		m["cwd"] = cwd
		m["gitBranch"] = "main"
		m["version"] = "2.1.3"
		m["isSidechain"] = false
		b, err := json.Marshal(m)
		if err != nil {
			return err
		}
		k, err := bw.Write(append(b, '\n'))
		written += int64(k)
		return err
	}
	filler := strings.Repeat("lorem ipsum dolor sit amet ğüşıöç ", 60)
	output := strings.Repeat("ok  \tgithub.com/example/pkg\t0.123s\n", 400) // ~14 KB tool output
	turn := 0
	for written < size {
		turn++
		if err := emit(map[string]any{"type": "user", "message": map[string]any{
			"role":    "user",
			"content": fmt.Sprintf("step %d: please continue with the refactor. %s", turn, filler[:200]),
		}}); err != nil {
			return err
		}
		msgID := fmt.Sprintf("msg_%08d", turn)
		usage := map[string]any{"input_tokens": 1200, "output_tokens": 300, "cache_read_input_tokens": 50000, "cache_creation_input_tokens": 100}
		if err := emit(map[string]any{"type": "assistant", "message": map[string]any{
			"id": msgID, "role": "assistant", "model": "claude-opus-4-1",
			"content": []any{map[string]any{"type": "text", "text": "Working on step " + fmt.Sprint(turn) + ". " + filler}}, "usage": usage,
		}}); err != nil {
			return err
		}
		toolID := fmt.Sprintf("toolu_%08d", turn)
		if err := emit(map[string]any{"type": "assistant", "message": map[string]any{
			"id": msgID, "role": "assistant", "model": "claude-opus-4-1",
			"content": []any{map[string]any{"type": "tool_use", "id": toolID, "name": "Bash", "input": map[string]any{"command": "go test ./...", "description": "run tests"}}}, "usage": usage,
		}}); err != nil {
			return err
		}
		if err := emit(map[string]any{"type": "user", "message": map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "tool_result", "tool_use_id": toolID, "content": output}},
		}}); err != nil {
			return err
		}
		if turn%10 == 0 {
			if err := emit(map[string]any{"type": "assistant", "message": map[string]any{
				"id": msgID + "b", "role": "assistant", "model": "claude-opus-4-1",
				"content": []any{map[string]any{"type": "tool_use", "id": toolID + "t", "name": "TodoWrite", "input": map[string]any{"todos": []any{
					map[string]any{"content": fmt.Sprintf("step %d", turn), "status": "completed"},
					map[string]any{"content": fmt.Sprintf("step %d", turn+1), "status": "in_progress"},
				}}}}, "usage": usage,
			}}); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}
