package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
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

func TestSettingsProviders_ShowsAddCustomProviderButton(t *testing.T) {
	s := NewStyles(true)
	view := renderSettingsView(120, 40, s,
		0, 0, 0, 0,
		"GPT-5.5", "codex",
		nil, nil,
		[]ProviderSettingsView{{Provider: "codex", DisplayName: "ChatGPT (codex)"}},
		1, 0, SettingsOtherView{}, SettingsInspectView{},
		false, "", "",
		SettingsWizardView{},
	)
	plain := stripANSI(view)
	for _, want := range []string{"+ Add custom provider", "A", "A add custom"} {
		if !strings.Contains(plain, want) && !strings.Contains(view, want) {
			t.Fatalf("expected add-custom-provider UI to mention %q, got:\n%s", want, view)
		}
	}
}

func TestSettingsProviderRows_ShowCustomBadge(t *testing.T) {
	s := NewStyles(true)
	view := renderSettingsView(120, 40, s,
		0, 0, 0, 0,
		"my-local/qwen2.5-coder-32b", "my-local",
		nil, nil,
		[]ProviderSettingsView{
			{Provider: "codex", DisplayName: "ChatGPT (codex)"},
			{Provider: "my-local", DisplayName: "my-local", IsCustom: true},
		},
		1, 0, SettingsOtherView{}, SettingsInspectView{},
		false, "", "",
		SettingsWizardView{},
	)
	var found bool
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(stripANSI(line), "my-local") {
			found = true
			if !strings.Contains(stripANSI(line), "custom") {
				t.Fatalf("expected custom badge on my-local row, got:\n%s", line)
			}
		}
	}
	if !found {
		t.Fatalf("my-local provider row not found in:\n%s", view)
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
		0, 4, other, SettingsInspectView{},
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
	if !strings.Contains(row, lipgloss.NewStyle().Foreground(s.ColorWhite).Render(" Other Settings ")) {
		t.Fatalf("inactive section tab should be white, got %q", plain)
	}
}

func TestRenderSettingsView_UsesMatchingHeaderDividers(t *testing.T) {
	s := NewStyles(true)
	view := renderSettingsView(120, 40, s,
		0, 0, 0, 0, "", "", nil, nil,
		[]ProviderSettingsView{{Provider: "codex", DisplayName: "ChatGPT (codex)"}},
		0, 0, SettingsOtherView{}, SettingsInspectView{},
		false, "", "", SettingsWizardView{},
	)
	var dividerLines int
	for _, line := range strings.Split(stripANSI(view), "\n") {
		if strings.Count(line, "─") != settingsHeaderDividerLen {
			continue
		}
		withoutDashes := strings.ReplaceAll(line, "─", "")
		if strings.Trim(withoutDashes, " │\t") == "" {
			dividerLines++
		}
	}
	if dividerLines != 2 {
		t.Fatalf("expected two %d-char divider lines, got %d in:\n%s", settingsHeaderDividerLen, dividerLines, view)
	}
}

func TestSettingsOtherSettingsRows_AllBoldWhiteUnlessCursor(t *testing.T) {
	s := NewStyles(true)
	other := SettingsOtherView{
		Theme:        "auto",
		PrimaryColor: "default (#3B82F6)",
		ShowThinking: true,
		ShowUsage:    true,
		AutoCompact:  true,
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
	if strings.Contains(view, "\x1b[2m") {
		t.Fatalf("Other Settings rows should not use dim style, got:\n%s", view)
	}
	for _, label := range []string{"Theme", "Primary color", "Show extended thinking", "Show token usage", "Auto-compact context"} {
		if !strings.Contains(stripANSI(view), label) {
			t.Fatalf("missing Other Settings row %q", label)
		}
	}
}

func TestSettingsAddCustomProvider_EnterOpensNamePrompt(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	m.activeTab = TabKindSettings
	m.settingsActiveSection = 0
	m.refreshSettingsKeys()
	m.settingsKeySel = settingsProviderAddRowIndex(m.settingsKeys)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	um := updated.(Model)
	if !um.settingsInKeyInput {
		t.Fatal("expected custom provider name prompt after Enter on add row")
	}
	if um.settingsKeyInputLabel != "New provider name" {
		t.Fatalf("settingsKeyInputLabel = %q, want New provider name", um.settingsKeyInputLabel)
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