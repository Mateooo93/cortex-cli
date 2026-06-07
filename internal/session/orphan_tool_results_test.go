package session

import (
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/provider"
)

// TestStripOrphanToolResults_DropsMatching pins
// the user-reported MiniMax HTTP 400 fix: a
// `role: tool` message whose tool_call_id doesn't
// match any tool_call in the previous assistant
// message is dropped from the outgoing history.
// Strict providers (MiniMax, some OpenRouter
// backends) reject the request with
//   tool result's tool id() not found
// otherwise.
func TestStripOrphanToolResults_DropsMatching(t *testing.T) {
	in := []provider.Message{
		// System message: pass through
		{Role: "system", Content: "you are a coder"},
		// User message: pass through
		{Role: "user", Content: "edit foo.go"},
		// Assistant with one tool call (id=X)
		{Role: "assistant", Content: "editing...",
			ToolCalls: []provider.ToolCall{
				{ID: "X", Name: "edit_file", Arguments: map[string]any{"path": "foo.go"}},
			}},
		// Tool result for X: VALID, keep
		{Role: "tool", Content: "edited", ToolName: "edit_file", ToolCallID: "X"},
		// Tool result for Y: ORPHAN (no matching
		// tool_call in the assistant message), drop
		{Role: "tool", Content: "ghost", ToolName: "edit_file", ToolCallID: "Y"},
		// Assistant follow-up
		{Role: "assistant", Content: "done"},
	}
	out := stripOrphanToolResults(in)

	// We expect 5 messages: system, user, assistant,
	// tool (X), assistant. The orphan tool (Y) is
	// dropped.
	if len(out) != 5 {
		t.Fatalf("expected 5 messages, got %d: %+v", len(out), out)
	}
	// Verify the surviving tool message is X.
	if out[3].Role != "tool" || out[3].ToolCallID != "X" {
		t.Errorf("expected tool X to survive, got %+v", out[3])
	}
	// Verify Y was dropped.
	for _, m := range out {
		if m.Role == "tool" && m.ToolCallID == "Y" {
			t.Errorf("orphan tool Y should have been dropped, but survived: %+v", m)
		}
	}
}

// TestStripOrphanToolResults_ResetOnNewAssistantTurn
// pins the per-turn reset: a tool_call_id from
// turn N is NOT valid for the following turn N+1.
// Each assistant message "owns" its own set of
// valid tool_call_ids.
func TestStripOrphanToolResults_ResetOnNewAssistantTurn(t *testing.T) {
	in := []provider.Message{
		// Turn 1: assistant calls tool X
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "X", Name: "edit_file"}}},
		// Turn 1: result for X
		{Role: "tool", ToolCallID: "X", ToolName: "edit_file", Content: "ok"},
		// Turn 2: assistant calls tool Y (X is
		// no longer valid)
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "Y", Name: "edit_file"}}},
		// Turn 2: result for Y: VALID
		{Role: "tool", ToolCallID: "Y", ToolName: "edit_file", Content: "ok"},
		// Stray result for X from turn 1: ORPHAN
		// (turn 2's assistant message only declared
		// tool_calls=[Y], so X is no longer
		// "owned" by a valid assistant message)
		{Role: "tool", ToolCallID: "X", ToolName: "edit_file", Content: "stale"},
	}
	out := stripOrphanToolResults(in)
	// Should keep: 2 assistant + 2 tool results (X
	// from turn 1, Y from turn 2) = 4. The
	// "stale X" after turn 2's assistant is
	// dropped.
	toolCount := 0
	for _, m := range out {
		if m.Role == "tool" {
			toolCount++
		}
	}
	if toolCount != 2 {
		t.Errorf("expected 2 tool results, got %d", toolCount)
	}
}

// TestStripOrphanToolResults_NoAssistantPrefix
// pins the edge case: a tool message at the start
// of the history (no preceding assistant message)
// is dropped because no tool_call_id can be valid.
func TestStripOrphanToolResults_NoAssistantPrefix(t *testing.T) {
	in := []provider.Message{
		{Role: "tool", ToolCallID: "X", Content: "orphan"},
		{Role: "assistant", Content: "hi"},
	}
	out := stripOrphanToolResults(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out))
	}
	if out[0].Role != "assistant" {
		t.Errorf("expected only assistant message to survive, got %+v", out[0])
	}
}

// TestStripOrphanToolResults_EmptyInput pins the
// no-op case: an empty history is returned as-is.
func TestStripOrphanToolResults_EmptyInput(t *testing.T) {
	out := stripOrphanToolResults(nil)
	if len(out) != 0 {
		t.Errorf("expected empty output for nil input, got %d", len(out))
	}
}
