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

// ThemeColorPreset is a named primary accent cycled in Settings.
type ThemeColorPreset struct {
	Name    string
	Primary string // empty = built-in default blue
}

// ThemeColorPresets are the primary accent colors users cycle with Enter.
// Secondary always stays the default sky blue.
var ThemeColorPresets = []ThemeColorPreset{
	{Name: "default", Primary: ""},
	{Name: "violet", Primary: "#8B5CF6"},
	{Name: "emerald", Primary: "#10B981"},
	{Name: "amber", Primary: "#F59E0B"},
	{Name: "rose", Primary: "#F43F5E"},
	{Name: "cyan", Primary: "#06B6D4"},
	{Name: "orange", Primary: "#F97316"},
	{Name: "indigo", Primary: "#6366F1"},
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

func themeColorPresetIndex(primary string) int {
	p := normalizeStoredColor(primary)
	for i, preset := range ThemeColorPresets {
		if preset.Primary == p {
			return i
		}
	}
	return -1
}

// NextThemeColorPreset returns the next primary preset after the current value.
func NextThemeColorPreset(primary string) string {
	idx := themeColorPresetIndex(primary)
	next := ThemeColorPresets[(idx+1)%len(ThemeColorPresets)]
	return next.Primary
}

// ThemeColorPresetName returns the display name for a stored primary color.
func ThemeColorPresetName(primary string) string {
	idx := themeColorPresetIndex(primary)
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

func sanitizeThemePrimary(primary string) string {
	primary = normalizeStoredColor(primary)
	if primary != "" && themeColorPresetIndex(primary) < 0 {
		return ""
	}
	return primary
}

func validateThemePrimaryForStorage(primary string) error {
	if primary == "" {
		return nil
	}
	if themeColorPresetIndex(primary) < 0 {
		return fmt.Errorf("primary %q is not a supported theme preset", primary)
	}
	return nil
}

// SetThemeColors writes the primary color to ~/.cortex/settings.json.
// Secondary is always cleared so the built-in default blue is used.
// Only preset primary colors may be stored; empty clears the override.
func SetThemeColors(primary, secondary string) error {
	var err error
	primary, err = NormalizeHexColor(primary)
	if err != nil {
		return err
	}
	if err := validateThemePrimaryForStorage(primary); err != nil {
		return err
	}
	if _, err := NormalizeHexColor(secondary); err != nil {
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
	delete(theme, "secondary")
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