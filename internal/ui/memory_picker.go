package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Mateooo93/cortex-cli/internal/memory"
)

// MemoryPickerRow is one entry in the /memory browser.
type MemoryPickerRow struct {
	Entry    memory.Entry
	Expanded bool
}

// MemoryPicker is a centered modal for browsing and deleting project memories.
type MemoryPicker struct {
	visible  bool
	query    string
	selected int
	rows     []MemoryPickerRow
	enabled  bool
	errText  string
}

// NewMemoryPicker creates an empty picker.
func NewMemoryPicker() MemoryPicker { return MemoryPicker{} }

// Open loads memories from store and shows the picker.
func (p *MemoryPicker) Open(store *memory.Store, enabled bool) {
	p.visible = true
	p.query = ""
	p.selected = 0
	p.enabled = enabled
	p.errText = ""
	p.reload(store)
}

// Close hides the picker.
func (p *MemoryPicker) Close() { p.visible = false }

// IsVisible reports whether the picker is open.
func (p *MemoryPicker) IsVisible() bool { return p.visible }

// SetQuery updates the search filter.
func (p *MemoryPicker) SetQuery(q string, store *memory.Store) {
	p.query = q
	p.reload(store)
}

// Query returns the active search string.
func (p *MemoryPicker) Query() string { return p.query }

func (p *MemoryPicker) reload(store *memory.Store) {
	if store == nil {
		p.rows = nil
		if !p.enabled {
			p.errText = "Project memory is disabled in Settings."
		} else {
			p.errText = "Memory store unavailable."
		}
		return
	}
	entries, err := store.Search(p.query)
	if err != nil {
		p.rows = nil
		p.errText = err.Error()
		return
	}
	p.errText = ""
	rows := make([]MemoryPickerRow, len(entries))
	for i, e := range entries {
		rows[i] = MemoryPickerRow{Entry: e}
	}
	p.rows = rows
	if p.selected >= len(p.rows) {
		p.selected = max(0, len(p.rows)-1)
	}
}

// MoveUp moves selection up.
func (p *MemoryPicker) MoveUp() {
	if p.selected > 0 {
		p.selected--
	}
}

// MoveDown moves selection down.
func (p *MemoryPicker) MoveDown() {
	if p.selected < len(p.rows)-1 {
		p.selected++
	}
}

// ToggleExpand expands or collapses the selected row.
func (p *MemoryPicker) ToggleExpand() {
	if p.selected < 0 || p.selected >= len(p.rows) {
		return
	}
	p.rows[p.selected].Expanded = !p.rows[p.selected].Expanded
}

// SelectedEntry returns the highlighted memory, if any.
func (p *MemoryPicker) SelectedEntry() *memory.Entry {
	if p.selected < 0 || p.selected >= len(p.rows) {
		return nil
	}
	e := p.rows[p.selected].Entry
	return &e
}

// DeleteSelected removes the highlighted memory from the store.
func (p *MemoryPicker) DeleteSelected(store *memory.Store) error {
	e := p.SelectedEntry()
	if e == nil || store == nil {
		return nil
	}
	if err := store.Delete(e.ID); err != nil {
		return err
	}
	p.reload(store)
	return nil
}

// VisibleHeight estimates modal height for centering.
func (p *MemoryPicker) VisibleHeight() int {
	rows := min(12, max(4, len(p.rows)))
	if p.errText != "" {
		rows = 4
	}
	return 6 + rows
}

// View renders the memory browser modal.
func (p *MemoryPicker) View(width, height int, s Styles) string {
	modalWidth := min(width-8, 88)
	if modalWidth < 40 {
		modalWidth = 40
	}
	innerWidth := modalWidth - 6

	title := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render("Project memory")
	subtitle := lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render("Scoped to this repository's .cortex directory")

	searchPrompt := " /"
	searchStyle := lipgloss.NewStyle().Foreground(colorAccentWarm).Bold(true)
	if p.query == "" {
		searchStyle = searchStyle.Italic(true).Foreground(colorDim)
	}
	searchLine := searchStyle.Render(searchPrompt) + p.query + "\u2588"
	searchBox := lipgloss.NewStyle().Width(innerWidth).Render(searchLine)

	var bodyRows []string
	if p.errText != "" {
		bodyRows = append(bodyRows, lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render("  "+p.errText))
	} else if len(p.rows) == 0 {
		bodyRows = append(bodyRows, lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render("  (no memories yet — the agent can save durable facts with memory_write)"))
	} else {
		maxVisible := min(12, height-8)
		start, end := settingsWindow(p.selected, len(p.rows), maxVisible)
		for i := start; i < end; i++ {
			row := p.rows[i]
			prefix := "  "
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
			if i == p.selected {
				prefix = "▸ "
				style = selectedRowStyle()
			}
			line := fmt.Sprintf("%s[%s] %s", prefix, row.Entry.Type, truncateMiddle(row.Entry.Content, innerWidth-12))
			bodyRows = append(bodyRows, style.Render(settingsTruncate(line, innerWidth)))
			if row.Expanded && i == p.selected {
				meta := fmt.Sprintf("    id: %s · importance: %.2f · source: %s",
					row.Entry.ID[:8], row.Entry.Importance, row.Entry.Source)
				bodyRows = append(bodyRows, lipgloss.NewStyle().Foreground(colorDim).Render(settingsTruncate(meta, innerWidth)))
				updated := fmt.Sprintf("    updated: %s", row.Entry.UpdatedAt.Format(time.RFC3339))
				bodyRows = append(bodyRows, lipgloss.NewStyle().Foreground(colorDim).Render(settingsTruncate(updated, innerWidth)))
				bodyRows = append(bodyRows, lipgloss.NewStyle().Foreground(colorDim).Render(settingsTruncate("    "+row.Entry.Content, innerWidth)))
			}
		}
	}

	body := strings.Join([]string{searchBox, "", strings.Join(bodyRows, "\n")}, "\n")
	inner := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Width(modalWidth).
		Render(title + "\n" + subtitle + "\n\n" + body)

	footer := lipgloss.NewStyle().
		Foreground(colorDim).
		Width(modalWidth).
		Align(lipgloss.Center).
		Render("↑↓ navigate · Enter expand · d delete · Esc close · type to search")

	return lipgloss.PlaceHorizontal(width, lipgloss.Center, inner+"\n"+footer)
}

func truncateMiddle(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max < 8 {
		return s[:max]
	}
	keep := (max - 1) / 2
	return s[:keep] + "…" + s[len(s)-keep:]
}