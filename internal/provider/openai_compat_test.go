package provider

import (
	"errors"
	"testing"
)

func TestRouteAwareBaseURLAndModel_OpenGatewayScoped(t *testing.T) {
	p := NewOpenAICompat("opengateway", "test", "https://opengateway.gitlawb.com/v1")
	baseURL, model := p.routeAwareBaseURLAndModel("minimax/minimax-m3")
	if baseURL != "https://opengateway.gitlawb.com/v1/minimax" {
		t.Fatalf("baseURL = %q, want scoped minimax URL", baseURL)
	}
	if model != "minimax-m3" {
		t.Fatalf("model = %q, want raw minimax-m3", model)
	}
}

func TestRouteAwareBaseURLAndModel_OpenGatewayInfersOldUnscopedMiniMax(t *testing.T) {
	p := NewOpenAICompat("opengateway", "test", "https://opengateway.gitlawb.com/v1")
	baseURL, model := p.routeAwareBaseURLAndModel("minimax-m3")
	if baseURL != "https://opengateway.gitlawb.com/v1/minimax" {
		t.Fatalf("baseURL = %q, want inferred minimax URL", baseURL)
	}
	if model != "minimax-m3" {
		t.Fatalf("model = %q, want raw minimax-m3", model)
	}
}

func TestQualifyOpenGatewayModelsScopesRawIDs(t *testing.T) {
	got := qualifyOpenGatewayModels("https://opengateway.gitlawb.com/v1/minimax", []string{"minimax-m3"})
	if len(got) != 1 || got[0] != "minimax/minimax-m3" {
		t.Fatalf("qualified models = %#v, want minimax/minimax-m3", got)
	}
}

func TestShouldUseNonStreaming_FreeModels(t *testing.T) {
	p := NewOpenAICompat("openrouter", "test", "https://openrouter.ai/api/v1")
	for _, model := range []string{"mimo-v2.5-free", "open/mimo-v2.5-free", "meta-llama/free"} {
		if !p.shouldUseNonStreaming(model) {
			t.Fatalf("expected non-streaming for %q", model)
		}
	}
	if p.shouldUseNonStreaming("anthropic/claude-opus-4-8") {
		t.Fatal("paid models should stream")
	}
}

func TestProviderHTTPError_ParsesMessage(t *testing.T) {
	err := newProviderHTTPError("stream", 502, []byte(`{"error":{"message":"Request failed."}}`))
	if got := err.Error(); got != "stream: upstream error (502): Request failed." {
		t.Fatalf("error = %q", got)
	}
	var pe *ProviderHTTPError
	if !errors.As(err, &pe) || !isRetryableHTTPStatus(pe.Status) {
		t.Fatalf("expected retryable ProviderHTTPError, got %#v", err)
	}
}
