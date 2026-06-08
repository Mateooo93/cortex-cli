package config

import "path/filepath"

// CortexPaths resolves all .cortex-relative filesystem paths for a session.
//
// When Override is set, every path resolves under the override directory and
// neither ~/.cortex nor cwd/.cortex is consulted. This enables fully
// isolated runs that ignore the user's and project's real configuration.
//
// When Override is empty (normal mode), Layers() returns [home, cwd/.cortex]
// so callers can merge home-level defaults with project-level overrides.
type CortexPaths struct {
	override string
	home     string
	cwd      string
}

// NewCortexPaths constructs a resolver. override may be empty (normal mode).
// home should be the result of HomeCortexDir() (may be empty if UserHomeDir fails).
// cwd is the session working directory.
func NewCortexPaths(override, home, cwd string) CortexPaths {
	return CortexPaths{override: override, home: home, cwd: cwd}
}

// Override returns the override directory, or "" if not set.
func (p CortexPaths) Override() string { return p.override }

// IsOverride reports whether the session is running in config-dir override mode.
func (p CortexPaths) IsOverride() bool { return p.override != "" }

// Home returns the home .cortex directory. Empty in override mode or if unavailable.
func (p CortexPaths) Home() string {
	if p.override != "" {
		return ""
	}
	return p.home
}

// Project returns the project-level .cortex directory. Empty in override mode.
func (p CortexPaths) Project() string {
	if p.override != "" {
		return ""
	}
	return filepath.Join(p.cwd, ".cortex")
}

// Layers returns the ordered list of .cortex root directories to read from.
// Override mode: [override]
// Normal mode:   [home, cwd/.cortex] (home first, later entries override earlier)
// Empty entries (e.g. unavailable home) are filtered out.
func (p CortexPaths) Layers() []string {
	if p.override != "" {
		return []string{p.override}
	}
	var out []string
	if p.home != "" {
		out = append(out, p.home)
	}
	out = append(out, filepath.Join(p.cwd, ".cortex"))
	return out
}

// UserThemeSettings returns the settings.json path for theme colors.
// Theme is a user preference: home settings in normal mode, or the override
// directory with --config-dir. Project .cortex never contributes theme.
func (p CortexPaths) UserThemeSettings() string {
	if p.override != "" {
		return filepath.Join(p.override, "settings.json")
	}
	if p.home == "" {
		return ""
	}
	return filepath.Join(p.home, "settings.json")
}

// Settings returns the settings.json paths to merge, in load order.
func (p CortexPaths) Settings() []string {
	layers := p.Layers()
	out := make([]string, len(layers))
	for i, d := range layers {
		out[i] = filepath.Join(d, "settings.json")
	}
	return out
}

// Agents returns the agents/ directories to scan, in load order (later wins).
func (p CortexPaths) Agents() []string {
	return p.subdirs("agents")
}

// Skills returns the skills/ directories to scan, in load order.
func (p CortexPaths) Skills() []string {
	return p.subdirs("skills")
}

// Plugins returns the plugins/ directories to scan, in load order.
func (p CortexPaths) Plugins() []string { return p.subdirs("plugins") }

// ClaudeMD returns the CLAUDE.md paths to load, in order.
// Normal mode also includes the project root CLAUDE.md (outside .cortex).
func (p CortexPaths) ClaudeMD() []string {
	if p.override != "" {
		return []string{filepath.Join(p.override, "CLAUDE.md")}
	}
	var out []string
	if p.home != "" {
		out = append(out, filepath.Join(p.home, "CLAUDE.md"))
	}
	out = append(out, filepath.Join(p.cwd, "CLAUDE.md"))
	return out
}

// Primary returns the write target for session-scoped state (history, plans,
// access stats when override is set, etc.). Override mode: override.
// Normal mode: cwd/.cortex.
func (p CortexPaths) Primary() string {
	if p.override != "" {
		return p.override
	}
	return filepath.Join(p.cwd, ".cortex")
}

// Logs returns where LLM logs should be written for this session.
// Override mode: override/logs. Normal mode: home/logs (or "" if home empty).
func (p CortexPaths) Logs() string {
	if p.override != "" {
		return filepath.Join(p.override, "logs")
	}
	if p.home == "" {
		return ""
	}
	return filepath.Join(p.home, "logs")
}

// AccessStatsDB returns the sqlite path for per-session tool access stats.
// Override mode: override/access_stats.db.
// Normal mode:   cwd/.cortex/access_stats.db.
func (p CortexPaths) AccessStatsDB() string {
	return filepath.Join(p.Primary(), "access_stats.db")
}

// History returns the TUI input history file path.
func (p CortexPaths) History() string {
	return filepath.Join(p.Primary(), "history.txt")
}

// Plans returns the plans/ directory path.
func (p CortexPaths) Plans() string {
	return filepath.Join(p.Primary(), "plans")
}

// Brain returns the brain index directory.
// Override mode: override (brain lives directly in the override root).
// Normal mode:   cwd/.cortex.
func (p CortexPaths) Brain() string {
	return p.Primary()
}

// MemoryDB returns the project-scoped persistent memory database.
func (p CortexPaths) MemoryDB() string {
	return filepath.Join(p.Primary(), "memory.db")
}

// ContextMD returns the human-readable project context summary.
func (p CortexPaths) ContextMD() string {
	return filepath.Join(p.Primary(), "context.md")
}

// MemoryMetadata returns memory subsystem metadata (version, counts).
func (p CortexPaths) MemoryMetadata() string {
	return filepath.Join(p.Primary(), "metadata.json")
}

// ProjectSettingsWrite returns the settings.json path to use for persisting
// project-level edits (e.g. appending allowed directories). Override mode
// writes to the override dir; normal mode writes to cwd/.cortex.
func (p CortexPaths) ProjectSettingsWrite() string {
	return filepath.Join(p.Primary(), "settings.json")
}

func (p CortexPaths) subdirs(name string) []string {
	layers := p.Layers()
	out := make([]string, len(layers))
	for i, d := range layers {
		out[i] = filepath.Join(d, name)
	}
	return out
}
