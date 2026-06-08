package session

import (
	"strings"
	"testing"
)

// TestDefaultSystemPrompt_ContainsThinkTags pins the
// user-reported request: "hide model thinking (if it
// doesnt emit tags when thinking, tell it to so we
// can differentiate) unless specified otherwise in
// settings". The default system prompt must tell the
// model to use <think>...</think> tags for its
// internal reasoning, and to keep the actual
// user-visible response OUTSIDE those tags. The
// UI's "show extended thinking" toggle (default
// off) gates the visibility of the <think> content.
func TestDefaultSystemPrompt_ContainsThinkTags(t *testing.T) {
	prompt := DefaultSystemPrompt()
	if !strings.Contains(prompt, "<think>") {
		t.Errorf("default system prompt must mention <think> tags so the model emits them, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "</think>") {
		t.Errorf("default system prompt must mention </think> closing tags, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "hides these by default") {
		t.Errorf("default system prompt must note that <think> content is hidden by default, got:\n%s", prompt)
	}
	for _, want := range []string{"DO the task", "For large writes", "Split file creation/rewrites", "Narrate selectively"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("default prompt missing %q, got:\n%s", want, prompt)
		}
	}
}
