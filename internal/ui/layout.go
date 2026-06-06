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
	TabBarHeight    int
	PanelHeight     int
	ChatWidth       int // same as Width; kept for call-site compatibility
}

// computeLayout calculates the vertical space allocation.
// panelHeights are optional heights for attachment panel, history panel, etc.
func computeLayout(width, height, inputLineCount int, panelHeights ...int) Layout {
	// Status bar: 1 line (slim footer with connection · model ·
	// ctx% · ⏱ · F1/F2/F3). The status bar grows to 2 lines
	// when a transient message is active, but the layout
	// doesn't reserve space for it; the status bar simply
	// overlaps the bottom row of the chat viewport in that
	// case (which is fine — the user can see the message and
	// the message is short-lived).
	const statusBarHeight = 1
	const tabBarHeight = 3

	// Input area: textarea lines + 2 for top/bottom border
	inputHeight := inputLineCount + 2
	if inputHeight < 3 {
		inputHeight = 3
	}

	// Sum panel heights (attachment, history, mode warning, etc.)
	panelHeight := 0
	for _, h := range panelHeights {
		panelHeight += h
	}

	chatHeight := height - inputHeight - statusBarHeight - tabBarHeight - panelHeight
	if chatHeight < 3 {
		chatHeight = 3
	}

	return Layout{
		Width:           width,
		ChatHeight:      chatHeight,
		InputHeight:     inputHeight,
		StatusBarHeight: statusBarHeight,
		TabBarHeight:    tabBarHeight,
		PanelHeight:     panelHeight,
		ChatWidth:       width,
	}
}

// centerRect returns a rectangle centered within the given area.
func centerRect(area uv.Rectangle, width, height int) uv.Rectangle {
	cx := area.Min.X + area.Dx()/2
	cy := area.Min.Y + area.Dy()/2
	return image.Rect(cx-width/2, cy-height/2, cx-width/2+width, cy-height/2+height)
}
