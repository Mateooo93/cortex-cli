package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	llmprovider "github.com/Mateooo93/cortex-cli/internal/provider"
)

// sessionTitleGeneratedMsg is the result of an AI session-naming request.
// sessionID matches SessionState.persistID. err is non-nil only when the AI
// call itself failed; the cmd always returns a usable title in either case
// (the fallback is derived from the first user message locally).
type sessionTitleGeneratedMsg struct {
	sessionID string
	title     string
	err       error
}

// generateSessionTitleCmd asks the default-model provider to produce a short
// (3-6 word) title summarizing firstUserMessage. If the AI call fails, the
// returned title is derived locally from the first words of the user message
// so the session still gets a meaningful label.
func generateSessionTitleCmd(sessionID, firstUserMessage string) tea.Cmd {
	return func() tea.Msg {
		title := deriveTitleFromMessage(firstUserMessage)
		if sessionID == "" || strings.TrimSpace(firstUserMessage) == "" {
			return sessionTitleGeneratedMsg{sessionID: sessionID, title: title}
		}

		cfg, err := cortexconfig.Load()
		if err != nil || cfg == nil || cfg.DefaultModel == "" {
			return sessionTitleGeneratedMsg{sessionID: sessionID, title: title}
		}

		mc, ok := cfg.Models[cfg.DefaultModel]
		if !ok {
			return sessionTitleGeneratedMsg{sessionID: sessionID, title: title}
		}

		prov, perr := llmprovider.New(llmprovider.ModelConfig{
			Provider: mc.Provider,
			Model:    mc.Model,
			BaseURL:  mc.BaseURL,
			APIKey:   mc.APIKey,
		})
		if perr != nil {
			return sessionTitleGeneratedMsg{sessionID: sessionID, title: title, err: perr}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()

		systemPrompt := "You are a session-naming assistant. Given a single user message, reply with a concise title of 3-6 words that summarises the request. " +
			"Reply with ONLY the title. No quotes, no leading words like \"Title:\", no trailing punctuation, no preamble, no explanation."

		resp, cerr := prov.Chat(ctx, llmprovider.Request{
			Model: mc.Model,
			Messages: []llmprovider.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: firstUserMessage},
			},
			MaxTokens:   40,
			Temperature: 0.2,
		})
		if cerr != nil {
			return sessionTitleGeneratedMsg{sessionID: sessionID, title: title, err: cerr}
		}

		aiTitle := cleanAITitle(resp.Content)
		if aiTitle == "" {
			return sessionTitleGeneratedMsg{sessionID: sessionID, title: title, err: errors.New("empty ai title")}
		}
		return sessionTitleGeneratedMsg{sessionID: sessionID, title: aiTitle}
	}
}

// cleanAITitle normalises the raw model output: strips wrapping quotes,
// leading "Title:"-style labels, bullet markers, and trailing punctuation;
// collapses whitespace; and caps length to 60 runes.
func cleanAITitle(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Strip wrapping quotes of any flavour.
	for _, q := range []string{`"`, "'", "`", "“", "”", "‘", "’"} {
		if strings.HasPrefix(s, q) && strings.HasSuffix(s, q) && len(s) >= 2 {
			s = s[1 : len(s)-1]
			s = strings.TrimSpace(s)
		}
	}
	// Drop a leading "Title:" / "Subject:" etc.
	for _, prefix := range []string{"Title:", "Subject:", "title:", "subject:"} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimSpace(strings.TrimPrefix(s, prefix))
		}
	}
	// Drop a leading list marker like "- " or "* ".
	for _, marker := range []string{"- ", "* ", "• "} {
		if strings.HasPrefix(s, marker) {
			s = strings.TrimSpace(strings.TrimPrefix(s, marker))
		}
	}
	// Drop trailing punctuation (one or more).
	s = strings.TrimRight(s, ".,;:!?\"'")
	// Collapse internal whitespace.
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) > 60 {
		s = string(runes[:60])
		s = strings.TrimRight(s, " .,;:!?")
	}
	return s
}

// deriveTitleFromMessage produces a short fallback title from a user message
// when the AI call is unavailable. It takes the first 4-6 meaningful words,
// skips leading commands ("/", "@", etc.), and strips common noise.
func deriveTitleFromMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	// Drop a leading slash command and any args.
	if strings.HasPrefix(msg, "/") {
		return ""
	}
	// Collapse whitespace.
	msg = strings.Join(strings.Fields(msg), " ")
	runes := []rune(msg)
	if len(runes) > 48 {
		msg = string(runes[:48])
	}
	return strings.TrimRight(msg, ".,;:!?")
}
