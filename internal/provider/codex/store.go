package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

// keyringService matches the constant in internal/config/keyring.go.
// Duplicated here so the codex package stays free of config-package
// import cycles (config already imports provider types).
const keyringService = "cortex-cli"

// keyringUser is the keychain "user" field under which the JSON-encoded
// Token is stored. The same user is also used in
// internal/config/keyring.go's env-var lookup for `CODEX_CODEX_TOKEN`,
// so users can also provision a token via the environment.
const keyringUser = "codex-oauth-token"

// load reads the stored Token. Returns (nil, nil) if not stored.
func load() (*Token, error) {
	raw, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		// keyring returns an error both for "not found" and for actual
		// transport errors. Treat any error as "not stored" so callers
		// can fall back to the env var or prompt the user.
		return nil, nil
	}
	if raw == "" {
		return nil, nil
	}
	var t Token
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return nil, fmt.Errorf("codex: stored token malformed: %w", err)
	}
	return &t, nil
}

// save persists the Token to the OS keychain. The token is JSON-encoded
// so the access + refresh + account_id can travel together.
func save(t *Token) error {
	if t == nil || t.Access == "" {
		return errors.New("codex: refusing to store empty token")
	}
	buf, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return keyring.Set(keyringService, keyringUser, string(buf))
}

// delete removes the stored Token. Idempotent.
func delete() error {
	err := keyring.Delete(keyringService, keyringUser)
	if err != nil && !isNotFound(err) {
		return err
	}
	return nil
}

// Load resolves a usable Token from the environment first, then the
// keychain. If the resolved token is expired and has a refresh_token,
// it transparently refreshes and re-saves.
func Load(ctx context.Context) (*Token, error) {
	if v := os.Getenv("CODEX_CODEX_TOKEN"); v != "" {
		// The env-var path is for headless / CI / docker uses. We don't
		// have a refresh token, so the user has to rotate it themselves.
		t := &Token{Access: v}
		if claims, ok := parseJWT(v); ok {
			t.AccountID = claims.AccountID
			t.Email = claims.Email
			t.PlanType = claims.PlanType
			t.ExpiresAt = claims.ExpiresAt
		}
		return t, nil
	}
	t, err := load()
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	if t.Expired() && t.Refresh != "" {
		fresh, err := Refresh(ctx, t.Refresh)
		if err != nil {
			// Refresh failed — surface the (still-old) token. The next
			// API call will likely 401 and the UI will prompt the user
			// to sign in again.
			return t, nil
		}
		// Preserve the refresh token if the server didn't return one
		// (some IdPs rotate, some don't).
		if fresh.Refresh == "" {
			fresh.Refresh = t.Refresh
		}
		_ = save(fresh)
		return fresh, nil
	}
	return t, nil
}

// Delete removes the stored token (sign-out). Idempotent.
func Delete() error { return delete() }

// Save persists t. Exported so the OAuth flow can write back the result
// of a fresh login.
func Save(t *Token) error { return save(t) }

// isNotFound checks the keyring's "not found" sentinel across versions.
func isNotFound(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	for _, s := range []string{"not found", "no such", "secrets not found", "ErrNotFound"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
