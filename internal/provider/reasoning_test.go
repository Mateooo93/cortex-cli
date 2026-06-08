package provider

import "testing"

func TestSupportsReasoningEffort(t *testing.T) {
	cases := []struct {
		provider string
		model    string
		want     bool
	}{
		{"openai", "gpt-4o", false},
		{"openai", "gpt-5.2", true},
		{"openai", "o3-mini", true},
		{"anthropic", "claude-opus-4-8", false},
		{"ollama", "qwen3.5", true},
		{"ollama", "llama3.2", false},
		{"cortex", "cortex-code", true},
		{"groq", "llama-3.3-70b-versatile", false},
	}
	for _, c := range cases {
		got := SupportsReasoningEffort(c.provider, c.model)
		if got != c.want {
			t.Fatalf("SupportsReasoningEffort(%q, %q) = %v, want %v", c.provider, c.model, got, c.want)
		}
	}
}

func TestRequestReasoningEffort_DropsUnsupported(t *testing.T) {
	if got := RequestReasoningEffort("openai", "gpt-4o", "high"); got != "" {
		t.Fatalf("got %q, want empty for unsupported model", got)
	}
	if got := RequestReasoningEffort("openai", "gpt-5.2", "high"); got != "high" {
		t.Fatalf("got %q, want high", got)
	}
	if got := RequestReasoningEffort("openai", "gpt-5.2", "ultracode"); got != "xhigh" {
		t.Fatalf("got %q, want xhigh", got)
	}
}