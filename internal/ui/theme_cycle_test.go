package ui

import (
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
)

func TestCycleThemeColors_AdvancesPreset(t *testing.T) {
	m := NewModel(&config.Config{}, nil, nil, true, "", true, true)
	m.themeColors = config.ThemeConfig{}

	if err := m.cycleThemeColors(); err != nil {
		t.Fatal(err)
	}
	if m.themeColors.Primary != "#8B5CF6" || m.themeColors.Secondary != "#A78BFA" {
		t.Fatalf("after first cycle = %+v", m.themeColors)
	}
	if config.ThemeColorPresetName(m.themeColors.Primary, m.themeColors.Secondary) != "violet" {
		t.Fatalf("expected violet preset")
	}
	if primaryHex != "#8B5CF6" {
		t.Fatalf("primaryHex = %q", primaryHex)
	}
}