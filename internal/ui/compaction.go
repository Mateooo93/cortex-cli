package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	llmprovider "github.com/Mateooo93/cortex-cli/internal/provider"
)

// compactMsg is fired by compactCmd when the compaction finishes.
// `ok=true` means the LLM returned a summary and we successfully
// replaced the history. `ok=false` means we degraded gracefully
// (the LLM call failed so we kept the last N messages and dropped
// older ones).
type compactMsg struct {
	ok        bool
	oldCount  int    // number of messages before compaction
	oldTokens int64  // approximate input token count before
	newCount  int    // number of messages after compaction
	newTokens int64  // approximate input token count after
	err       error  // only set when ok=false
}

// compactCmd asks the current model to summarize the conversation
// history, then replaces the history with the summary + the last
// few messages. The whole thing runs in a goroutine so the TUI
// stays responsive.
//
// Compaction strategy:
//
//  1. Build a transcript of the current chat history (just the
//     user + assistant text, no UI noise).
//
//  2. If the transcript fits in < 4 messages, there's nothing
//     meaningful to compact — emit a no-op status and return.
//
//  3. Send the transcript to the LLM with a fixed prompt
//     ("Summarize the conversation so far in 5-10 bullet points,
//     preserving any decisions, file paths, error messages, and
//     tool names the user is likely to need later. Keep the
//     summary under 1500 words.").
//
//  4. If the LLM call fails, fall back to dropping the oldest
//     half of the messages (a poor-man's compaction that's at
//     least better than nothing — the user keeps the most recent
//     turns intact).
//
//  5. Replace sess.chatMessages with [systemMessage(summary), …last 4
//     messages]. The summary message is rendered with the same
//     "system" style the rest of the system messages use.
//
//  6. Push the new history to the daemon (SendRestoreHistory) so
//     the next turn sees the compacted context.
//
//  7. Emit a compactMsg so the status bar can show
//     "compacted 142k → 4k tokens (97% reduction)".
func (m *Model) compactCmd() tea.Cmd {
	return tea.Batch(
		compactProgressTick(), // start the 120ms spinner tick
		func() tea.Msg {
			sess := m.currentSession()
			if sess == nil {
				m.compactInFlight = false
				return compactMsg{ok: false, err: fmt.Errorf("no active session")}
			}
			oldCount := len(sess.chatMessages)
			oldTokens := sess.inputTokens
			if oldTokens == 0 {
				// Fall back to chars/4 estimate so the
				// before/after numbers are non-zero.
				chars := 0
				for _, msg := range sess.chatMessages {
					chars += len(msg.Text)
				}
				oldTokens = int64(chars / 4)
			}

			// Build a transcript of user + assistant messages
			// only. Tool calls, system messages, and confirm
			// prompts are noise from the LLM's perspective.
			type tx struct{ role, text string }
			var transcript []tx
			for _, msg := range sess.chatMessages {
				switch msg.Type {
				case MsgUser:
					transcript = append(transcript, tx{"user", msg.Text})
				case MsgAssistant:
					if msg.Text != "" {
						transcript = append(transcript, tx{"assistant", msg.Text})
					}
				}
			}
			if len(transcript) < 4 {
				m.compactInFlight = false
				return compactMsg{
					ok:        false,
					oldCount:  oldCount,
					oldTokens: oldTokens,
					newCount:  oldCount,
					newTokens: oldTokens,
					err:       fmt.Errorf("nothing to compact (only %d messages)", oldCount),
				}
			}

			// Build the prompt the LLM will summarize.
			var b strings.Builder
			b.WriteString("Summarize the following conversation in 5-10 bullet points.\n")
			b.WriteString("Preserve any decisions, file paths, error messages, function names,\n")
			b.WriteString("and tool calls the user is likely to need in later turns.\n")
			b.WriteString("Keep the summary under 1500 words. Use markdown.\n\n")
			for _, t := range transcript {
				fmt.Fprintf(&b, "**%s:** %s\n\n", t.role, t.text)
			}

			summary, ok := m.callLLMForSummary(b.String(), sess)
			if !ok {
				// LLM call failed — fall back to dropping the
				// oldest half of the messages so the user
				// gets at least a partial reset.
				half := len(sess.chatMessages) / 2
				if half < 1 {
					half = 1
				}
				dropped := sess.chatMessages[:half]
				// Convert the dropped messages into a single
				// "compaction" message so the LLM has some
				// context about what was lost.
				var droppedText strings.Builder
				droppedText.WriteString("**[compaction: dropped ")
				droppedText.WriteString(fmt.Sprintf("%d", half))
				droppedText.WriteString(" older messages — LLM summarisation failed]**\n\n")
				for _, msg := range dropped {
					if msg.Type == MsgUser || msg.Type == MsgAssistant {
						fmt.Fprintf(&droppedText, "- %s\n", msg.Text)
					}
				}
				sess.chatMessages = append(
					[]ChatMessage{renderSystemSuccessMessage(droppedText.String())},
					sess.chatMessages[half:]...,
				)
				// Push the new history to the daemon.
				if sess.client != nil {
					_ = sess.client.SendRestoreHistory(chatMessagesToProviderHistory(sess.chatMessages))
				}
				newChars := 0
				for _, msg := range sess.chatMessages {
					newChars += len(msg.Text)
				}
				m.compactInFlight = false
				return compactMsg{
					ok:        false,
					oldCount:  oldCount,
					oldTokens: oldTokens,
					newCount:  len(sess.chatMessages),
					newTokens: int64(newChars / 4),
					err:       fmt.Errorf("LLM summarisation failed; kept last %d messages", len(sess.chatMessages)-1),
				}
			}

			// Success path: build the new message list = [system
			// message with the summary, …last 4 messages]. We
			// keep the last 4 verbatim so the LLM doesn't lose
			// immediate context (the user's most recent question,
			// the tool calls it just made, etc.).
			const keepLast = 4
			startKeep := len(sess.chatMessages) - keepLast
			if startKeep < 0 {
				startKeep = 0
			}
			newMessages := []ChatMessage{renderSystemSuccessMessage("**Conversation summary (auto-compacted):**\n\n" + summary)}
			newMessages = append(newMessages, sess.chatMessages[startKeep:]...)
			sess.chatMessages = newMessages

			// Reset the per-turn token counters so the next
			// turn starts at 0 and the user sees the context
			// usage drop in the status bar.
			sess.inputTokens = 0
			sess.outputTokens = 0
			sess.cacheReadTokens = 0
			sess.cacheCreationTokens = 0
			sess.turnStartInputTokens = 0
			sess.turnStartOutputTokens = 0

			// Push the new history to the daemon.
			if sess.client != nil {
				_ = sess.client.SendRestoreHistory(chatMessagesToProviderHistory(sess.chatMessages))
			}
			newChars := len(summary) + 200 // ballpark for the 4 kept messages
			m.compactInFlight = false
			return compactMsg{
				ok:        true,
				oldCount:  oldCount,
				oldTokens: oldTokens,
				newCount:  len(sess.chatMessages),
				newTokens: int64(newChars / 4),
			}
		},
	)
}

// callLLMForSummary runs a one-shot completion against the
// active model with the summary prompt. It bypasses the
// daemon's full session machinery (we just want a one-time
// text completion) and reuses the provider's HTTP client.
//
// Returns the summary text and true on success, or "" and
// false on any error. Errors are swallowed here; the caller
// decides whether to fall back to a degraded compaction or
// surface a status message.
func (m *Model) callLLMForSummary(prompt string, sess *SessionState) (string, bool) {
	if m.cortexCfg == nil || sess == nil {
		return "", false
	}
	// Look up the active spec; if none, use the configured
	// default. (Same resolution the rest of the code uses.)
	spec := m.currentSettingsModel()
	if spec == "" {
		spec = m.cortexCfg.DefaultModel
	}
	if spec == "" {
		return "", false
	}
	// Resolve the provider name and its API key / base URL
	// from the cortexconfig so we can build a one-shot
	// provider instance.
	colon := strings.Index(spec, ":")
	providerName := spec
	if colon > 0 {
		providerName = spec[:colon]
	}
	// API key: look in the config (for env-var /
	// config-file providers) and the keyring (for
	// interactive providers).
	apiKey := ""
	if mc, ok := m.cortexCfg.Models[spec]; ok && mc.APIKey != "" {
		apiKey = mc.APIKey
	}
	if apiKey == "" {
		// Fall back to the keyring.
		apiKey, _ = resolveProviderKey(providerName, m.cortexCfg)
	}
	if apiKey == "" {
		return "", false
	}
	// Base URL: the config's model override, falling back to
	// the provider's default.
	baseURL := ""
	if mc, ok := m.cortexCfg.Models[spec]; ok && mc.BaseURL != "" {
		baseURL = mc.BaseURL
	}
	llm, err := llmprovider.New(llmprovider.ModelConfig{
		Provider: providerName,
		Model:    spec,
		BaseURL:  baseURL,
		APIKey:   apiKey,
	})
	if err != nil || llm == nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := llm.Chat(ctx, llmprovider.Request{
		Model: spec,
		Messages: []llmprovider.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 2000,
	})
	if err != nil {
		return "", false
	}
	if strings.TrimSpace(resp.Content) == "" {
		return "", false
	}
	return resp.Content, true
}

// resolveProviderKey is a thin wrapper around
// config.ResolveProviderKey. We allow OAuth here because
// compaction runs against whatever model is active, which
// might be a subscription provider.
func resolveProviderKey(name string, cfg *cortexconfig.Config) (string, error) {
	key, _ := config.ResolveProviderKey(name, true)
	return key, nil
}

// handleCompactMsg surfaces the result of a /compact run in the
// status bar. Success shows the token delta; failure shows
// either a no-op message ("nothing to compact") or the
// degraded-mode warning ("LLM summarisation failed; kept last
// N messages").
func (m *Model) handleCompactMsg(msg compactMsg) tea.Cmd {
	if !msg.ok {
		if msg.err != nil {
			// No-op when there's nothing to compact is a
			// status-line message, not an error.
			if strings.Contains(msg.err.Error(), "nothing to compact") {
				return m.emitStatusMsg(msg.err.Error(), StatusMsgInfo)
			}
			return m.emitStatusMsg(msg.err.Error(), StatusMsgWarning)
		}
		return m.emitStatusMsg("Compaction failed", StatusMsgError)
	}
	delta := msg.oldTokens - msg.newTokens
	pct := 0.0
	if msg.oldTokens > 0 {
		pct = float64(delta) / float64(msg.oldTokens) * 100
	}
	return m.emitStatusMsg(fmt.Sprintf(
		"✓ done compacting — %s → %s tokens (%.0f%% reduction)",
		formatTokenCount(msg.oldTokens),
		formatTokenCount(msg.newTokens),
		pct,
	), StatusMsgInfo)
}

// maybeAutoCompact checks whether the current context usage
// exceeds 80% of the model's window and, if so, fires a
// /compact run in the background. Returns the tea.Cmd to run
// (or nil if no compaction is needed). The threshold matches
// the warning colour the right panel uses for the context
// bar.
//
// Called from the done-handler for every assistant turn, so
// the next turn always starts with a clean context when the
// previous one ran the user close to the limit.
func (m *Model) maybeAutoCompact() tea.Cmd {
	if !m.configuredAutoCompact() {
		return nil
	}
	sess := m.currentSession()
	if sess == nil {
		return nil
	}
	spec := m.currentSettingsModel()
	if spec == "" {
		spec = m.cortexCfg.DefaultModel
	}
	max := cortexconfig.ModelContextWindow(spec)
	if max == 0 {
		return nil
	}
	used := sess.inputTokens + sess.cacheReadTokens
	pct := float64(used) / float64(max) * 100
	if pct < 80 {
		return nil
	}
	// Already compacting? Don't fire another one.
	if m.compactInFlight {
		return nil
	}
	m.compactInFlight = true
	return tea.Batch(
		m.emitStatusMsg(fmt.Sprintf("⚠ context at %.0f%% — auto-compacting…", pct), StatusMsgWarning),
		func() tea.Msg {
			cmd := m.compactCmd()
			msg := cmd()
			// Clear the in-flight flag when we return
			// (the orchestrator runs this cmd, then we
			// reset on the next tick).
			m.compactInFlight = false
			return msg
		},
	)
}
