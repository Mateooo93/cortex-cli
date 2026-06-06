package provider

import "testing"

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
