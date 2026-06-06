package ui

import (
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/provider"
)

func TestChatMessagesToProviderHistory_DropsUIOnlyMessages(t *testing.T) {
	in := []ChatMessage{
		{Type: MsgSystem, Text: "Welcome"},
		{Type: MsgUser, Text: "What is 2+2?"},
		{Type: MsgAssistant, Text: "It's 4."},
		{Type: MsgThinking, Text: "user is asking arithmetic"},
		{Type: MsgToolCall, ToolName: "read_file", Text: "foo.go"},
		{Type: MsgToolResult, ToolName: "read_file", Text: "package main"},
		{Type: MsgError, Text: "oops"},
		{Type: MsgUser, Text: "thanks!"},
	}
	got := chatMessagesToProviderHistory(in)
	want := []provider.Message{
		{Role: "user", Content: "What is 2+2?"},
		{Role: "assistant", Content: "It's 4."},
		{Role: "tool", Content: "package main", ToolName: "read_file"},
		{Role: "user", Content: "thanks!"},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d messages, got %d (%+v)", len(want), len(got), got)
	}
	for i, m := range got {
		if m.Role != want[i].Role || m.Content != want[i].Content || m.ToolName != want[i].ToolName {
			t.Errorf("[%d] got %+v, want %+v", i, m, want[i])
		}
	}
}

func TestChatMessagesToProviderHistory_Empty(t *testing.T) {
	got := chatMessagesToProviderHistory(nil)
	if len(got) != 0 {
		t.Errorf("expected 0 messages, got %d", len(got))
	}
	got = chatMessagesToProviderHistory([]ChatMessage{})
	if len(got) != 0 {
		t.Errorf("expected 0 messages, got %d", len(got))
	}
}
