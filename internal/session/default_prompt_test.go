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
	for _, want := range []string{"DO the task", "For large writes", "Split file creation/rewrites", "read_file", "why", "Only change files", "background=true", "timeout_sec"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("default prompt missing %q, got:\n%s", want, prompt)
		}
	}
}

func TestBuildSystemPrompt_IncludesWorkdir(t *testing.T) {
	prompt := BuildSystemPrompt("/home/user/myproject")
	if !strings.Contains(prompt, "/home/user/myproject") {
		t.Fatalf("expected workdir in prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Working directory:") {
		t.Fatalf("expected working directory section, got:\n%s", prompt)
	}
	for _, want := range []string{
		"list_dir",
		`path "."`,
		"home/user/myproject",
		"never cd just to",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildSystemPrompt_RequiresVisibleNarration(t *testing.T) {
	prompt := BuildSystemPrompt("/tmp")
	for _, want := range []string{
		"Do NOT put user-facing narration only inside thinking",
		"Before every tool batch",
		"visible chat",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
