// Package codex is the ChatGPT-subscription provider for cortex-cli.
//
// It reuses the OpenAI-compatible transport from internal/provider by
// injecting the JWT from the OAuth login as a Bearer token, plus the
// `chatgpt-account-id` header that ChatGPT-subscription accounts
// require. This makes models like `gpt-5`, `gpt-5-codex`, and `o3`
// reachable through a user's normal ChatGPT plan (no separate
// OpenAI-API key required).
package codex

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Mateooo93/cortex-cli/internal/provider"
)

// New returns a provider.Provider that talks to api.openai.com using the
// stored ChatGPT-subscription OAuth token. If no token is stored, the
// returned provider will fail every request with a clear "not signed
// in" error — call Login first, or set the CODEX_CODEX_TOKEN env var.
//
// The returned provider is cheap to construct; it loads + (if needed)
// refreshes the token on every call so long-lived sessions don't have
// to worry about JWT expiry.
func New(ctx context.Context) (provider.Provider, error) {
	tok, err := Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("codex: load token: %w", err)
	}
	if tok == nil || tok.Access == "" {
		return nil, ErrNotSignedIn
	}
	return &Provider{
		name:    "codex",
		baseURL: "https://api.openai.com/v1",
		token:   tok,
		ctx:     ctx,
	}, nil
}

// ErrNotSignedIn is returned by New when no ChatGPT OAuth token is
// stored. The UI checks for this and launches the OAuth flow.
var ErrNotSignedIn = errors.New("codex: not signed in (run Settings → codex → Sign in)")

// Provider implements provider.Provider against api.openai.com using a
// ChatGPT OAuth token.
type Provider struct {
	name    string
	baseURL string
	token   *Token
	// ctx is the long-lived context used for background token refresh.
	// Per-request ctxs are passed in to Chat / Stream and take
	// precedence.
	ctx context.Context
}

// Name implements provider.Provider.
func (p *Provider) Name() string { return p.name }

// Token returns the currently active Token (refreshing if expired).
// The Token's AccountID, Email, and PlanType are useful for the
// Settings tab status line.
func (p *Provider) Token() *Token { return p.token }

// Chat implements provider.Provider. The token is reloaded for every
// call so a 1-hour JWT can be transparently refreshed mid-session.
func (p *Provider) Chat(ctx context.Context, req provider.Request) (provider.Response, error) {
	wrapper, err := p.wrapper()
	if err != nil {
		return provider.Response{}, err
	}
	return wrapper.Chat(ctx, req)
}

// Stream implements provider.Provider.
func (p *Provider) Stream(ctx context.Context, req provider.Request, onChunk func(provider.Chunk)) (provider.Response, error) {
	wrapper, err := p.wrapper()
	if err != nil {
		return provider.Response{}, err
	}
	return wrapper.Stream(ctx, req, onChunk)
}

// wrapper returns a fresh OpenAICompat with the current token's JWT as
// the Bearer and the chatgpt-account-id header populated. A new wrapper
// is built per request so the underlying apiKey/headerHook reflect the
// current Token state.
func (p *Provider) wrapper() (*provider.OpenAICompat, error) {
	// Reload + refresh if the cached token has gone stale.
	tok := p.token
	if tok.Expired() {
		fresh, err := Load(p.ctx)
		if err == nil && fresh != nil {
			tok = fresh
			p.token = fresh
		}
	}
	if tok == nil || tok.Access == "" {
		return nil, ErrNotSignedIn
	}
	wrapper := provider.NewOpenAICompat("codex", tok.Access, p.baseURL)
	if accountID := strings.TrimSpace(tok.AccountID); accountID != "" {
		wrapper.WithHeaderHook(func(r *http.Request) {
			r.Header.Set("chatgpt-account-id", accountID)
		})
	}
	return wrapper, nil
}

// Register installs the codex provider into the provider package's
// custom-provider table so New() can resolve provider == "codex".
// Called from main / init so the registration happens before any
// model is constructed.
func Register() {
	provider.RegisterCustom("codex", New)
}
