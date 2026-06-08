package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// streamDisplayCache holds incrementally rendered assistant streaming output.
// Only new complete lines and the active tail are processed on each chunk.
type streamDisplayCache struct {
	stableTextLen int
	stableRendered string
}

func (c *streamDisplayCache) reset() {
	*c = streamDisplayCache{}
}

func streamLineStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(bodyColor))
}

func streamCursorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#58A6FF"))
}

// renderStreamLine renders one logical line for the streaming preview.
// Word wrap is skipped during streaming so each chunk appends in O(1).
func renderStreamLine(line string) string {
	return "  " + streamLineStyle().Render(line) + "\n"
}

// renderStreamTail renders the in-progress last line plus a live cursor.
func renderStreamTail(line string) string {
	if line == "" {
		return "  " + streamCursorStyle().Render("▌") + "\n"
	}
	return "  " + streamLineStyle().Render(line) + streamCursorStyle().Render("▌") + "\n"
}

// updateStreamingDisplay incrementally extends the streaming preview as
// assistantBuf grows. Full markdown formatting still runs on stream_done.
func updateStreamingDisplay(sess *SessionState) {
	buf := sess.assistantBuf
	if len(buf) < sess.streamCache.stableTextLen {
		sess.streamCache.reset()
	}

	pending := buf[sess.streamCache.stableTextLen:]
	for {
		idx := strings.IndexByte(pending, '\n')
		if idx < 0 {
			break
		}
		sess.streamCache.stableRendered += renderStreamLine(pending[:idx])
		pending = pending[idx+1:]
		sess.streamCache.stableTextLen = len(buf) - len(pending)
	}

	var tail string
	if pending != "" || buf == "" {
		tail = renderStreamTail(pending)
	}
	sess.assistantRendered = sess.streamCache.stableRendered + tail
}