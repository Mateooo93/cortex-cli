package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucasb-eyer/go-colorful"
)

// Default brand colors (match internal/ui/styles.go).
const (
	DefaultThemePrimary   = "#3B82F6"
	DefaultThemeSecondary = "#60A5FA"
)

// ThemeColorPreset is a named primary/secondary pair cycled in Settings.
type ThemeColorPreset struct {
	Name      string
	Primary   string // empty = built-in default
	Secondary string
}

// ThemeColorPresets are the accent palettes users cycle with Enter in Settings.
var ThemeColorPresets = []ThemeColorPreset{
	{Name: "default", Primary: "", Secondary: ""},
	{Name: "violet", Primary: "#8B5CF6", Secondary: "#A78BFA"},
	{Name: "emerald", Primary: "#10B981", Secondary: "#34D399"},
	{Name: "amber", Primary: "#F59E0B", Secondary: "#FBBF24"},
	{Name: "rose", Primary: "#F43F5E", Secondary: "#FB7185"},
	{Name: "cyan", Primary: "#06B6D4", Secondary: "#22D3EE"},
	{Name: "orange", Primary: "#F97316", Secondary: "#FB923C"},
	{Name: "indigo", Primary: "#6366F1", Secondary: "#818CF8"},
}

func normalizeStoredColor(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	c, err := colorful.Hex(raw)
	if err != nil {
		return strings.ToUpper(raw)
	}
	return strings.ToUpper(c.Hex())
}

func themeColorPresetIndex(primary, secondary string) int {
	p := normalizeStoredColor(primary)
	s := normalizeStoredColor(secondary)
	for i, preset := range ThemeColorPresets {
		if preset.Primary == p && preset.Secondary == s {
			return i
		}
	}
	return -1
}

// NextThemeColorPreset returns the next preset pair after the current colors.
func NextThemeColorPreset(primary, secondary string) (string, string) {
	idx := themeColorPresetIndex(primary, secondary)
	next := ThemeColorPresets[(idx+1)%len(ThemeColorPresets)]
	return next.Primary, next.Secondary
}

// ThemeColorPresetName returns the display name for a stored primary/secondary pair.
func ThemeColorPresetName(primary, secondary string) string {
	idx := themeColorPresetIndex(primary, secondary)
	if idx < 0 {
		return "custom"
	}
	return ThemeColorPresets[idx].Name
}

// ThemePrimaryDisplayName formats the primary color row in Settings.
func ThemePrimaryDisplayName(stored string) string {
	stored = normalizeStoredColor(stored)
	for _, preset := range ThemeColorPresets {
		if preset.Primary == stored {
			if preset.Name == "default" {
				return "default (" + DefaultThemePrimary + ")"
			}
			return preset.Name + " (" + stored + ")"
		}
	}
	if stored == "" {
		return "default (" + DefaultThemePrimary + ")"
	}
	return stored
}

// ThemeSecondaryDisplayName formats the secondary color row in Settings.
func ThemeSecondaryDisplayName(stored string) string {
	stored = normalizeStoredColor(stored)
	if stored == "" {
		return "default (" + DefaultThemeSecondary + ")"
	}
	for _, preset := range ThemeColorPresets {
		if preset.Secondary == stored {
			return preset.Name + " (" + stored + ")"
		}
	}
	return stored
}

// NormalizeHexColor validates and canonicalizes a #RRGGBB color. Empty input
// means "use the built-in default" (caller clears the stored override).
func NormalizeHexColor(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.HasPrefix(raw, "#") {
		raw = "#" + raw
	}
	if len(raw) != 7 {
		return "", fmt.Errorf("expected #RRGGBB, got %q", raw)
	}
	c, err := colorful.Hex(raw)
	if err != nil {
		return "", fmt.Errorf("invalid hex color %q", raw)
	}
	return strings.ToUpper(c.Hex()), nil
}

// EffectiveThemePrimary returns the configured primary or the default.
func (tc ThemeConfig) EffectivePrimary() string {
	if tc.Primary != "" {
		return tc.Primary
	}
	return DefaultThemePrimary
}

// EffectiveSecondary returns the configured secondary or the default.
func (tc ThemeConfig) EffectiveSecondary() string {
	if tc.Secondary != "" {
		return tc.Secondary
	}
	return DefaultThemeSecondary
}

// SetThemeColors writes primary/secondary colors to ~/.cortex/settings.json so
// they survive binary updates and reinstalls. Empty strings remove overrides and
// restore the built-in defaults on next launch.
func SetThemeColors(primary, secondary string) error {
	primary, err := NormalizeHexColor(primary)
	if err != nil {
		return err
	}
	secondary, err = NormalizeHexColor(secondary)
	if err != nil {
		return err
	}

	home := HomeCortexDir()
	if home == "" {
		return fmt.Errorf("no home directory")
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	p := filepath.Join(home, "settings.json")

	raw := map[string]any{}
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &raw)
	}

	theme, _ := raw["theme"].(map[string]any)
	if theme == nil {
		theme = map[string]any{}
	}
	if primary == "" {
		delete(theme, "primary")
	} else {
		theme["primary"] = primary
	}
	if secondary == "" {
		delete(theme, "secondary")
	} else {
		theme["secondary"] = secondary
	}
	if len(theme) == 0 {
		delete(raw, "theme")
	} else {
		raw["theme"] = theme
	}

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, out, 0o644)
}