package session

import (
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/memory"
)

func TestBuildSystemMessage_IncludesProjectMemory(t *testing.T) {
	dir := t.TempDir()
	cortexDir := dir + "/.cortex"
	store, err := memory.OpenAt(cortexDir+"/memory.db", cortexDir, memory.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Create("Use Go modules", memory.TypeConvention, 0.9, "test"); err != nil {
		t.Fatal(err)
	}

	cfg := cortexconfig.Default()
	s, err := New(Config{CortexCfg: cfg, Workdir: dir, ConfigDir: "", ActiveModel: cfg.DefaultModel})
	if err != nil {
		t.Fatal(err)
	}
	s.memoryStore = store
	s.memoryEnabled = true
	s.lastUserPrompt = "go modules"

	msg := s.buildSystemMessage()
	if !strings.Contains(msg, "Relevant project memories") {
		t.Fatalf("expected memory block in system message, got:\n%s", msg)
	}
	if !strings.Contains(msg, "Use Go modules") {
		t.Fatal("expected memory content in system message")
	}
}

func TestProjectMemoryEnabled_DefaultOn(t *testing.T) {
	paths := config.NewCortexPaths("", "", t.TempDir())
	if !config.ProjectMemoryEnabled(paths) {
		t.Fatal("project memory should default to enabled")
	}
}