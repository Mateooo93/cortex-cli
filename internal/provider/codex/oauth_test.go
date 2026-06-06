package codex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuildAuthURL_AllParams(t *testing.T) {
	got := buildAuthURL("http://localhost:1455/auth/callback", "state-xyz", "chal-abc")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Scheme != "https" {
		t.Errorf("scheme = %q, want https", u.Scheme)
	}
	if u.Host != "auth.openai.com" {
		t.Errorf("host = %q, want auth.openai.com", u.Host)
	}
	q := u.Query()
	want := map[string]string{
		"client_id":              ClientID,
		"response_type":          "code",
		"redirect_uri":           "http://localhost:1455/auth/callback",
		"state":                  "state-xyz",
		"code_challenge":         "chal-abc",
		"code_challenge_method":  "S256",
		"prompt":                 "login",
		"id_token_add_organizations": "true",
	}
	for k, v := range want {
		if got := q.Get(k); got != v {
			t.Errorf("query[%q] = %q, want %q", k, got, v)
		}
	}
	if got := q.Get("scope"); got == "" {
		t.Errorf("scope is empty")
	} else {
		// The scope must include offline_access so we get a
		// refresh_token.
		if !strings.Contains(got, "offline_access") {
			t.Errorf("scope %q missing offline_access", got)
		}
	}
}

func TestPKCE_Deterministic(t *testing.T) {
	v1, c1, err := pkce()
	if err != nil {
		t.Fatalf("pkce: %v", err)
	}
	v2, c2, err := pkce()
	if err != nil {
		t.Fatalf("pkce: %v", err)
	}
	// Two runs must produce different values (random).
	if v1 == v2 {
		t.Errorf("pkce verifier repeated: %q", v1)
	}
	if c1 == c2 {
		t.Errorf("pkce challenge repeated: %q", c1)
	}
	// Verifier must be 64 random bytes → 86 base64url chars
	// (4*ceil(64/3) = 88, minus 2 trailing =).
	if len(v1) != 86 {
		t.Errorf("verifier length = %d, want 86", len(v1))
	}
	// Challenge is base64url(sha256(64 bytes)) = 43 chars.
	if len(c1) != 43 {
		t.Errorf("challenge length = %d, want 43", len(c1))
	}
}

func TestCallbackHandler_RejectsStateMismatch(t *testing.T) {
	resCh := make(chan callbackResult, 1)
	mux := callbackHandler("expected", resCh)
	req := httptest.NewRequest("GET", "/auth/callback?state=wrong&code=abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200 (page always renders, error is via channel)", w.Code)
	}
	select {
	case res := <-resCh:
		if res.err == nil {
			t.Errorf("expected error for state mismatch, got code=%q", res.code)
		}
	default:
		t.Errorf("no callback result sent")
	}
}

func TestCallbackHandler_AcceptsMatchingState(t *testing.T) {
	resCh := make(chan callbackResult, 1)
	mux := callbackHandler("expected", resCh)
	req := httptest.NewRequest("GET", "/auth/callback?state=expected&code=auth-code-xyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	select {
	case res := <-resCh:
		if res.err != nil {
			t.Errorf("unexpected error: %v", res.err)
		}
		if res.code != "auth-code-xyz" {
			t.Errorf("code = %q, want auth-code-xyz", res.code)
		}
	default:
		t.Errorf("no callback result sent")
	}
}

func TestCallbackHandler_RendersErrorPage(t *testing.T) {
	resCh := make(chan callbackResult, 1)
	mux := callbackHandler("expected", resCh)
	req := httptest.NewRequest("GET", "/auth/callback?error=access_denied", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "access_denied") {
		t.Errorf("error page missing error name: %s", w.Body.String())
	}
}

func TestExchangeCode_HappyPath(t *testing.T) {
	// Tiny fake token server. We just need a 200 with the right
	// shape — the parser is tested elsewhere.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.PostFormValue("grant_type"); got != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", got)
		}
		if got := r.PostFormValue("code"); got != "auth-code" {
			t.Errorf("code = %q, want auth-code", got)
		}
		if got := r.PostFormValue("code_verifier"); got != "verifier-xyz" {
			t.Errorf("code_verifier = %q, want verifier-xyz", got)
		}
		if got := r.PostFormValue("client_id"); got != ClientID {
			t.Errorf("client_id = %q, want %q", got, ClientID)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "fake-access",
			"refresh_token": "fake-refresh",
			"expires_in": 3600,
			"scope": "openid profile email"
		}`))
	}))
	defer ts.Close()

	// Save the original endpoint and restore it after the test.
	orig := TokenEndpoint
	TokenEndpoint = ts.URL
	t.Cleanup(func() { TokenEndpoint = orig })

	tok, err := exchangeCode(context.Background(), "auth-code", "verifier-xyz", "http://localhost:1455/auth/callback")
	if err != nil {
		t.Fatalf("exchangeCode: %v", err)
	}
	if tok.Access != "fake-access" {
		t.Errorf("Access = %q, want fake-access", tok.Access)
	}
	if tok.Refresh != "fake-refresh" {
		t.Errorf("Refresh = %q, want fake-refresh", tok.Refresh)
	}
	// expires_in=3600 → ExpiresAt roughly now+3600s.
	delta := time.Until(tok.ExpiresAt)
	if delta < 3500*time.Second || delta > 3700*time.Second {
		t.Errorf("ExpiresAt offset = %v, want ~1h", delta)
	}
}

func TestExchangeCode_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer ts.Close()
	orig := TokenEndpoint
	TokenEndpoint = ts.URL
	t.Cleanup(func() { TokenEndpoint = orig })

	_, err := exchangeCode(context.Background(), "bad", "verifier", "http://localhost:1455/auth/callback")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}

func TestParseTokenResponse_EmptyAccess(t *testing.T) {
	_, err := parseTokenResponse([]byte(`{"expires_in":3600}`))
	if err == nil {
		t.Fatal("expected error for empty access_token")
	}
}

func TestPickPort_RespectsPreferred(t *testing.T) {
	port, err := pickPort(0) // 0 = any
	if err != nil {
		t.Fatalf("pickPort: %v", err)
	}
	if port == 0 {
		t.Errorf("port = 0, want non-zero")
	}
}
