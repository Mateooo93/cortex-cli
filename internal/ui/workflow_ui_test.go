package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/workflow"
)

// TestRenderStatusBarShowsWorkflow verifies the slim footer
// includes a "● workflow <name> (2:13)" segment when a
// workflow is running. The user explicitly asked for this
// so they can see the orchestrator is busy even when
// they're in the chat tab.
func TestRenderStatusBarShowsWorkflow(t *testing.T) {
	s := NewStyles(true)
	info := StatusBarInfo{
		ModelName:       "GPT-5.5",
		InputTokens:     1000,
		ContextMax:      200_000,
		WorkflowName:    "code",
		WorkflowStatus:  "running",
		WorkflowElapsed: 2*time.Minute + 13*time.Second,
	}
	bar := renderStatusBar(120, true, false, StatusMessage{}, s, info)
	if !strings.Contains(bar, "workflow code") {
		t.Errorf("expected 'workflow code' in footer, got %q", bar)
	}
	if !strings.Contains(bar, "2:13") {
		t.Errorf("expected '2:13' elapsed time, got %q", bar)
	}
}

// TestRenderStatusBarOmitsWorkflowWhenIdle verifies the
// footer doesn't show a workflow segment when no
// workflow is running.
func TestRenderStatusBarOmitsWorkflowWhenIdle(t *testing.T) {
	s := NewStyles(true)
	bar := renderStatusBar(120, true, false, StatusMessage{}, s, StatusBarInfo{
		ModelName: "GPT-5.5",
	})
	if strings.Contains(bar, "workflow") {
		t.Errorf("expected no 'workflow' segment when no workflow is running, got %q", bar)
	}
}

// TestRightPanel_InfoMode_ShowsWorkflowRunning verifies the
// right panel includes a "Workflow" block with the name,
// status, and currentMsg when a workflow is active.
func TestRightPanel_InfoMode_ShowsWorkflowRunning(t *testing.T) {
	rp := RightPanel{}
	rp.OpenInfo(40)
	s := NewStyles(true)
	info := RightPanelInfoView{
		ModelName:        "GPT-5.5",
		InputTokens:      1000,
		ContextMax:       200_000,
		Connected:        true,
		WorkflowName:     "code",
		WorkflowStatus:   "running",
		WorkflowElapsed:  2*time.Minute + 13*time.Second,
		WorkflowCurrent:  "developer: writing the auth middleware",
	}
	view := rp.View(40, s, true, "GPT-5.5", nil, nil, info)
	for _, want := range []string{"Workflow", "code", "developer", "auth", "2:13"} {
		if !strings.Contains(view, want) {
			t.Errorf("info panel missing %q, got:\n%s", want, view)
		}
	}
}

// TestRenderWorkflowsView_EmptyState verifies the empty
// state of the Workflows tab. When no workflows are
// running, the tab shows a brief description of what
// workflows are and how to start one with
// `/workflow <prompt>`. The user asked for a low-ceremony
// view with no preset picker clutter.
func TestRenderWorkflowsView_EmptyState(t *testing.T) {
	s := NewStyles(true)
	engine := workflow.NewEngine(nil)
	view := renderWorkflowsView(120, 40, s, engine, 0)
	for _, want := range []string{"Workflows", "/workflow <prompt>", "build a CLI todo app in Go"} {
		if !strings.Contains(view, want) {
			t.Errorf("empty state missing %q, got:\n%s", want, view)
		}
	}
	// No preset list: the user explicitly asked us to
	// remove the preset picker from the Workflows tab.
	// We can't check for "code" / "research" / "review" /
	// "docs" as bare substrings (they could appear in
	// prose), so we look for the "·" bullet that the
	// preset list used.
	if strings.Contains(view, "  · ") {
		t.Errorf("empty state should not list presets with bullet '·', got:\n%s", view)
	}
}

// TestRenderWorkflowsView_WithActiveWorkflow verifies the
// view shows the active workflow's per-step breakdown
// when one is running.
func TestRenderWorkflowsView_WithActiveWorkflow(t *testing.T) {
	s := NewStyles(true)
	engine := workflow.NewEngine(nil)
	id, err := engine.Start(context.Background(), "code", "build a thing", "development", 3)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer engine.Cancel(id)
	// Wait briefly for the workflow to register (the
	// engine spawns a goroutine).
	time.Sleep(10 * time.Millisecond)
	view := renderWorkflowsView(120, 40, s, engine, 0)
	if !strings.Contains(view, "code") {
		t.Errorf("expected active workflow 'code' in view, got:\n%s", view)
	}
}
