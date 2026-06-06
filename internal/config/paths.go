package config

import "path/filepath"

// VixPaths resolves all .vix-relative filesystem paths for a session.
//
// When Override is set, every path resolves under the override directory and
// neither ~/.vix nor cwd/.vix is consulted. This enables fully isolated runs
// that ignore the user's and project's real configuration.
//
// When Override is empty (normal mode), Layers() returns [home, cwd/.vix] so
// callers can merge home-level defaults with project-level overrides.
type VixPaths struct {
	override string
	home     string
	cwd      string
}

// NewVixPaths constructs a resolver. override may be empty (normal mode).
// home should be the result of HomeVixDir() (may be empty if UserHomeDir fails).
// cwd is the session working directory.
func NewVixPaths(override, home, cwd string) VixPaths {
	return VixPaths{override: override, home: home, cwd: cwd}
}

// Override returns the override directory, or "" if not set.
func (p VixPaths) Override() string { return p.override }

// IsOverride reports whether the session is running in config-dir override mode.
func (p VixPaths) IsOverride() bool { return p.override != "" }

// Home returns the home .vix directory. Empty in override mode or if unavailable.
func (p VixPaths) Home() string {
	if p.override != "" {
		return ""
	}
	return p.home
}

// Project returns the project-level .vix directory. Empty in override mode.
func (p VixPaths) Project() string {
	if p.override != "" {
		return ""
	}
	return filepath.Join(p.cwd, ".vix")
}

// Layers returns the ordered list of .vix root directories to read from.
// Override mode: [override]
// Normal mode:   [home, cwd/.vix] (home first, later entries override earlier)
// Empty entries (e.g. unavailable home) are filtered out.
func (p VixPaths) Layers() []string {
	if p.override != "" {
		return []string{p.override}
	}
	var out []string
	if p.home != "" {
		out = append(out, p.home)
	}
	out = append(out, filepath.Join(p.cwd, ".vix"))
	return out
}

// Settings returns the settings.json paths to merge, in load order.
func (p VixPaths) Settings() []string {
	layers := p.Layers()
	out := make([]string, len(layers))
	for i, d := range layers {
		out[i] = filepath.Join(d, "settings.json")
	}
	return out
}

// Agents returns the agents/ directories to scan, in load order (later wins).
func (p VixPaths) Agents() []string {
	return p.subdirs("agents")
}

// Skills returns the skills/ directories to scan, in load order.
func (p VixPaths) Skills() []string {
	return p.subdirs("skills")
}

// Plugins returns the plugins/ directories to scan, in load order.
func (p VixPaths) Plugins() []string { return p.subdirs("plugins") }

// ClaudeMD returns the CLAUDE.md paths to load, in order.
// Normal mode also includes the project root CLAUDE.md (outside .vix).
func (p VixPaths) ClaudeMD() []string {
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
// Normal mode: cwd/.vix.
func (p VixPaths) Primary() string {
	if p.override != "" {
		return p.override
	}
	return filepath.Join(p.cwd, ".vix")
}

// Logs returns where LLM logs should be written for this session.
// Override mode: override/logs. Normal mode: home/logs (or "" if home empty).
func (p VixPaths) Logs() string {
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
// Normal mode:   cwd/.vix/access_stats.db.
func (p VixPaths) AccessStatsDB() string {
	return filepath.Join(p.Primary(), "access_stats.db")
}

// History returns the TUI input history file path.
func (p VixPaths) History() string {
	return filepath.Join(p.Primary(), "history.txt")
}

// Plans returns the plans/ directory path.
func (p VixPaths) Plans() string {
	return filepath.Join(p.Primary(), "plans")
}

// Brain returns the brain index directory.
// Override mode: override (brain lives directly in the override root).
// Normal mode:   cwd/.vix.
func (p VixPaths) Brain() string {
	return p.Primary()
}

// ProjectSettingsWrite returns the settings.json path to use for persisting
// project-level edits (e.g. appending allowed directories). Override mode
// writes to the override dir; normal mode writes to cwd/.vix.
func (p VixPaths) ProjectSettingsWrite() string {
	return filepath.Join(p.Primary(), "settings.json")
}

func (p VixPaths) subdirs(name string) []string {
	layers := p.Layers()
	out := make([]string, len(layers))
	for i, d := range layers {
		out[i] = filepath.Join(d, name)
	}
	return out
}
