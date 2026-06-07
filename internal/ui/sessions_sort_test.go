package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/daemon"
)

// TestRenderSessionsView_SortsByDateNewestFirst pins the
// user-reported bug: "sort sessions by date from newest
// (top) to oldest (bottom)".
func TestRenderSessionsView_SortsByDateNewestFirst(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	daemon.SetGlobalConfigLoader(func() *cortexconfig.Config { return cortexCfg })

	m := NewModel(cfg, cortexCfg, nil, false, "", true, true)
	m.width = 140
	m.height = 35

	// Replace any restored sessions with three sessions
	// at distinct, non-equal createdAt times so we can
	// assert the order. The labels are unique so the
	// position-in-string check is reliable.
	now := time.Now()
	m.sessions = []*SessionState{
		{createdAt: now.Add(-2 * time.Hour), label: "OLDEST", agentState: StateWaitingForInput, modelName: "a"},
		{createdAt: now.Add(-1 * time.Hour), label: "MIDDLE", agentState: StateWaitingForInput, modelName: "b"},
		{createdAt: now, label: "NEWEST", agentState: StateWaitingForInput, modelName: "c"},
	}

	view := renderSessionsView(m.sessions, 140, 35, NewStyles(true), "", "", 0)

	posNewest := strings.Index(view, "NEWEST")
	posMiddle := strings.Index(view, "MIDDLE")
	posOldest := strings.Index(view, "OLDEST")
	if posNewest < 0 || posMiddle < 0 || posOldest < 0 {
		t.Fatalf("expected all three labels in view: posNewest=%d posMiddle=%d posOldest=%d",
			posNewest, posMiddle, posOldest)
	}
	if !(posNewest < posMiddle && posMiddle < posOldest) {
		t.Errorf("expected NEWEST < MIDDLE < OLDEST in render, got NEWEST=%d MIDDLE=%d OLDEST=%d",
			posNewest, posMiddle, posOldest)
	}
}

// TestVisibleSessionIndices_SortedByDate verifies that
// the visibleSessionIndices helper returns indices in
// the same order as the rendered list (newest first).
func TestVisibleSessionIndices_SortedByDate(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	daemon.SetGlobalConfigLoader(func() *cortexconfig.Config { return cortexCfg })

	m := NewModel(cfg, cortexCfg, nil, false, "", true, true)

	// Build a self-contained 3-session list with
	// distinct createdAt values.
	now := time.Now()
	m.sessions = []*SessionState{
		{createdAt: now.Add(-2 * time.Hour), label: "A", agentState: StateWaitingForInput},
		{createdAt: now.Add(-1 * time.Hour), label: "B", agentState: StateWaitingForInput},
		{createdAt: now, label: "C", agentState: StateWaitingForInput},
	}
	indices := m.visibleSessionIndices()
	if len(indices) != 3 {
		t.Fatalf("expected 3 indices, got %d", len(indices))
	}
	// Expected order (newest first): C (index 2), B (1), A (0)
	if indices[0] != 2 {
		t.Errorf("expected indices[0]=2 (newest, label=C), got %d", indices[0])
	}
	if indices[1] != 1 {
		t.Errorf("expected indices[1]=1 (middle, label=B), got %d", indices[1])
	}
	if indices[2] != 0 {
		t.Errorf("expected indices[2]=0 (oldest, label=A), got %d", indices[2])
	}
}
