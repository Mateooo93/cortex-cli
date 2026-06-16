package ui

import (
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestReleaseStreamPlayback_AdaptiveSizing(t *testing.T) {
	pending := "abcdefghijklmnopqrstuvwxyz"
	got := releaseStreamPlayback(&pending)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 chars released, got %q", got)
	}
	if pending == "" {
		t.Fatal("expected remainder in pending buffer")
	}

	pending = strings.Repeat("x", 500)
	got = releaseStreamPlayback(&pending)
	if len(got) < 24 {
		t.Fatalf("expected large release for backlog, got %d chars", len(got))
	}
}

func TestStreamPlaybackPreservesManualChatScroll(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.width = 80
	m.height = 20
	m.updateChatWidth()

	sess := m.currentSession()
	sess.agentState = StateStreaming
	sess.assistantBuf = strings.Repeat("old line\n", 40)
	updateStreamingDisplay(sess)

	prevMax := m.sessionMaxScrollOffset(sess)
	if prevMax <= 0 {
		t.Fatalf("expected scrollable transcript, max offset = %d", prevMax)
	}

	const manualOffset = 5
	sess.chatScrollOffset = manualOffset
	sess.streamPending = strings.Repeat("new line\n", 5)
	sess.streamPlayback.active = true

	_, _ = m.Update(streamPlaybackMsg{gen: sess.streamPlayback.gen, anim: &sess.streamPlayback})

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
		t.Fatal("stream playback forced chat to bottom while manually scrolled up")
	}
}
