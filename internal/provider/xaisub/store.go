package xaisub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const keyringService = "cortex-cli"
const keyringUser = "xai-sub-oauth-token"

func load() (*Token, error) {
	raw, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		return nil, nil
	}
	if raw == "" {
		return nil, nil
	}
	var t Token
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return nil, fmt.Errorf("xaisub: stored token malformed: %w", err)
	}
	return &t, nil
}

func save(t *Token) error {
	if t == nil || t.Access == "" {
		return errors.New("xaisub: refusing to store empty token")
	}
	buf, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return keyring.Set(keyringService, keyringUser, string(buf))
}

func deleteToken() error {
	err := keyring.Delete(keyringService, keyringUser)
	if err != nil && !isNotFound(err) {
		return err
	}
	return nil
}

// Load resolves a usable Token from the environment first, then the keychain.
// Expired tokens with a refresh_token are refreshed automatically.
func Load(ctx context.Context) (*Token, error) {
	if v := os.Getenv("XAI_OAUTH_TOKEN"); v != "" {
		t := &Token{Access: v}
		if claims, ok := parseJWT(v); ok {
			t.Email = claims.Email
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
		fresh, err := Refresh(ctx, t.TokenEndpoint, t.Refresh)
		if err != nil {
			return t, nil
		}
		if fresh.Refresh == "" {
			fresh.Refresh = t.Refresh
		}
		if fresh.TokenEndpoint == "" {
			fresh.TokenEndpoint = t.TokenEndpoint
		}
		_ = save(fresh)
		return fresh, nil
	}
	return t, nil
}

// Delete removes the stored token (sign-out).
func Delete() error { return deleteToken() }

// Save persists t after a fresh login.
func Save(t *Token) error { return save(t) }

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