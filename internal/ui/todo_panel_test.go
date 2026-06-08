package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

func TestRightPanel_InfoMode_KeepsTodosWhenOverflow(t *testing.T) {
	rp := RightPanel{}
	rp.OpenInfo(18)
	s := NewStyles(true)
	info := RightPanelInfoView{
		ModelName:    "GPT-5",
		ProviderName: "openai",
		InputTokens:  170_000,
		ContextMax:   200_000,
		AutoCompact:  true,
		Elapsed:      12 * time.Minute,
		Subagents:    make([]protocol.LocalSubagentItem, 5),
		Processes:    make([]protocol.BackgroundProcessItem, 4),
		Todos: []protocol.TodoItem{
			{ID: "1", Content: "fix panel bug", Status: protocol.TodoInProgress},
			{ID: "2", Content: "add tests", Status: protocol.TodoPending},
		},
	}
	for i := range info.Subagents {
		info.Subagents[i] = protocol.LocalSubagentItem{
			ID:     "sub-" + string(rune('a'+i)),
			Role:   "explore",
			Task:   "long running background task",
			Status: protocol.LocalSubagentRunning,
		}
	}
	for i := range info.Processes {
		info.Processes[i] = protocol.BackgroundProcessItem{
			ID:        "proc-" + string(rune('a'+i)),
			PID:       100 + i,
			Command:   "npm run dev",
			StartedAt: time.Now().Unix(),
			Running:   true,
		}
	}

	view := rp.View(18, s, true, "GPT-5", nil, info)
	plain := stripANSI(view)
	if !strings.Contains(plain, "Todos") {
		t.Fatalf("expected todos section in cramped info panel, got:\n%s", plain)
	}
	if !strings.Contains(plain, "fix panel bug") {
		t.Fatalf("expected in-progress todo to remain visible, got:\n%s", plain)
	}
}

func TestRightPanel_TodosMode_StatusOnlyUpdateStillShowsLabels(t *testing.T) {
	rp := RightPanel{}
	rp.OpenTodos(16)
	s := NewStyles(true)
	prev := []protocol.TodoItem{
		{ID: "1", Content: "Implement feature", Status: protocol.TodoInProgress},
	}
	merged := protocol.MergeTodoList(prev, []protocol.TodoItem{
		{ID: "1", Status: protocol.TodoCompleted},
	})
	view := rp.View(16, s, true, "", merged, RightPanelInfoView{})
	plain := stripANSI(view)
	if !strings.Contains(plain, "Implement feature") {
		t.Fatalf("expected todo label after status-only update, got:\n%s", plain)
	}
	if !strings.Contains(plain, "✓") {
		t.Fatalf("expected completed checkmark, got:\n%s", plain)
	}
}

func TestRightPanel_TodosMode_ScrollsLongLists(t *testing.T) {
	rp := RightPanel{}
	rp.OpenTodos(16)
	s := NewStyles(true)
	todos := make([]protocol.TodoItem, 8)
	for i := range todos {
		todos[i] = protocol.TodoItem{
			ID:      "todo-" + string(rune('a'+i)),
			Content: "task " + string(rune('a'+i)),
			Status:  protocol.TodoPending,
		}
	}

	view := rp.View(16, s, true, "", todos, RightPanelInfoView{})
	plain := stripANSI(view)
	if !strings.Contains(plain, "↓ 3 more") {
		t.Fatalf("expected scroll hint for long todo list, got:\n%s", plain)
	}
	visible := strings.Count(plain, "○ task")
	if visible != rpMaxVisibleTodos {
		t.Fatalf("expected %d visible todos, got %d in:\n%s", rpMaxVisibleTodos, visible, plain)
	}
}
