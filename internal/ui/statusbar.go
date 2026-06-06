package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// renderStatusBar renders the two-line status bar.
//
// Line 1 — always visible: shortcut hints on the left, connection status on the right.
// Line 2 — either a transient message (warning / info / error) shown for 3 s, or the
//          always-visible keybind hint that tells the user how to send, queue, and
//          interrupt the agent. The hint is shown on the bottom-left so it never
//          gets squeezed out by the connection badge.
func renderStatusBar(
	width int,
	connected bool,
	reconnecting bool,
	msg StatusMessage,
	s Styles,
) string {
	// ── Line 1: shortcuts + connection status ───────────────────────────────
	badgeStyle := lipgloss.NewStyle().Background(colorSecondary).Foreground(lipgloss.Color("0")).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	f1Badge := badgeStyle.Render(" F1 ")
	f2Badge := badgeStyle.Render(" F2 ")
	f3Badge := badgeStyle.Render(" F3 ")
	tabBadge := badgeStyle.Render(" Tab ")
	enterBadge := badgeStyle.Render(" Enter ")
	ctrlTBadge := badgeStyle.Render(" Ctrl+T ")
	shortcuts := f1Badge + labelStyle.Render(" Sessions ") +
		f2Badge + labelStyle.Render(" Chat ") +
		f3Badge + labelStyle.Render(" Settings ") +
		enterBadge + labelStyle.Render(" send ") +
		tabBadge + labelStyle.Render(" queue ") +
		ctrlTBadge + labelStyle.Render(" new session")

	var connStatus string
	if connected {
		connStatus = statusConnectedStyle.Render("● Connected")
	} else if reconnecting {
		connStatus = statusReconnectingStyle.Render("● Reconnecting")
	} else {
		connStatus = statusDisconnectedStyle.Render("● Disconnected")
	}

	shortcutsLen := lipgloss.Width(shortcuts)
	connLen := lipgloss.Width(connStatus)
	totalContent := shortcutsLen + connLen
	remaining := width - totalContent - 2
	if remaining < 2 {
		remaining = 2
	}
	leftPad := remaining / 2
	rightPad := remaining - leftPad
	line1 := strings.Repeat(" ", leftPad) + shortcuts + strings.Repeat(" ", rightPad) + connStatus

	// ── Line 2: transient message OR always-visible keybind hint ────────────
	var line2 string
	if msg.Text != "" {
		var msgStyle lipgloss.Style
		var prefix string
		switch msg.Kind {
		case StatusMsgWarning:
			msgStyle = lipgloss.NewStyle().Foreground(colorWarning).Italic(true)
			prefix = " ⚠ "
		case StatusMsgError:
			msgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
			prefix = " ✖ "
		default: // StatusMsgInfo
			msgStyle = lipgloss.NewStyle().Foreground(s.ColorDimGray).Italic(true)
			prefix = " ℹ "
		}
		line2 = msgStyle.Render(prefix + msg.Text)
	} else {
		// No active status message — use the second line for the
		// always-visible keybind hint. This is the "bottom-left
		// footer telling the user what keybind to press to queue
		// or to send it the next turn" the user asked for.
		line2 = renderKeybindHint(width, s)
	}
	// Always pad line 2 to full width so the layout never shifts.
	line2 = lipgloss.NewStyle().Width(width).Render(line2)

	return line2 + "\n" + s.StatusBarStyle.Width(width).Render(line1)
}

// renderKeybindHint returns the always-visible keybind hint shown on
// the status bar's second line. It explains how to send, queue, and
// interrupt the agent in plain language so the user does not have to
// dig through the docs to discover the Tab-vs-Enter distinction.
//
// The hint is left-aligned (it sits in the bottom-left of the screen
// as the user requested) and uses the dim color so it does not
// compete with the connection status badge on line 1.
func renderKeybindHint(width int, s Styles) string {
	badgeStyle := lipgloss.NewStyle().Background(colorSecondary).Foreground(lipgloss.Color("0")).Bold(true)
	dimLabel := lipgloss.NewStyle().Foreground(s.ColorDimGray)
	enterBadge := badgeStyle.Render(" Enter ")
	tabBadge := badgeStyle.Render(" Tab ")
	escBadge := badgeStyle.Render(" Esc ")

	parts := []string{
		enterBadge + dimLabel.Render(" send (interrupts after current edit)  "),
		tabBadge + dimLabel.Render(" queue (run after this turn)  "),
		escBadge + dimLabel.Render(" cancel now"),
	}
	hint := strings.Join(parts, "")
	hintVisual := lipgloss.Width(hint)
	// Pad to full width so the line stays right-anchored to the
	// edge of the screen (looks tidier than a left-anchored short
	// string with the rest of the line blank).
	if hintVisual < width {
		hint = hint + strings.Repeat(" ", width-hintVisual)
	}
	return hint
}

func formatTokenCount(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%dk", n/1000)
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
