package session

// TodosFromArgs extracts a task list from a tool call's arguments, accepting
// the shapes used by the agents' todo/plan tools:
//
//	Claude TodoWrite:  {todos: [{content, status}]}
//	Codex update_plan:  {plan: [{step, status}]}
//
// It returns nil when no list is present.
func TodosFromArgs(args map[string]any) []Todo {
	var list []any
	for _, k := range []string{"todos", "plan", "tasks", "items"} {
		if l, ok := args[k].([]any); ok {
			list = l
			break
		}
	}
	if list == nil {
		return nil
	}
	out := []Todo{}
	for _, it := range list {
		switch v := it.(type) {
		case string:
			if v != "" {
				out = append(out, Todo{Text: v})
			}
		case map[string]any:
			text := ""
			for _, k := range []string{"content", "description", "step", "text", "title", "task"} {
				if s, ok := v[k].(string); ok && s != "" {
					text = s
					break
				}
			}
			if text == "" {
				continue
			}
			status, _ := v["status"].(string)
			if done, ok := v["done"].(bool); ok && done && status == "" {
				status = "done"
			}
			out = append(out, Todo{Text: text, Status: ParseTodoStatus(status)})
		}
	}
	return out
}
