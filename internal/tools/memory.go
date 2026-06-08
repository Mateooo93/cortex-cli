package tools

import (
	"fmt"
	"strings"

	"github.com/Mateooo93/cortex-cli/internal/memory"
)

// MemoryWriteTool lets the agent persist durable project-scoped facts.
// The session layer attaches the store via tools.Context.Memory.
type MemoryWriteTool struct{}

func (t *MemoryWriteTool) Name() string { return "memory_write" }

func (t *MemoryWriteTool) Description() string {
	return "Save a durable project-scoped memory for future sessions in this repository. " +
		"Use sparingly — only for reusable facts (stack, conventions, architecture, workflow preferences). " +
		"Do NOT save temporary tasks, bugs in progress, branch names, or session-specific notes. " +
		"Keep each memory under 500 characters. The store caps at ~100 entries."
}

func (t *MemoryWriteTool) Parameters() map[string]Param {
	return map[string]Param{
		"content": {
			Type:        "string",
			Description: "The durable fact to remember for this project.",
			Required:    true,
		},
		"type": {
			Type:        "string",
			Description: "Category: preference | convention | architecture | workflow | project_fact",
			Required:    true,
		},
		"importance": {
			Type:        "number",
			Description: "0.55–1.0 relevance score. Use ≥0.75 only for high-value facts.",
			Required:    false,
		},
	}
}

func (t *MemoryWriteTool) Run(ctx Context, args map[string]any) (Result, error) {
	if !ctx.MemoryEnabled {
		return Result{OK: false, Error: "project memory is disabled in settings"}, nil
	}
	if ctx.Memory == nil {
		return Result{OK: false, Error: "memory store unavailable"}, nil
	}
	content, _ := args["content"].(string)
	typStr, _ := args["type"].(string)
	importance := 0.7
	if v, ok := args["importance"].(float64); ok {
		importance = v
	}
	entry, err := ctx.Memory.Create(content, memory.Type(strings.TrimSpace(typStr)), importance, "agent")
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	return Result{
		OK:     true,
		Output: fmt.Sprintf("Saved memory %s (%s)", entry.ID[:8], entry.Type),
		Details: map[string]any{
			"id":      entry.ID,
			"type":    entry.Type,
			"content": entry.Content,
		},
	}, nil
}