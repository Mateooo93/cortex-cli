package ui

import (
	"image"

	uv "github.com/charmbracelet/ultraviolet"
)

// Layout holds the computed vertical and horizontal dimensions for the UI.
type Layout struct {
	Width           int
	ChatHeight      int
	InputHeight     int
	StatusBarHeight int
	TabBarHeight        int
	ActivityStripHeight int
	PanelHeight         int
	ChatWidth           int // same as Width; kept for call-site compatibility
}

// computeLayout calculates the vertical space allocation.
// activityStripRows is 0 or 1 for the compact tool-activity footer.
// panelHeights are optional heights for attachment panel, history panel, etc.
func computeLayout(width, height, inputLineCount, activityStripRows int, panelHeights ...int) Layout {
	// Status bar: 1 line (slim footer with connection ·
	// model · ctx% · ⏱). The status bar is ALWAYS 1
	// line tall, even when a transient message is
	// active — the message REPLACES the slim footer
	// (the connection / model readouts are still
	// visible in the right panel during the message).
	// The previous behaviour rendered 2 lines for the
	// status bar when a message was active, which
	// overlapped the bottom row of the chat viewport
	// and made the bottom of the conversation appear
	// to "disappear" — see the user-reported bug
	// "when i scroll up the bottom of the chat
	// starts disappearing".
	const statusBarHeight = 1
	const tabBarHeight = 3

	// Input area: textarea lines + 2 for top/bottom border +
	// 1 for the key-hint row below the prompt (Tab=queue,
	// Enter=send, Esc=cancel, Shift+Enter=newline, ↑/↓
	// history). The right panel no longer carries these
	// hints so the prompt area owns them.
	inputHeight := inputLineCount + 2 + 1
	if inputHeight < 4 {
		inputHeight = 4
	}

	// Sum panel heights (attachment, history, mode warning, etc.)
	panelHeight := 0
	for _, h := range panelHeights {
		panelHeight += h
	}

	if activityStripRows < 0 {
		activityStripRows = 0
	}
	if activityStripRows > 1 {
		activityStripRows = 1
	}

	chatHeight := height - inputHeight - statusBarHeight - tabBarHeight - panelHeight - activityStripRows
	if chatHeight < 3 {
		chatHeight = 3
	}

	return Layout{
		Width:               width,
		ChatHeight:          chatHeight,
		InputHeight:         inputHeight,
		StatusBarHeight:     statusBarHeight,
		TabBarHeight:        tabBarHeight,
		ActivityStripHeight: activityStripRows,
		PanelHeight:         panelHeight,
		ChatWidth:           width,
	}
}

// centerRect returns a rectangle centered within the given area.
func centerRect(area uv.Rectangle, width, height int) uv.Rectangle {
	cx := area.Min.X + area.Dx()/2
	cy := area.Min.Y + area.Dy()/2
	return image.Rect(cx-width/2, cy-height/2, cx-width/2+width, cy-height/2+height)
}
