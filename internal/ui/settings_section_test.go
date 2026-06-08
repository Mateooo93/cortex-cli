package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func settingsSectionTabLine(view string) string {
	for _, line := range strings.Split(view, "\n") {
		plain := stripANSI(line)
		if strings.Contains(plain, "Providers") && strings.Contains(plain, "Other Settings") {
			return line
		}
	}
	return ""
}

// TestSettingsSectionOtherSettings_HighlightedWhenActive verifies Tab
// switches to Other Settings: the section tab row highlights Other
// Settings and only that section's rows are shown.
func TestSettingsSectionOtherSettings_HighlightedWhenActive(t *testing.T) {
	s := NewStyles(true)
	other := SettingsOtherView{
		Theme:        "auto",
		ShowThinking: true,
		ShowUsage:    true,
		AutoCompact:  true,
	}
	inspect := SettingsInspectView{}
	view := renderSettingsView(120, 40, s,
		1, // activeSection = Other Settings
		0, 0, 0,
		"GPT-5.5", "codex",
		nil, nil,
		[]ProviderSettingsView{
			{Provider: "codex", DisplayName: "ChatGPT (codex)"},
			{Provider: "openai", DisplayName: "OpenAI"},
		},
		0, 0, other, inspect,
		false, "", "",
		SettingsWizardView{},
	)
	tabLine := settingsSectionTabLine(view)
	if tabLine == "" {
		t.Fatal("could not find section tab row")
	}
	if !strings.Contains(tabLine, s.TabActiveStyle.Render(" Other Settings ")) {
		t.Errorf("expected Other Settings tab to be active, got %q", stripANSI(tabLine))
	}
	if strings.Contains(stripANSI(view), "ChatGPT (codex)") {
		t.Errorf("provider rows should be hidden in Other Settings section, got:\n%s", view)
	}
	var foundCursor bool
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Theme") {
			if strings.Contains(line, "\u25b8 Theme") {
				foundCursor = true
			}
			break
		}
	}
	if !foundCursor {
		t.Errorf("expected 'Theme' row to be highlighted with ▸ cursor, lines:\n%s", view)
	}
}

// TestSettingsSectionProviders_HighlightedWhenActive verifies the
// Providers tab is active and provider rows are shown.
func TestSettingsSectionProviders_HighlightedWhenActive(t *testing.T) {
	s := NewStyles(true)
	other := SettingsOtherView{
		Theme:        "auto",
		ShowThinking: true,
		ShowUsage:    true,
		AutoCompact:  true,
	}
	view := renderSettingsView(120, 40, s,
		0, // activeSection = Providers
		0, 0, 0,
		"GPT-5.5", "codex",
		nil, nil,
		[]ProviderSettingsView{
			{Provider: "codex", DisplayName: "ChatGPT (codex)"},
		},
		0, 0, other, SettingsInspectView{},
		false, "", "",
		SettingsWizardView{},
	)
	tabLine := settingsSectionTabLine(view)
	if tabLine == "" {
		t.Fatal("could not find section tab row")
	}
	if !strings.Contains(tabLine, s.TabActiveStyle.Render(" Providers ")) {
		t.Errorf("expected Providers tab to be active, got %q", stripANSI(tabLine))
	}
	if !strings.Contains(stripANSI(view), "ChatGPT (codex)") {
		t.Errorf("expected provider row in Providers section, got:\n%s", view)
	}
	if strings.Contains(stripANSI(view), "Auto-compact context") {
		t.Errorf("Other Settings rows should be hidden in Providers section, got:\n%s", view)
	}
}

func TestSettingsProviderRows_AllBoldUnlessCursor(t *testing.T) {
	s := NewStyles(true)
	view := renderSettingsView(120, 40, s,
		0, 1, 0, 0,
		"GPT-5.5", "codex",
		nil, nil,
		[]ProviderSettingsView{
			{Provider: "codex", DisplayName: "ChatGPT (codex)"},
			{Provider: "openai", DisplayName: "OpenAI"},
			{Provider: "anthropic", DisplayName: "Anthropic"},
		},
		1, 0, SettingsOtherView{}, SettingsInspectView{},
		false, "", "",
		SettingsWizardView{},
	)
	plain := stripANSI(view)
	for _, name := range []string{"ChatGPT (codex)", "OpenAI", "Anthropic"} {
		if !strings.Contains(plain, name) {
			t.Fatalf("missing provider %q in view", name)
		}
	}
	if strings.Contains(view, "\x1b[2m") {
		t.Fatalf("provider rows should not use dim style, got:\n%s", view)
	}
}

func TestSettingsSelectedRowHighlightsTextOnly(t *testing.T) {
	s := NewStyles(true)
	innerWidth := 80
	short := "▸ Theme                      auto"
	full := renderSettingsLine(s.TabInactiveStyle, short, innerWidth)
	textOnly := renderSettingsSelectLine(
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorPrimary),
		short,
		innerWidth,
	)
	if lipgloss.Width(textOnly) >= innerWidth {
		t.Fatalf("selected row should not span full width, got width %d", lipgloss.Width(textOnly))
	}
	if lipgloss.Width(full) < innerWidth-2 {
		t.Fatalf("non-selected row should still use full line width, got %d", lipgloss.Width(full))
	}
}

func TestSettingsOtherSettings_IncludesColorRows(t *testing.T) {
	s := NewStyles(true)
	other := SettingsOtherView{
		Theme:        "auto",
		PrimaryColor: "default (#3B82F6)",
	}
	view := renderSettingsView(120, 40, s,
		1, 0, 0, 0,
		"GPT-5.5", "codex",
		nil, nil,
		[]ProviderSettingsView{{Provider: "codex", DisplayName: "ChatGPT (codex)"}},
		0, 1, other, SettingsInspectView{},
		false, "", "",
		SettingsWizardView{},
	)
	if !strings.Contains(view, "Primary color") {
		t.Errorf("expected Primary color row, got:\n%s", view)
	}
	if strings.Contains(view, "Secondary color") {
		t.Errorf("unexpected Secondary color row, got:\n%s", view)
	}
}

// TestSettingsOtherSettings_IncludesAutoCompactRow pins the
// new "Auto-compact context" row in the Other Settings list.
func TestSettingsOtherSettings_IncludesAutoCompactRow(t *testing.T) {
	s := NewStyles(true)
	other := SettingsOtherView{
		Theme:        "auto",
		ShowThinking: false,
		ShowUsage:    false,
		AutoCompact:  true,
	}
	view := renderSettingsView(120, 40, s,
		1, // Other Settings
		0, 0, 0,
		"GPT-5.5", "codex",
		nil, nil,
		[]ProviderSettingsView{{Provider: "codex", DisplayName: "ChatGPT (codex)"}},
		0, 6, other, SettingsInspectView{},
		false, "", "",
		SettingsWizardView{},
	)
	if !strings.Contains(view, "Auto-compact context") {
		t.Errorf("expected 'Auto-compact context' row, got:\n%s", view)
	}
	if !strings.Contains(view, "On") {
		t.Errorf("expected 'On' status for Auto-compact when enabled, got:\n%s", view)
	}
}

func TestRenderSettingsSectionTabBar_ShowsBothSections(t *testing.T) {
	s := NewStyles(true)
	row := renderSettingsSectionTabBar(0, 80, s)
	plain := stripANSI(row)
	for _, want := range []string{"Providers", "Other Settings"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in %q", want, plain)
		}
	}
}

func TestRenderSettingsSectionSwitchHint_ContextAware(t *testing.T) {
	tests := []struct {
		section int
		want    string
	}{
		{0, "switch to Other Settings"},
		{1, "switch to Providers"},
	}
	for _, tc := range tests {
		plain := stripANSI(renderSettingsSectionSwitchHint(tc.section, 80))
		if !strings.Contains(plain, "Tab") {
			t.Fatalf("section %d: missing Tab in %q", tc.section, plain)
		}
		if !strings.Contains(plain, tc.want) {
			t.Fatalf("section %d: want %q in %q", tc.section, tc.want, plain)
		}
	}
}