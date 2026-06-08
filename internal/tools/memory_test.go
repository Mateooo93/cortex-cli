package tools

import (
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/memory"
)

func TestMemoryWriteTool_Registered(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("memory_write"); !ok {
		t.Fatal("memory_write not registered")
	}
}

func TestMemoryWriteTool_Create(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.OpenAt(dir+"/memory.db", dir, memory.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	tool := &MemoryWriteTool{}
	res, err := tool.Run(Context{
		Memory:        store,
		MemoryEnabled: true,
	}, map[string]any{
		"content":    "Always use pnpm",
		"type":       "preference",
		"importance": 0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("create failed: %s", res.Error)
	}
}