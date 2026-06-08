package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestWorkflowPicker_OpenPrefillsPrompt(t *testing.T) {
	wp := NewWorkflowPicker()
	wp.Open("build a CLI todo app in Go")
	if !wp.IsVisible() {
		t.Fatal("picker should be visible")
	}
	if wp.Prompt() != "build a CLI todo app in Go" {
		t.Fatalf("prompt = %q", wp.Prompt())
	}
	sel := wp.Selected()
	if sel == nil || sel.Name != "code" {
		t.Fatalf("expected code preset for build task, got %#v", sel)
	}
}

func TestWorkflowPicker_ViewShowsPromptField(t *testing.T) {
	wp := NewWorkflowPicker()
	wp.Open("write tests for auth module")
	view := wp.View(100, NewStyles(true))
	plain := stripANSI(view)
	for _, want := range []string{"Start workflow", "Task prompt", "Preset", "code", "test"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in view:\n%s", want, view)
		}
	}
}

func TestWorkflowPicker_EnterRequiresPrompt(t *testing.T) {
	wp := NewWorkflowPicker()
	wp.Open("")
	result, consumed := wp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !consumed {
		t.Fatal("Enter should be consumed")
	}
	if result != nil {
		t.Fatal("empty prompt should not start workflow")
	}
	if !wp.IsVisible() {
		t.Fatal("picker should stay open when prompt is empty")
	}
}

func TestWorkflowPicker_EnterStartsWithPromptAndPreset(t *testing.T) {
	wp := NewWorkflowPicker()
	wp.Open("run the tests for login flow")
	result, consumed := wp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !consumed || result == nil {
		t.Fatalf("expected start result, consumed=%v result=%v", consumed, result)
	}
	if result.Prompt != "run the tests for login flow" {
		t.Fatalf("prompt = %q", result.Prompt)
	}
	if result.Preset.Name != "test" {
		t.Fatalf("preset = %q, want test", result.Preset.Name)
	}
	if wp.IsVisible() {
		t.Fatal("picker should close after confirm")
	}
}

func TestOpenWorkflowPicker_ActionPrefillsArgs(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]

	m.handleCommandAction("open_workflow_picker", sess, "build a REST API in Go")
	if !m.workflowPicker.IsVisible() {
		t.Fatal("workflow picker should be visible")
	}
	if m.workflowPicker.Prompt() != "build a REST API in Go" {
		t.Fatalf("prompt = %q", m.workflowPicker.Prompt())
	}
}

func TestTryDispatchSlashInput_WorkflowOpensPicker(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]

	_, handled := m.tryDispatchSlashInput(sess, "/workflow add pagination to the API")
	if !handled {
		t.Fatal("expected /workflow to be handled")
	}
	if !m.workflowPicker.IsVisible() {
		t.Fatal("workflow picker should open")
	}
	if m.workflowPicker.Prompt() != "add pagination to the API" {
		t.Fatalf("prompt = %q", m.workflowPicker.Prompt())
	}
}

func TestWorkflowPicker_ModelUpdateStartsWorkflow(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	m.workflowPicker.Open("implement dark mode toggle")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	um := updated.(Model)
	if um.workflowPicker.IsVisible() {
		t.Fatal("picker should close after start")
	}
	if sess.activeWorkflow == "" {
		t.Fatal("expected activeWorkflow to be set")
	}
	if um.activeTab != TabKindWorkflows {
		t.Fatalf("activeTab = %v, want workflows", um.activeTab)
	}
}