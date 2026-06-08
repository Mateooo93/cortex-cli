package protocol

import (
	"fmt"
	"strings"
)

type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

func (s TodoStatus) Valid() bool {
	switch s {
	case TodoPending, TodoInProgress, TodoCompleted:
		return true
	}
	return false
}

type TodoItem struct {
	ID        string     `json:"id"`
	Content   string     `json:"content"`
	Status    TodoStatus `json:"status"`
	DependsOn []string   `json:"depends_on,omitempty"`
}

type EventTodoListUpdated struct {
	Todos []TodoItem `json:"todos"`
}

// TodoContentFromMap extracts display text from a todo_write item. Models
// vary on field names; status-only updates often omit content entirely.
func TodoContentFromMap(item map[string]any) string {
	for _, key := range []string{"content", "activeForm", "text", "title", "task", "name", "description"} {
		if v, ok := item[key].(string); ok {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// NormalizeTodoItem fills in missing id/content so the UI never renders a
// bare status icon with no label.
func NormalizeTodoItem(item TodoItem, index int) TodoItem {
	out := item
	if strings.TrimSpace(out.ID) == "" {
		out.ID = fmt.Sprintf("todo-%d", index+1)
	}
	if strings.TrimSpace(out.Content) == "" {
		out.Content = "Todo " + out.ID
	}
	return out
}

// MergeTodoList applies a todo_write payload onto the previous list. Incoming
// items replace the list in order, but empty content/status fields inherit
// values from the prior item with the same id so partial updates do not erase
// labels (the user reported checkmarks with invisible todo text).
func MergeTodoList(prev, next []TodoItem) []TodoItem {
	prevByID := make(map[string]TodoItem, len(prev))
	for _, t := range prev {
		prevByID[t.ID] = t
	}
	out := make([]TodoItem, 0, len(next))
	for i, item := range next {
		merged := item
		if old, ok := prevByID[merged.ID]; ok {
			if strings.TrimSpace(merged.Content) == "" {
				merged.Content = old.Content
			}
			if strings.TrimSpace(string(merged.Status)) == "" {
				merged.Status = old.Status
			}
		}
		out = append(out, NormalizeTodoItem(merged, i))
	}
	return out
}
