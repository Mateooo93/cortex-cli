package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/provider"
)

func TestExecuteToolCalls_ParallelPreservesHistoryOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := cortexconfig.Default()
	cfg.Tools.AllowShell = true

	sess, err := New(Config{CortexCfg: cfg, Workdir: dir, ActiveModel: cfg.DefaultModel})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	calls := []provider.ToolCall{
		{ID: "slow", Name: "run_shell", Arguments: map[string]any{"command": "sleep 0.15; echo slow"}},
		{ID: "fast", Name: "run_shell", Arguments: map[string]any{"command": "sleep 0.01; echo fast"}},
	}
	start := time.Now()
	sess.executeToolCalls(context.Background(), calls)
	elapsed := time.Since(start)

	// Parallel: both sleeps overlap — should finish well under 0.30s sequential budget.
	if elapsed > 280*time.Millisecond {
		t.Fatalf("expected parallel execution (~0.15s), took %v", elapsed)
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()
	if len(sess.history) != 2 {
		t.Fatalf("history len = %d, want 2", len(sess.history))
	}
	if sess.history[0].ToolCallID != "slow" || sess.history[1].ToolCallID != "fast" {
		t.Fatalf("history order = [%s, %s], want [slow, fast]",
			sess.history[0].ToolCallID, sess.history[1].ToolCallID)
	}
}