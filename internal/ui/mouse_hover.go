package ui

import (
	"charm.land/lipgloss/v2"
)

const hoverTab = 1

type mouseHover struct {
	kind  int
	index int // tab kind when kind == hoverTab
}

func mouseHoverStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(lipgloss.Color("#2A3545"))
}

// updateMouseHover tracks hover for tab-bar labels and input clipboard buttons.
func (m *Model) updateMouseHover(x, y int) {
	m.mouseHover = mouseHover{}
	if m.mouseInTabBar(y) {
		if kind, ok := tabKindAtX(x); ok {
			m.mouseHover = mouseHover{kind: hoverTab, index: int(kind)}
		}
	}
	m.updateInputBtnHover(x, y)
	m.syncChatInputPrompt()
}

func (m Model) hoverTabKind() (TabKind, bool) {
	if m.mouseHover.kind != hoverTab {
		return 0, false
	}
	return TabKind(m.mouseHover.index), true
}