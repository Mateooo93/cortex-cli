package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

// newInput creates a configured text area component.
func newInput() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Ask the agent anything..."
	ta.Focus()
	ta.CharLimit = 0 // no limit
	ta.ShowLineNumbers = false
	ta.SetHeight(1) // Start with 1 line
	ta.MaxHeight = 10 // Maximum 10 lines before scrolling

	// Show prompt arrow only on the first line, blank indent on continuation lines
	ta.SetPromptFunc(2, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return "❯ "
		}
		return "  "
	})

	// Configure keybindings - Shift+Enter inserts newlines, Enter submits
	// ctrl+j is what iTerm2 sends for shift+enter; alt+enter is a universal fallback
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter", "alt+enter", "ctrl+j"),
		key.WithHelp("shift+enter", "new line"),
	)

	// Clear all background styles so textarea matches terminal background
	noStyle := lipgloss.NewStyle()
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	s := ta.Styles()
	s.Focused.Base = noStyle
	s.Focused.CursorLine = textStyle
	s.Focused.Placeholder = dimStyle
	s.Focused.Text = textStyle
	s.Focused.EndOfBuffer = noStyle
	s.Focused.Prompt = noStyle

	s.Blurred.Base = noStyle
	s.Blurred.CursorLine = noStyle
	s.Blurred.Placeholder = dimStyle
	s.Blurred.Text = dimStyle
	s.Blurred.EndOfBuffer = noStyle
	s.Blurred.Prompt = lipgloss.NewStyle().Foreground(colorDim)

	s.Cursor.Blink = true
	ta.SetStyles(s)

	return ta
}

// newSessionsInput creates a text input for filtering the sessions overview tab.
func newSessionsInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Filter sessions…"
	ti.Prompt = "  "
	return ti
}

// renderQueueIndicator draws a single-line banner above the input
// box whenever the user has a message queued (pendingInput is
// non-nil). The banner stays visible until the message is sent,
// so the user always knows what's waiting in the queue.
//
// Two variants:
//   - "Queued:" — Tab pressed; the message will run after the
//     current turn finishes.
//   - "Sending after current edit:" — Enter pressed during a
//     generation; the message will be sent when the current
//     edit boundary is reached.
func renderQueueIndicator(pending *pendingMsg, width int, styles Styles) string {
	if pending == nil {
		return ""
	}
	preview := pending.text
	// Strip newlines so the banner stays a single visible line.
	preview = strings.ReplaceAll(preview, "\n", " ")
	preview = strings.ReplaceAll(preview, "\r", " ")
	preview = strings.ReplaceAll(preview, "\t", " ")
	// Trim repeated whitespace.
	for strings.Contains(preview, "  ") {
		preview = strings.ReplaceAll(preview, "  ", " ")
	}
	preview = strings.TrimSpace(preview)
	if preview == "" && len(pending.attachments) > 0 {
		preview = fmt.Sprintf("[%d attachment(s)]", len(pending.attachments))
	}
	if preview == "" {
		preview = "(empty)"
	}
	// Cap to ~80 chars; the surrounding badge text + width
	// accounting is handled by lipgloss.
	maxLen := 80
	if len(preview) > maxLen {
		preview = preview[:maxLen-1] + "…"
	}
	// Render with a distinct style so the user notices it.
	//   ▸ Queued: "fix the typo"
	//   ▸ Sending after current edit: "and add tests"
	var badge string
	if pending.Queued {
		badge = "▸ Queued: "
	} else {
		badge = "▸ Sending after current edit: "
	}
	indicatorStyle := lipgloss.NewStyle().
		Foreground(colorAccentWarm).
		Bold(true).
		Width(width)
	body := badge + lipgloss.NewStyle().Foreground(colorAccentCool).Italic(true).Render(`"`+preview+`"`)
	return indicatorStyle.Render(body)
}

// renderInputBox wraps the textarea in a rounded border box with mode title embedded in top border.
// When focused is false, the border uses a dim grey color instead of the mode color.
func renderInputBox(modeName string, isWorkflow bool, textareaView string, width int, focused bool, dimColor color.Color) string {
	var titleStyle lipgloss.Style
	var borderColor color.Color

	title := " " + modeName + " "
	if isWorkflow {
		titleStyle = planBarStyle
		borderColor = colorSecondary
	} else {
		titleStyle = chatBarStyle
		borderColor = colorPrimary
	}

	if !focused {
		borderColor = dimColor
		titleStyle = lipgloss.NewStyle().Foreground(dimColor)
	}

	// 1. Build custom top border with embedded title: "╭─ Title ──...──╮"
	borderCharStyle := lipgloss.NewStyle().Foreground(borderColor)

	// Total top border = width chars: "╭" + "─" + title + dashes + "╮"
	titleRendered := titleStyle.Render(title)
	titleLen := lipgloss.Width(titleRendered)

	// Fill remaining space with dashes: width - 2(╭╮) - 1(leading ─) - titleLen
	remainingDashes := width - 3 - titleLen
	if remainingDashes < 0 {
		remainingDashes = 0
	}
	dashes := strings.Repeat("─", remainingDashes)

	topBorder := borderCharStyle.Render("╭─") + titleRendered + borderCharStyle.Render(dashes) + borderCharStyle.Render("╮")

	// 2. Use lipgloss for sides + bottom (no top border)
	// Width is total visual width (includes border + padding)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderTop(false).
		BorderForeground(borderColor).
		Width(width).
		Padding(0, 1)

	body := boxStyle.Render(textareaView)

	return topBorder + "\n" + body
}

// renderInputKeybindHint draws Enter / Tab / Esc under the chat input box.
func renderInputKeybindHint(width int) string {
	badgeStyle := lipgloss.NewStyle().Background(colorPrimary).Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	dimLabel := lipgloss.NewStyle().Foreground(colorDim)
	parts := []string{
		badgeStyle.Render(" Enter ") + dimLabel.Render(" send  "),
		badgeStyle.Render(" Tab ") + dimLabel.Render(" queue  "),
		badgeStyle.Render(" Esc ") + dimLabel.Render(" cancel"),
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(parts, ""))
}
