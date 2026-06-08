package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

const streamPlaybackFPS = 60

// streamPlaybackMsg releases the next slice of buffered stream text.
type streamPlaybackMsg struct {
	gen  int
	anim *StreamPlayback
}

// StreamPlayback types out large SSE chunks smoothly instead of dumping them
// in one frame and then waiting for the next network packet.
type StreamPlayback struct {
	active bool
	gen    int
}

func NewStreamPlayback() StreamPlayback {
	return StreamPlayback{}
}

func (p *StreamPlayback) EnsureTick() tea.Cmd {
	if p.active {
		return nil
	}
	p.active = true
	return p.tick()
}

func (p *StreamPlayback) Stop() {
	p.active = false
	p.gen++
}

func (p *StreamPlayback) Advance(msg streamPlaybackMsg) tea.Cmd {
	if msg.anim != p || !p.active || msg.gen != p.gen {
		return nil
	}
	return p.tick()
}

func (p *StreamPlayback) tick() tea.Cmd {
	gen := p.gen
	anim := p
	return tea.Tick(time.Second/streamPlaybackFPS, func(time.Time) tea.Msg {
		return streamPlaybackMsg{gen: gen, anim: anim}
	})
}

func streamPlaybackChunkSize(pendingLen int) int {
	switch {
	case pendingLen > 800:
		return 48
	case pendingLen > 200:
		return 24
	case pendingLen > 80:
		return 12
	case pendingLen > 20:
		return 6
	default:
		return 2
	}
}

func releaseStreamPlayback(pending *string) string {
	if pending == nil || *pending == "" {
		return ""
	}
	n := streamPlaybackChunkSize(len(*pending))
	if n > len(*pending) {
		n = len(*pending)
	}
	out := (*pending)[:n]
	*pending = (*pending)[n:]
	return out
}

func flushStreamPlayback(sess *SessionState) {
	if sess == nil || sess.streamPending == "" {
		return
	}
	sess.assistantBuf += sess.streamPending
	sess.streamPending = ""
	updateStreamingDisplay(sess)
}