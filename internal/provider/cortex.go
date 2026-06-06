package provider

import (
	"context"
	"regexp"
	"strings"
)

// CortexProvider wraps OpenAICompat and applies Cortex-specific post-processing:
//   - Strips <cortex>...</cortex> and <cortex/> tags from streamed chunks
//   - Strips [RETRIEVED_SNIPPETS]...[/RETRIEVED_SNIPPETS] blocks
//   - Avoids sending a system prompt (Cortex injects its own)
//   - Forwards cortex-specific knobs (reasoning_effort, cortex_prompt_mode)
type CortexProvider struct {
	*OpenAICompat
}

// cortexTagRE matches <cortex>...</cortex>, <cortex .../>, and <cortex/>.
var cortexTagRE = regexp.MustCompile(`(?s)<cortex[^>]*>.*?</cortex>|<cortex\s*/?>`)
var retrievedRE = regexp.MustCompile(`(?s)\[RETRIEVED_SNIPPETS\].*?\[/RETRIEVED_SNIPPETS\]`)

func NewCortexProvider(apiKey, baseURL string) *CortexProvider {
	return &CortexProvider{
		OpenAICompat: NewOpenAICompat("cortex", apiKey, baseURL),
	}
}

func (p *CortexProvider) Name() string { return "cortex" }

func (p *CortexProvider) stripTags(s string) string {
	s = cortexTagRE.ReplaceAllString(s, "")
	s = retrievedRE.ReplaceAllString(s, "")
	return s
}

// Chat implements Provider. Cortex's server injects its own system prompt,
// so we never send one.
func (p *CortexProvider) Chat(ctx context.Context, req Request) (Response, error) {
	req.Messages = filterSystemPrompts(req.Messages)
	resp, err := p.OpenAICompat.Chat(ctx, req)
	if err != nil {
		return resp, err
	}
	resp.Content = p.stripTags(resp.Content)
	return resp, nil
}

// Stream implements Provider. Each chunk is filtered through stripTags before
// being forwarded, and the final accumulation is filtered again.
func (p *CortexProvider) Stream(ctx context.Context, req Request, onChunk func(Chunk)) (Response, error) {
	req.Messages = filterSystemPrompts(req.Messages)
	wrapped := func(c Chunk) {
		if c.Content != "" {
			c.Content = p.stripTags(c.Content)
		}
		onChunk(c)
	}
	resp, err := p.OpenAICompat.Stream(ctx, req, wrapped)
	if err != nil {
		return resp, err
	}
	resp.Content = p.stripTags(resp.Content)
	return resp, nil
}

// filterSystemPrompts removes any system messages — Cortex's server injects
// its own system prompt and sending one would conflict.
func filterSystemPrompts(msgs []Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		out = append(out, m)
	}
	return out
}

// Helper to ensure strings.Join is referenced (keeps imports clean for future use).
var _ = strings.Join
