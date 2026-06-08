package xaisub

import (
	"context"
	"errors"
	"fmt"

	"github.com/Mateooo93/cortex-cli/internal/provider"
)

// New returns a provider that talks to api.x.ai using a Grok OAuth token.
func New(ctx context.Context) (provider.Provider, error) {
	tok, err := Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("xaisub: load token: %w", err)
	}
	if tok == nil || tok.Access == "" {
		return nil, ErrNotSignedIn
	}
	return &Provider{
		name:    "xai-sub",
		baseURL: "https://api.x.ai/v1",
		token:   tok,
		ctx:     ctx,
	}, nil
}

// ErrNotSignedIn is returned when no Grok OAuth token is stored.
var ErrNotSignedIn = errors.New("xai-sub: not signed in (run Settings → xAI Grok (SuperGrok) → Sign in)")

// Provider implements provider.Provider against api.x.ai with OAuth bearer auth.
type Provider struct {
	name    string
	baseURL string
	token   *Token
	ctx     context.Context
}

func (p *Provider) Name() string { return p.name }

// Token returns the active token (refreshing if expired).
func (p *Provider) Token() *Token { return p.token }

func (p *Provider) Chat(ctx context.Context, req provider.Request) (provider.Response, error) {
	wrapper, err := p.wrapper()
	if err != nil {
		return provider.Response{}, err
	}
	return wrapper.Chat(ctx, req)
}

func (p *Provider) Stream(ctx context.Context, req provider.Request, onChunk func(provider.Chunk)) (provider.Response, error) {
	wrapper, err := p.wrapper()
	if err != nil {
		return provider.Response{}, err
	}
	return wrapper.Stream(ctx, req, onChunk)
}

func (p *Provider) wrapper() (*provider.OpenAICompat, error) {
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
	return provider.NewOpenAICompat("xai-sub", tok.Access, p.baseURL), nil
}

// Register installs the xai-sub provider into the custom-provider table.
func Register() {
	provider.RegisterCustom("xai-sub", New)
}