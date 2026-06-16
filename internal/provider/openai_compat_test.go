package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestStreamToolCallsUseIndexForArgumentAssembly(t *testing.T) {
	events := []string{
		mustJSON(t, map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{
					"tool_calls": []any{
						map[string]any{
							"index": 0,
							"id":    "call_a",
							"type":  "function",
							"function": map[string]any{
								"name":      "write_file",
								"arguments": `{"path":"a.txt","content":"A`,
							},
						},
						map[string]any{
							"index": 1,
							"id":    "call_b",
							"type":  "function",
							"function": map[string]any{
								"name":      "write_file",
								"arguments": `{"path":"b.txt","content":"B`,
							},
						},
					},
				},
			}},
		}),
		mustJSON(t, map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{
					"tool_calls": []any{
						map[string]any{
							"index":    0,
							"function": map[string]any{"arguments": `1"}`},
						},
						map[string]any{
							"index":    1,
							"function": map[string]any{"arguments": `2"}`},
						},
					},
				},
			}},
		}),
		mustJSON(t, map[string]any{
			"choices": []any{map[string]any{
				"delta":         map[string]any{},
				"finish_reason": "tool_calls",
			}},
		}),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, ev := range events {
			fmt.Fprintf(w, "data: %s\n\n", ev)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := NewOpenAICompat("test", "", srv.URL)
	resp, err := p.Stream(context.Background(), Request{Model: "test"}, func(Chunk) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(resp.ToolCalls))
	}
	if got := resp.ToolCalls[0].Arguments["content"]; got != "A1" {
		t.Fatalf("first content = %#v, want A1", got)
	}
	if got := resp.ToolCalls[1].Arguments["content"]; got != "B2" {
		t.Fatalf("second content = %#v, want B2", got)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
