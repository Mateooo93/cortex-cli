package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"image"
	uv "github.com/charmbracelet/ultraviolet"
)

type ctxMenuAction string

const (
	ctxActionPaste ctxMenuAction = "paste"
	ctxActionCopy  ctxMenuAction = "copy"
)

type ctxMenuItem struct {
	label  string
	action ctxMenuAction
}

type contextMenu struct {
	active bool
	x, y   int
	sel    int
	items  []ctxMenuItem
}

func (m Model) canOpenContextMenu() bool {
	if m.pasteTarget() != pasteTargetNone {
		return true
	}
	sess := m.currentSession()
	return sess != nil && sess.chatSel.active
}

func (m *Model) buildContextMenuItems() []ctxMenuItem {
	var items []ctxMenuItem
	if m.pasteTarget() != pasteTargetNone {
		items = append(items, ctxMenuItem{label: "Paste", action: ctxActionPaste})
	}
	sess := m.currentSession()
	if sess != nil && sess.chatSel.active {
		items = append(items, ctxMenuItem{label: "Copy selection", action: ctxActionCopy})
	}
	return items
}

func (m *Model) openContextMenu(x, y int, items []ctxMenuItem) {
	if len(items) == 0 {
		m.contextMenu.active = false
		return
	}
	m.contextMenu.active = true
	m.contextMenu.x = x
	m.contextMenu.y = y
	m.contextMenu.sel = 0
	m.contextMenu.items = items
}

func (m *Model) closeContextMenu() {
	m.contextMenu.active = false
	m.contextMenu.items = nil
	m.contextMenu.sel = 0
}

func (m Model) contextMenuRect() image.Rectangle {
	if !m.contextMenu.active || len(m.contextMenu.items) == 0 {
		return image.Rectangle{}
	}
	w := 0
	for _, it := range m.contextMenu.items {
		if lw := len(it.label); lw > w {
			w = lw
		}
	}
	w += 4
	h := len(m.contextMenu.items) + 2
	x0 := m.contextMenu.x
	y0 := m.contextMenu.y
	if x0+w > m.width {
		x0 = m.width - w
	}
	if y0+h > m.height {
		y0 = m.height - h
	}
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	return image.Rect(x0, y0, x0+w, y0+h)
}

func (m Model) contextMenuItemAt(x, y int) int {
	r := m.contextMenuRect()
	if x < r.Min.X || x >= r.Max.X || y < r.Min.Y+1 || y >= r.Max.Y-1 {
		return -1
	}
	idx := y - (r.Min.Y + 1)
	if idx < 0 || idx >= len(m.contextMenu.items) {
		return -1
	}
	return idx
}

func (m Model) handleContextMenuKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if !m.contextMenu.active {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.closeContextMenu()
		return m, nil
	case "up", "k":
		if m.contextMenu.sel > 0 {
			m.contextMenu.sel--
		}
		return m, nil
	case "down", "j":
		if m.contextMenu.sel < len(m.contextMenu.items)-1 {
			m.contextMenu.sel++
		}
		return m, nil
	case "enter":
		return m.executeContextMenuItem(m.contextMenu.sel)
	default:
		m.closeContextMenu()
		return m, nil
	}
}

func (m Model) executeContextMenuItem(idx int) (Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.contextMenu.items) {
		m.closeContextMenu()
		return m, nil
	}
	action := m.contextMenu.items[idx].action
	m.closeContextMenu()
	switch action {
	case ctxActionPaste:
		return m.handlePasteKey()
	case ctxActionCopy:
		if cmd := m.copyChatSelectionCmd(); cmd != nil {
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) handleContextMenuClick(x, y int) (Model, tea.Cmd) {
	if !m.contextMenu.active {
		return m, nil
	}
	idx := m.contextMenuItemAt(x, y)
	if idx < 0 {
		m.closeContextMenu()
		return m, nil
	}
	return m.executeContextMenuItem(idx)
}

func (m Model) drawContextMenu(canvas uv.ScreenBuffer) {
	if !m.contextMenu.active || len(m.contextMenu.items) == 0 {
		return
	}
	r := m.contextMenuRect()
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6B7280"))
	normal := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
	selected := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#3B82F6")).
		Bold(true)

	var lines []string
	for i, item := range m.contextMenu.items {
		label := " " + item.label + " "
		if i == m.contextMenu.sel {
			lines = append(lines, selected.Render(label))
		} else {
			lines = append(lines, normal.Render(label))
		}
	}
	menu := box.Render(strings.Join(lines, "\n"))
	uv.NewStyledString(menu).Draw(&canvas, r)
}