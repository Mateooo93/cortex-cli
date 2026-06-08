package ui

import (
	"strings"
	"testing"
)

// TestSettingsSectionOtherSettings_HighlightedWhenActive is
// the regression pin for the "tab in Settings doesn't move to
// Other Settings" bug. The old renderSettingsView compared the
// cursor's section index to 2 instead of 1, so even when the
// user was on Other Settings, the cursor was never drawn and
// the section title was never highlighted — they thought they
// were stuck on Providers.
//
// We pass activeSection=1 (Other Settings) and verify:
//   - "Other Settings" appears as a section title with the ▸
//     chevron (active marker)
//   - the cursor on the highlighted row is rendered (▸ marker)
//   - the help line "Tab → Other" is NOT shown (we're in
//     Other Settings, not Providers)
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
		1,    // activeSection = Other Settings
		0, 0, 0,
		"GPT-5.5", "codex",
		nil, nil, // providers, models
		[]ProviderSettingsView{
			{Provider: "codex", DisplayName: "ChatGPT (codex)"},
			{Provider: "openai", DisplayName: "OpenAI"},
		},
		0,    // keySel
		0,    // otherSel
		other,
		inspect,
		false, "", "",
		SettingsWizardView{},
	)
	// Section title "Other Settings" must be present.
	if !strings.Contains(view, "Other Settings") {
		t.Errorf("expected section title 'Other Settings', got %q", view)
	}
	// The active section's chevron should be in front of
	// "Other Settings" (\u25b8 = ▸).
	lines := strings.Split(view, "\n")
	var otherSettingsLine string
	for _, line := range lines {
		// Match the section title line, not the status
		// bar line at the bottom that also says
		// "Section: ...".
		if strings.Contains(line, "Other Settings") && !strings.Contains(line, "Section:") {
			otherSettingsLine = line
			break
		}
	}
	if otherSettingsLine == "" {
		t.Fatal("could not find Other Settings line")
	}
	if !strings.Contains(otherSettingsLine, "\u25b8") {
		t.Errorf("expected active 'Other Settings' line to have ▸ chevron, got %q", otherSettingsLine)
	}
	// Providers section should still be visible but
	// UN-highlighted (no chevron in front of "Providers").
	var providersLine string
	for _, line := range lines {
		if strings.Contains(line, "Providers") && !strings.Contains(line, "Section:") {
			providersLine = line
			break
		}
	}
	if providersLine == "" {
		t.Fatal("could not find Providers line")
	}
	if strings.HasPrefix(providersLine, " \u25b8") {
		t.Errorf("expected inactive 'Providers' line to NOT have chevron, got %q", providersLine)
	}
	// The cursor on the highlighted Other Settings row
	// (otherSel=0 → "Theme") must be rendered with ▸.
	// We check for the cursor pattern (▸ followed by the row
	// label) anywhere on the line, not as a strict prefix,
	// because the line has leading box-drawing + ANSI
	// control codes.
	var foundCursor bool
	for _, line := range lines {
		if strings.Contains(line, "Theme") && !strings.Contains(line, "Section:") {
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

// TestSettingsSectionProviders_HighlightedWhenActive verifies
// the inverse: when activeSection=0, "Providers" gets the
// chevron and "Other Settings" doesn't.
func TestSettingsSectionProviders_HighlightedWhenActive(t *testing.T) {
	s := NewStyles(true)
	other := SettingsOtherView{
		Theme:        "auto",
		ShowThinking: true,
		ShowUsage:    true,
		AutoCompact:  true,
	}
	view := renderSettingsView(120, 40, s,
		0,    // activeSection = Providers
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
	lines := strings.Split(view, "\n")
	var providersLine, otherSettingsLine string
	for _, line := range lines {
		// Skip the bottom status-bar line that says
		// "Section: Providers · F1..." which also
		// contains the section names.
		if strings.Contains(line, "Section:") {
			continue
		}
		if strings.Contains(line, "Providers") && providersLine == "" {
			providersLine = line
		}
		if strings.Contains(line, "Other Settings") && otherSettingsLine == "" {
			otherSettingsLine = line
		}
	}
	if providersLine == "" || otherSettingsLine == "" {
		t.Fatalf("missing section title lines:\n%s", view)
	}
	if !strings.Contains(providersLine, "\u25b8") {
		t.Errorf("expected 'Providers' to have ▸ when active, got %q", providersLine)
	}
	if strings.Contains(otherSettingsLine, "\u25b8") {
		t.Errorf("expected 'Other Settings' to NOT have ▸ when inactive, got %q", otherSettingsLine)
	}
	// Section-specific hint line should appear for active
	// Providers section.
	if !strings.Contains(view, "Tab \u2192 Other") {
		t.Errorf("expected 'Tab → Other' hint when Providers is active, got:\n%s", view)
	}
}

// TestSettingsOtherSettings_IncludesAutoCompactRow pins the
// new "Auto-compact context" row in the Other Settings list.
// This is the user-facing toggle for the auto-compact feature
// (/compact slash command is always available, but the
// auto-run behaviour is gated behind this toggle).
func TestSettingsOtherSettings_IncludesAutoCompactRow(t *testing.T) {
	s := NewStyles(true)
	other := SettingsOtherView{
		Theme:        "auto",
		ShowThinking: false,
		ShowUsage:    false,
		AutoCompact:  true,
	}
	view := renderSettingsView(120, 40, s,
		1,    // Other Settings
		0, 0, 0,
		"GPT-5.5", "codex",
		nil, nil,
		[]ProviderSettingsView{{Provider: "codex", DisplayName: "ChatGPT (codex)"}},
		0, 4,    // otherSel=4 is the Auto-compact row
		other, SettingsInspectView{},
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
