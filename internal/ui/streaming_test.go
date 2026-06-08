package ui

import (
	"strings"
	"testing"

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