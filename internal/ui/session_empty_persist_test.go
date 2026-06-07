package ui

import "testing"

func TestSessionHasPersistableContent_EmptyNewSessionFalse(t *testing.T) {
	sess := &SessionState{
		label:        "",
		modelName:    "open/minimax-m3-free",
		persistID:    "pending-abc",
		showThinking: false,
	}
	if sessionHasPersistableContent(sess) {
		t.Fatalf("empty new session should not be persistable")
	}
}

func TestSessionHasPersistableContent_UserMessageTrue(t *testing.T) {
	sess := &SessionState{chatMessages: []ChatMessage{{Type: MsgUser, Text: "hello"}}}
	if !sessionHasPersistableContent(sess) {
		t.Fatalf("session with user message should be persistable")
	}
}

func TestSessionHasPersistableContent_ThinkingOnlyFalse(t *testing.T) {
	sess := &SessionState{chatMessages: []ChatMessage{{Type: MsgThinking, Text: "internal reasoning"}}}
	if sessionHasPersistableContent(sess) {
		t.Fatalf("thinking-only session should not be persistable because thinking is hidden by default")
	}
}

func TestSessionHasPersistableContent_AssistantBufferTrue(t *testing.T) {
	sess := &SessionState{assistantBuf: "partial response"}
	if !sessionHasPersistableContent(sess) {
		t.Fatalf("in-flight assistant response should be persistable")
	}
}
