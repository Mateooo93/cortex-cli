package cortexconfig

import "testing"

func TestEffectiveModelContextWindow(t *testing.T) {
	tests := []struct {
		spec string
		want int64
	}{
		{"openai:gpt-4o", 128_000},
		{"anthropic:claude-opus-4-8", 200_000},
		{"openai:unknown-model", DefaultModelContextWindow},
		{"ollama:llama3.1", DefaultModelContextWindow},
		{"", DefaultModelContextWindow},
	}
	for _, tt := range tests {
		got := EffectiveModelContextWindow(tt.spec)
		if got != tt.want {
			t.Errorf("EffectiveModelContextWindow(%q) = %d, want %d", tt.spec, got, tt.want)
		}
	}
}
