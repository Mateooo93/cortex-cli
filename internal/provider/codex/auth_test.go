package codex

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

// makeJWT builds a JWT with the given payload (encoded as JSON).
// Header is the standard alg=none, typ=JWT — the parser doesn't
// verify signatures because we only ever inspect the claims.
func makeJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	header := `{"alg":"none","typ":"JWT"}`
	// JWT uses raw base64url without padding; parseJWT uses
	// RawURLEncoding, so we have to match.
	b64 := func(s string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(s))
	}
	return b64(header) + "." + b64(mustJSON(t, payload)) + "."
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// --- parseJWT tests ---

func TestParseJWT_FullClaims(t *testing.T) {
	exp := time.Now().Add(1 * time.Hour).Unix()
	jwt := makeJWT(t, map[string]any{
		"email": "user@example.com",
		"exp":   exp,
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct_abc123",
			"chatgpt_plan_type":  "plus",
		},
	})
	c, ok := parseJWT(jwt)
	if !ok {
		t.Fatal("parseJWT returned ok=false")
	}
	if c.Email != "user@example.com" {
		t.Errorf("Email = %q, want user@example.com", c.Email)
	}
	if c.AccountID != "acct_abc123" {
		t.Errorf("AccountID = %q, want acct_abc123", c.AccountID)
	}
	if c.PlanType != "plus" {
		t.Errorf("PlanType = %q, want plus", c.PlanType)
	}
	if c.ExpiresAt.Unix() != exp {
		t.Errorf("ExpiresAt = %d, want %d", c.ExpiresAt.Unix(), exp)
	}
}

func TestParseJWT_NotAJWT(t *testing.T) {
	cases := []string{
		"",
		"abc",
		"abc.def",
		"abc.def.ghi.jkl", // 4 parts
		"!!notbase64!!.!!notbase64!!.!!notbase64!!",
	}
	for _, s := range cases {
		if _, ok := parseJWT(s); ok {
			t.Errorf("parseJWT(%q) = ok=true, want false", s)
		}
	}
}

func TestParseJWT_NoRecognisableClaims(t *testing.T) {
	// Valid JWT shape, empty payload.
	jwt := makeJWT(t, map[string]any{"sub": "x"})
	if _, ok := parseJWT(jwt); ok {
		t.Errorf("parseJWT with no recognisable claims = ok=true, want false")
	}
}

func TestParseJWT_FallsBackToUserID(t *testing.T) {
	// Some token shapes nest account_id under "user" — make sure we
	// still pick it up.
	jwt := makeJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"user": map[string]any{
				"id": "user_xyz",
			},
		},
	})
	c, ok := parseJWT(jwt)
	if !ok {
		t.Fatal("parseJWT returned ok=false")
	}
	if c.AccountID != "user_xyz" {
		t.Errorf("AccountID = %q, want user_xyz (fallback to user.id)", c.AccountID)
	}
}

// --- Token.Expired tests ---

func TestToken_Expired(t *testing.T) {
	cases := []struct {
		name string
		t    Token
		want bool
	}{
		{"zero time is expired", Token{ExpiresAt: time.Time{}}, true},
		{"5 min in the past", Token{ExpiresAt: time.Now().Add(-5 * time.Minute)}, true},
		{"5 min in the future", Token{ExpiresAt: time.Now().Add(5 * time.Minute)}, false},
		{"1 hour in the future", Token{ExpiresAt: time.Now().Add(1 * time.Hour)}, false},
		// 60s-before-expiry grace: a token that expires in 30s is
		// treated as already expired so we don't race the API.
		{"30s in the future (within grace window)", Token{ExpiresAt: time.Now().Add(30 * time.Second)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.t.Expired(); got != c.want {
				t.Errorf("Expired() = %v, want %v", got, c.want)
			}
		})
	}
}

// --- mask test ---

func TestMask(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"", 8, ""},
		// "short" is 5 chars; n=8 >= len, so we mask the whole thing.
		{"short", 8, "•••••"},
		// "abcdefghij" is 10 chars; n=4, so the last 4 are visible
		// and the first 6 are masked.
		{"abcdefghij", 4, "••••••ghij"},
	}
	for _, c := range cases {
		if got := mask(c.in, c.n); got != c.want {
			t.Errorf("mask(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}
