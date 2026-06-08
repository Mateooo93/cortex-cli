package config

import "testing"

func TestOAuthProviderStatusPrefix_Unknown(t *testing.T) {
	if got := OAuthProviderStatusPrefix("openai"); got != "" {
		t.Fatalf("openai status = %q, want empty", got)
	}
}

func TestOAuthProviderSignedIn_Unknown(t *testing.T) {
	if OAuthProviderSignedIn("anthropic") {
		t.Fatal("anthropic should not report oauth signed in")
	}
}