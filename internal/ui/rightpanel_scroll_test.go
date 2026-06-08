package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

func TestRenderInfoView_ScrollableSectionsCapRows(t *testing.T) {
	rp := RightPanel{}
	s := NewStyles(true)
	info := RightPanelInfoView{
		Subagents: make([]protocol.LocalSubagentItem, 5),
		Processes: make([]protocol.BackgroundProcessItem, 4),
		Todos:     make([]protocol.TodoItem, 7),
	}
	for i := range info.Subagents {
		info.Subagents[i] = protocol.LocalSubagentItem{
			ID:     "sub-" + string(rune('a'+i)),
			Role:   "explore",
			Task:   "task",
			Status: protocol.LocalSubagentRunning,
		}
	}
	for i := range info.Processes {
		info.Processes[i] = protocol.BackgroundProcessItem{
			ID:        "proc-" + string(rune('a'+i)),
			PID:       100 + i,
			Command:   "echo hi",
			StartedAt: time.Now().Unix(),
			Running:   true,
		}
	}
	for i := range info.Todos {
		info.Todos[i] = protocol.TodoItem{
			ID:      "todo-" + string(rune('a'+i)),
			Content: "item",
			Status:  protocol.TodoPending,
		}
	}

	sections, _ := rp.renderInfoView(panelWidth-4, info, s)
	plain := stripANSI(strings.Join(joinInfoSections(sections), "\n"))

	subCount := strings.Count(plain, "▶ explore")
	if subCount != rpMaxVisibleSubagents {
		t.Fatalf("expected %d visible subagents, got %d in:\n%s", rpMaxVisibleSubagents, subCount, plain)
	}
	procCount := strings.Count(plain, "● pid")
	if procCount != rpMaxVisibleProcesses {
		t.Fatalf("expected %d visible processes, got %d", rpMaxVisibleProcesses, procCount)
	}
	todoCount := strings.Count(plain, "○ item")
	if todoCount != rpMaxVisibleTodos {
		t.Fatalf("expected %d visible todos, got %d", rpMaxVisibleTodos, todoCount)
	}
	if !strings.Contains(plain, "↓ 2 more") {
		t.Fatalf("expected subagent scroll hint, got:\n%s", plain)
	}
}

func TestRightPanel_ScrollSectionAt(t *testing.T) {
	rp := RightPanel{}
	info := RightPanelInfoView{
		Subagents: []protocol.LocalSubagentItem{
			{ID: "1", Role: "a", Task: "t", Status: protocol.LocalSubagentRunning},
			{ID: "2", Role: "b", Task: "t", Status: protocol.LocalSubagentRunning},
			{ID: "3", Role: "c", Task: "t", Status: protocol.LocalSubagentRunning},
			{ID: "4", Role: "d", Task: "t", Status: protocol.LocalSubagentRunning},
		},
	}
	_, _ = rp.renderInfoView(panelWidth-4, info, NewStyles(true))
	if !rp.ScrollSectionAt(rp.sectionLineRange[rpSectionSubagents][0], 1) {
		t.Fatal("expected scroll in subagents section")
	}
	if rp.subagentScroll != 1 {
		t.Fatalf("subagentScroll = %d, want 1", rp.subagentScroll)
	}
}
