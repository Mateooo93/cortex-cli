package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestWorkflowsTab_NilEngine_DoesNotPanic covers the
// regression that crashed cortex-cli when the user pressed
// F3 to switch to the Workflows tab before any workflow had
// been started. The engine was nil and calling
// sess.workflowEngine.Workflows() panicked with a nil
// pointer dereference. After the fix, the Workflows tab
// lazy-initialises the engine on entry and shows the
// empty-state prompt instead of crashing.
func TestWorkflowsTab_NilEngine_DoesNotPanic(t *testing.T) {
	cfg := &cortexconfig.Config{}
	cfg.EnsureProviderPresets()
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)
	// Switch to the Workflows tab.
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	// Try to navigate (up/down) — this used to panic
	// because the per-session engine was nil.
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	// Engine should be auto-initialised now.
	sess := m.currentSession()
	if sess == nil || sess.workflowEngine == nil {
		t.Fatal("workflow engine should be auto-initialised on F3")
	}
}
