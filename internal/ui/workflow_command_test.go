package ui

import (
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestHandleCommandAction_WorkflowWithPrompt_StartsEngine
// verifies that `/workflow build a CLI todo app in Go` (as a
// raw slash command, not the menu shortcut) starts a
// workflow with the supplied prompt. Before this fix the
// slash command only opened a preset picker.
func TestHandleCommandAction_WorkflowWithPrompt_StartsEngine(t *testing.T) {
	cfg := &cortexconfig.Config{}
	cfg.EnsureProviderPresets()
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)
	// Sanity: no workflows yet.
	sess := m.currentSession()
	if len(sess.workflowEngine.Workflows()) != 0 {
		t.Fatal("fresh session should have no workflows")
	}
	cmds := m.handleCommandAction("open_workflow_picker", sess, "build a CLI todo app in Go")
	_ = cmds
	if len(sess.workflowEngine.Workflows()) != 1 {
		t.Fatalf("after /workflow <prompt>, expected 1 workflow, got %d", len(sess.workflowEngine.Workflows()))
	}
}

// TestHandleCommandAction_WorkflowWithoutPrompt_EmitsError
// verifies that `/workflow` (no prompt) shows a usage error
// in the status bar instead of silently doing nothing.
func TestHandleCommandAction_WorkflowWithoutPrompt_EmitsError(t *testing.T) {
	cfg := &cortexconfig.Config{}
	cfg.EnsureProviderPresets()
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)
	sess := m.currentSession()
	cmds := m.handleCommandAction("open_workflow_picker", sess, "")
	_ = cmds
	if m.statusMsg.Text == "" {
		t.Error("expected status message after empty /workflow")
	}
	if !strings.Contains(strings.ToLower(m.statusMsg.Text), "usage") &&
		!strings.Contains(m.statusMsg.Text, "/workflow") {
		t.Errorf("status message should mention usage or /workflow, got %q", m.statusMsg.Text)
	}
	if len(sess.workflowEngine.Workflows()) != 0 {
		t.Errorf("empty /workflow should not start a workflow, got %d", len(sess.workflowEngine.Workflows()))
	}
}
