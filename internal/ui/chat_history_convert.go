package ui

import "github.com/Mateooo93/cortex-cli/internal/provider"

// chatMessagesToProviderHistory converts the UI's chat message
// representation into the provider's message list. Used to seed
// a freshly-reconnected session with the prior scrollback so the
// new daemon has full conversation context, not just the user's
// next message in isolation.
//
// We drop any non-conversational messages (errors, system
// notifications, turn-info rows) and skip thinking blocks. Only
// user / assistant / tool turns are kept, in original order.
func chatMessagesToProviderHistory(messages []ChatMessage) []provider.Message {
	out := make([]provider.Message, 0, len(messages))
	for _, m := range messages {
		switch m.Type {
		case MsgUser:
			out = append(out, provider.Message{Role: "user", Content: m.Text})
		case MsgAssistant:
			out = append(out, provider.Message{Role: "assistant", Content: m.Text})
		case MsgToolResult:
			role := "tool"
			out = append(out, provider.Message{
				Role:       role,
				Content:    m.Text,
				ToolName:   m.ToolName,
				ToolCallID: "", // not preserved in the UI representation
			})
		case MsgToolCall:
			// Tool-call intent lives on the assistant turn's
			// ToolCalls field; we don't emit a separate
			// message for it here.
		default:
			// Drop MsgThinking, MsgError, MsgSystem, MsgPlan*,
			// MsgWorkflow*, etc. These are UI-only and would
			// confuse the model.
		}
	}
	return out
}
