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
	rp.OpenInfo(30)
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
	view := rp.View(30, s, true, "GPT-5.5", nil, nil, info)
	// The panel must include the workflow name and
	// currentMsg. We don't assert on the section header
	// (the renderer uses different labels depending on
	// status) but the user-visible bits must all be
	// there.
	for _, want := range []string{"Workflow", "code", "developer", "auth", "2:13"} {
		if !strings.Contains(view, want) {
			t.Errorf("info panel missing %q, got:\n%s", want, view)
		}
	}
}

// TestRenderWorkflowsView_EmptyState verifies the empty
// state of the Workflows tab. When the user has never
// started a workflow, the tab shows the preset list as a
// hint and an instruction to press 'n' to start one.
func TestRenderWorkflowsView_EmptyState(t *testing.T) {
	s := NewStyles(true)
	engine := workflow.NewEngine(nil)
	view := renderWorkflowsView(120, 40, s, engine, 0, workflow.BuiltinPresets, 0, false, "", 0)
	for _, want := range []string{"Workflows", "No workflows yet", "code", "research", "test", "review", "docs"} {
		if !strings.Contains(view, want) {
			t.Errorf("empty state missing %q, got:\n%s", want, view)
		}
	}
}

// TestRenderWorkflowsView_NewModePrompt verifies the preset
// picker overlay that appears when the user presses 'n'.
// It must list every preset with a cursor marker on the
// highlighted row.
func TestRenderWorkflowsView_NewModePrompt(t *testing.T) {
	s := NewStyles(true)
	engine := workflow.NewEngine(nil)
	view := renderWorkflowsView(120, 40, s, engine, 0, workflow.BuiltinPresets, 2, true, "build me a thing", 0)
	for _, want := range []string{"New workflow", "code", "research", "test", "Enter start", "Esc cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("new-mode prompt missing %q, got:\n%s", want, view)
		}
	}
}

// TestRenderWorkflowsView_WithActiveWorkflow verifies the
// view shows the active workflow's per-step breakdown
// when one is running. The engine is populated manually
// (not via Start) so the test doesn't need an LLM.
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
	view := renderWorkflowsView(120, 40, s, engine, 0, workflow.BuiltinPresets, 0, false, "", 0)
	if !strings.Contains(view, "code") {
		t.Errorf("expected active workflow 'code' in view, got:\n%s", view)
	}
}
