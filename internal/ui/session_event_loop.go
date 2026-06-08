package ui

import (
	"encoding/json"
	"strings"

	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

func normalizeSessionEvent(ev protocol.SessionEvent) protocol.SessionEvent {
	if ev.Type != "" && !strings.HasPrefix(ev.Type, "event.") {
		ev.Type = "event." + ev.Type
	}
	return ev
}

func isStreamChunkEvent(ev protocol.SessionEvent) bool {
	return ev.Type == "event.stream_chunk"
}

func streamChunkText(ev protocol.SessionEvent) string {
	data, err := json.Marshal(ev.Data)
	if err != nil {
		return ""
	}
	var chunk protocol.EventStreamChunk
	if json.Unmarshal(data, &chunk) != nil {
		return ""
	}
	return chunk.Text
}

// coalesceStreamChunkEvents drains any additional stream_chunk events already
// queued so the UI can paint a backlog in one frame instead of dropping deltas
// when it falls behind.
func coalesceStreamChunkEvents(ch <-chan protocol.SessionEvent, first protocol.SessionEvent, held **protocol.SessionEvent) protocol.SessionEvent {
	text := streamChunkText(first)
drain:
	for {
		select {
		case next, ok := <-ch:
			if !ok {
				break drain
			}
			next = normalizeSessionEvent(next)
			if isStreamChunkEvent(next) {
				text += streamChunkText(next)
				continue
			}
			*held = &next
			break drain
		default:
			break drain
		}
	}
	if text == streamChunkText(first) {
		return first
	}
	return protocol.SessionEvent{
		Type: "event.stream_chunk",
		Data: protocol.EventStreamChunk{Text: text},
	}
}