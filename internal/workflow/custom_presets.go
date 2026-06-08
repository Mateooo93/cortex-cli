package workflow

import (
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// CustomPreset is a user-defined workflow preset from the cortex config.
// It matches the shape of BuiltinPresets but comes from user configuration.
type CustomPreset struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Strategy    string   `json:"strategy" yaml:"strategy"`
	MaxAgents   int      `json:"maxAgents" yaml:"maxAgents"`
	Roles       []string `json:"roles" yaml:"roles"`
}

// LoadCustomPresets reads user-defined workflow presets from the
// cortex config. If the config has no custom presets, the built-in
// set is returned unchanged.
func LoadCustomPresets(cfg *cortexconfig.Config) []Preset {
	builtins := BuiltinPresets
	if cfg == nil {
		return builtins
	}

	customs := cfg.WorkflowPresetsConfig()
	if len(customs) == 0 {
		return builtins
	}

	// Merge: custom presets override builtins with the same name,
	// and new presets are appended.
	merged := make([]Preset, 0, len(builtins)+len(customs))
	seen := map[string]bool{}

	for _, cp := range customs {
		seen[cp.Name] = true
		merged = append(merged, Preset{
			Name:        cp.Name,
			Description: cp.Description,
			Strategy:    cp.Strategy,
			MaxAgents:   cp.MaxAgents,
			Roles:       cp.Roles,
		})
	}

	for _, bp := range builtins {
		if !seen[bp.Name] {
			merged = append(merged, bp)
		}
	}

	return merged
}

// AllPresets returns the merged built-in + custom presets.
func AllPresets(cfg *cortexconfig.Config) []Preset {
	return LoadCustomPresets(cfg)
}

// FindPreset looks up a preset by name in the merged set.
func FindPreset(name string, cfg *cortexconfig.Config) *Preset {
	for _, p := range AllPresets(cfg) {
		if p.Name == name {
			return &p
		}
	}
	return nil
}
