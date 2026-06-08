package xaisub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscover_UsesOpenIDConfiguration(t *testing.T) {
	discoveryMu.Lock()
	discoveryCache = nil
	discoveryMu.Unlock()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(discovery{
			AuthorizationEndpoint: "https://auth.x.ai/oauth2/authorize",
			TokenEndpoint:         "https://auth.x.ai/oauth2/token",
		})
	}))
	defer srv.Close()

	oldURL := DiscoveryURL
	t.Cleanup(func() {
		DiscoveryURL = oldURL
		discoveryMu.Lock()
		discoveryCache = nil
		discoveryMu.Unlock()
	})
	DiscoveryURL = srv.URL

	d, err := Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !strings.HasSuffix(d.AuthorizationEndpoint, "/oauth2/authorize") {
		t.Fatalf("authorize endpoint = %q", d.AuthorizationEndpoint)
	}
	if !strings.HasSuffix(d.TokenEndpoint, "/oauth2/token") {
		t.Fatalf("token endpoint = %q", d.TokenEndpoint)
	}
}

func TestValidateEndpoint_RejectsUntrustedHost(t *testing.T) {
	if err := validateEndpoint("https://evil.example.com/token"); err == nil {
		t.Fatal("expected untrusted host to be rejected")
	}
}

func TestParseJWT_ExtractsEmail(t *testing.T) {
	// header.payload.signature — payload is {"email":"user@x.ai","exp":2000000000}
	payload := "eyJlbWFpbCI6InVzZXJAeC5haSIsImV4cCI6MjAwMDAwMDAwMH0"
	token := "aaa." + payload + ".bbb"
	claims, ok := parseJWT(token)
	if !ok {
		t.Fatal("parseJWT failed")
	}
	if claims.Email != "user@x.ai" {
		t.Fatalf("email = %q", claims.Email)
	}
}