package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// Config holds client configuration.
type Config struct {
	Model      string
	CWD        string
	Workdir    string
	ConfigDir  string // absolute path, or "" for default ~/.cortex + ./.cortex behavior
	Paths      CortexPaths
	ForceInit  bool
	SocketPath string
}

// Load reads configuration from environment variables.
// The API key is no longer needed on the client side — the daemon handles it.
// If workdir is non-empty, it is resolved to an absolute path and used as the
// session working directory instead of os.Getwd().
// If configDir is non-empty, it is resolved to an absolute path and used as
// the sole .cortex config root (ignoring ~/.cortex and ./.cortex).
// If socketPath is empty, /tmp/cortexd.sock is used.
func Load(forceInit bool, workdir, configDir, socketPath string) (*Config, error) {
	// Model selection now lives in the active chat agent's `model:` YAML
	// frontmatter (resolved per-session in the daemon). The Config.Model
	// field is left as a final fallback only — see session.go for the
	// resolution chain.
	const model = "anthropic/claude-sonnet-4-6"

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine working directory: %w", err)
	}

	if workdir != "" {
		abs, err := filepath.Abs(workdir)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve workdir %q: %w", workdir, err)
		}
		cwd = abs
	}

	if configDir != "" {
		abs, err := filepath.Abs(configDir)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve config-dir %q: %w", configDir, err)
		}
		configDir = abs
	}

	if socketPath == "" {
		socketPath = "/tmp/cortexd.sock"
	}

	return &Config{
		Model:      model,
		CWD:        cwd,
		Workdir:    workdir,
		ConfigDir:  configDir,
		Paths:      NewCortexPaths(configDir, HomeCortexDir(), cwd),
		ForceInit:  forceInit,
		SocketPath: socketPath,
	}, nil
}

// HomeCortexDir returns the path to ~/.cortex/.
func HomeCortexDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cortex")
}

// DaemonConfig holds daemon-side configuration.
type DaemonConfig struct {
	HomeCortexDir string
	// AuthToken is the shared-secret string the daemon will require on every
	// incoming socket message. Loaded from the file pointed at by the
	// -auth-token-path flag. Empty means "no auth check" — that mode exists
	// for in-process tests and trusted-host embeddings; production
	// deployments always populate it.
	AuthToken string
}

// ToolsConfig holds tool backend configuration.
type ToolsConfig struct {
	Grep ToolBackendConfig `json:"grep"`
	Glob ToolBackendConfig `json:"glob"`
}

// ToolBackendConfig holds a single tool's backend selection.
type ToolBackendConfig struct {
	Backend string `json:"backend"`
}

// LoadDaemonConfig loads daemon configuration with defaults.
func LoadDaemonConfig() (*DaemonConfig, error) {
	homeDir := HomeCortexDir()
	if homeDir != "" {
		os.MkdirAll(homeDir, 0o755)
		if err := BootstrapHomeCortexDir(homeDir); err != nil {
			log.Printf("[config] bootstrap failed: %v", err)
		}
	}

	return &DaemonConfig{
		HomeCortexDir: homeDir,
	}, nil
}

// TelemetryEnabled reads the telemetry feature flag from ~/.cortex/settings.json.
// Returns true if the flag is absent (opt-out model).
func TelemetryEnabled() bool {
	p := filepath.Join(HomeCortexDir(), "settings.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return true // default: enabled
	}
	var cfg struct {
		Features map[string]bool `json:"features"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return true
	}
	if v, ok := cfg.Features["telemetry"]; ok {
		return v
	}
	return true
}

// ShowThinking reads the show_thinking feature flag from ~/.cortex/settings.json.
// Returns false if the flag is absent (opt-in: thinking is hidden by default).
func ShowThinking() bool {
	p := filepath.Join(HomeCortexDir(), "settings.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	var cfg struct {
		Features map[string]bool `json:"features"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}
	return cfg.Features["show_thinking"]
}

// SetShowThinking writes the show_thinking feature flag to ~/.cortex/settings.json,
// preserving other top-level keys (theme, other features, etc).
func SetShowThinking(v bool) error {
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

	features, _ := raw["features"].(map[string]any)
	if features == nil {
		features = map[string]any{}
	}
	features["show_thinking"] = v
	raw["features"] = features

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, out, 0o644)
}

// ThemeConfig holds user-configurable brand colors.
type ThemeConfig struct {
	Primary   string `json:"primary"`   // hex color like "#3B82F6"
	Secondary string `json:"secondary"` // hex color like "#A3FC63"
}

// ElevenLabsAgentID reads the elevenlabs.agent_id from the layered settings
// files (home then project, last non-empty wins). Falls back to the built-in
// default if no value is configured.
func ElevenLabsAgentID(paths CortexPaths) string {
	const defaultID = "agent_7501kqrztj1te17ssqz5wqpnvkf3"
	result := defaultID
	for _, p := range paths.Settings() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var cfg struct {
			ElevenLabs struct {
				AgentID string `json:"agent_id"`
			} `json:"elevenlabs"`
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if cfg.ElevenLabs.AgentID != "" {
			result = cfg.ElevenLabs.AgentID
		}
	}
	return result
}

// ElevenLabsAuthMode reads the elevenlabs.auth_mode from the layered settings
// files (home then project, last non-empty wins). Returns "public" by default.
// Set to "signed_url" to require a server-side ELEVENLABS_API_KEY instead.
func ElevenLabsAuthMode(paths CortexPaths) string {
	result := "public"
	for _, p := range paths.Settings() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var cfg struct {
			ElevenLabs struct {
				AuthMode string `json:"auth_mode"`
			} `json:"elevenlabs"`
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if cfg.ElevenLabs.AuthMode != "" {
			result = cfg.ElevenLabs.AuthMode
		}
	}
	return result
}

// LoadThemeConfig reads theme colors from settings.json files in the order
// returned by paths.Settings() — home then project in normal mode, or just
// the override in config-dir mode.
func LoadThemeConfig(paths CortexPaths) ThemeConfig {
	var tc ThemeConfig

	for _, p := range paths.Settings() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var wrapper struct {
			Theme ThemeConfig `json:"theme"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			log.Printf("[config] failed to parse theme from %s: %v", p, err)
			continue
		}
		if wrapper.Theme.Primary != "" {
			tc.Primary = wrapper.Theme.Primary
		}
		if wrapper.Theme.Secondary != "" {
			tc.Secondary = wrapper.Theme.Secondary
		}
	}

	return tc
}
