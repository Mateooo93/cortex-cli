package ui

import (
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

func TestUpdateStreamingDisplay_IncrementalAppend(t *testing.T) {
	sess := &SessionState{}
	sess.assistantBuf = "hel"
	updateStreamingDisplay(sess)
	first := sess.assistantRendered

	sess.assistantBuf = "hello"
	updateStreamingDisplay(sess)
	if sess.assistantRendered == "" {
		t.Fatal("expected rendered output")
	}
	if !strings.Contains(stripANSI(sess.assistantRendered), "hello") {
		t.Fatalf("expected hello in output, got %q", sess.assistantRendered)
	}
	if !strings.ContainsRune(stripANSI(sess.assistantRendered), '▌') {
		t.Fatalf("expected streaming cursor, got %q", sess.assistantRendered)
	}

	sess.assistantBuf = "hello\nworld"
	updateStreamingDisplay(sess)
	if !strings.Contains(stripANSI(sess.assistantRendered), "world") {
		t.Fatalf("expected world after newline, got %q", sess.assistantRendered)
	}
	_ = first
}

func TestStreamChunkBuffersForPlayback(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.width = 80
	m.height = 20
	m.updateChatWidth()

	sess := m.currentSession()
	sess.agentState = StateStreaming

	cmds := m.applyEventToSession(0, protocol.SessionEvent{
		Type: "event.stream_chunk",
		Data: protocol.EventStreamChunk{Text: "hello"},
	})

	if sess.streamPending != "hello" {
		t.Fatalf("streamPending = %q, want hello", sess.streamPending)
	}
	if sess.assistantBuf != "" {
		t.Fatalf("assistantBuf should stay empty until playback tick, got %q", sess.assistantBuf)
	}
	if len(cmds) == 0 {
		t.Fatal("expected playback tick cmd")
	}

	sess.streamPlayback.active = true
	_, _ = m.Update(streamPlaybackMsg{gen: sess.streamPlayback.gen, anim: &sess.streamPlayback})
	if sess.assistantBuf == "" {
		t.Fatal("expected playback tick to release text into assistantBuf")
	}
	if !strings.Contains(stripANSI(sess.assistantRendered), "he") {
		t.Fatalf("expected streaming preview after tick, got %q", sess.assistantRendered)
	}
}

func TestCoalesceStreamChunkEvents(t *testing.T) {
	ch := make(chan protocol.SessionEvent, 4)
	ch <- protocol.SessionEvent{Type: "event.stream_chunk", Data: protocol.EventStreamChunk{Text: "lo"}}
	ch <- protocol.SessionEvent{Type: "event.stream_chunk", Data: protocol.EventStreamChunk{Text: "!"}}
	ch <- protocol.SessionEvent{Type: "event.stream_done", Data: protocol.EventStreamDone{}}

	first := protocol.SessionEvent{Type: "event.stream_chunk", Data: protocol.EventStreamChunk{Text: "hel"}}
	var held *protocol.SessionEvent
	merged := coalesceStreamChunkEvents(ch, first, &held)
	if streamChunkText(merged) != "hello!" {
		t.Fatalf("merged text = %q, want hello!", streamChunkText(merged))
	}
	if held == nil || held.Type != "event.stream_done" {
		t.Fatalf("expected held stream_done event, got %#v", held)
	}
}

func TestRenderStreamTail_ShowsCursor(t *testing.T) {
	got := renderStreamTail("abc")
	if !strings.ContainsRune(stripANSI(got), '▌') {
		t.Fatalf("expected cursor in tail, got %q", got)
	}
	if !strings.Contains(stripANSI(got), "abc") {
		t.Fatalf("expected abc in tail, got %q", got)
	}
}

func TestStreamDonePreservesManualChatScroll(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.width = 80
	m.height = 20
	m.updateChatWidth()

	sess := m.currentSession()
	sess.agentState = StateStreaming
	sess.assistantBuf = strings.Repeat("- old line\n", 40)
	updateStreamingDisplay(sess)

	prevMax := m.sessionMaxScrollOffset(sess)
	if prevMax <= 0 {
		t.Fatalf("expected scrollable transcript, max offset = %d", prevMax)
	}

	const manualOffset = 5
	sess.chatScrollOffset = manualOffset
	sess.streamPending = strings.Repeat("- new line\n", 5)
	sess.streamPlayback.active = true

	m.applyEventToSession(0, protocol.SessionEvent{Type: "event.stream_done", Data: protocol.EventStreamDone{}})
	for sess.streamPending != "" {
		_, _ = m.Update(streamPlaybackMsg{gen: sess.streamPlayback.gen, anim: &sess.streamPlayback})
	}

	nextMax := m.sessionMaxScrollOffset(sess)
	want := manualOffset + nextMax - prevMax
	if want < 0 {
		want = 0
	}
	if want > nextMax {
		want = nextMax
	}
	if sess.chatScrollOffset != want {
		t.Fatalf("chatScrollOffset = %d, want %d", sess.chatScrollOffset, want)
	}
	if sess.chatScrollOffset == 0 {
		t.Fatal("stream_done forced chat to bottom while manually scrolled up")
	}
}

func TestAgentDonePreservesManualChatScroll(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.width = 80
	m.height = 20
	m.updateChatWidth()

	sess := m.currentSession()
	sess.agentState = StateStreaming
	sess.assistantBuf = strings.Repeat("- old line\n", 40)
	updateStreamingDisplay(sess)

	prevMax := m.sessionMaxScrollOffset(sess)
	if prevMax <= 0 {
		t.Fatalf("expected scrollable transcript, max offset = %d", prevMax)
	}

	const manualOffset = 5
	sess.chatScrollOffset = manualOffset

	m.applyEventToSession(0, protocol.SessionEvent{Type: "event.agent_done"})

	nextMax := m.sessionMaxScrollOffset(sess)
	want := manualOffset + nextMax - prevMax
	if want < 0 {
		want = 0
	}
	if want > nextMax {
		want = nextMax
	}
	if sess.chatScrollOffset != want {
		t.Fatalf("chatScrollOffset = %d, want %d", sess.chatScrollOffset, want)
	}
	if sess.chatScrollOffset == 0 {
		t.Fatal("agent_done forced chat to bottom while manually scrolled up")
	}
}
