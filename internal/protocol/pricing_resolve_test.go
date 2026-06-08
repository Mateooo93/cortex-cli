package protocol

import "testing"

func TestResolvePricingSpec(t *testing.T) {
	if got := ResolvePricingSpec("openai", "openai", "gpt-5.5"); got != "openai/gpt-5.5" {
		t.Fatalf("got %q", got)
	}
	if got := ResolvePricingSpec("anthropic/claude-opus-4-8", "", ""); got != "anthropic/claude-opus-4-8" {
		t.Fatalf("got %q", got)
	}
}

func TestCalculateCost_ConfigKeyOpenAI(t *testing.T) {
	cost := CalculateCost(ResolvePricingSpec("openai", "openai", "gpt-5.5"), 1_000_000, 100_000, 0, 0)
	if cost < 4.0 || cost > 4.5 {
		t.Fatalf("expected non-zero openai cost, got %f", cost)
	}
}

func TestCalculateCost_OllamaLocal(t *testing.T) {
	cost := CalculateCost("ollama/qwen3.5", 1_000_000, 1_000_000, 0, 0)
	if cost != 0 {
		t.Fatalf("expected free local cost, got %f", cost)
	}
}