// Package agents loads local sub-agent definitions from layered
// .cortex/agents/*.md files (same layout as Cursor/Claude Code skills).
package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Agent is a local sub-agent definition parsed from an agents/*.md file.
type Agent struct {
	Name         string
	Model        string
	Tools        []string
	MaxTurns     int
	SystemPrompt string
	Source       string // file path the definition was loaded from
}

// roleAliases maps spawn_agent role strings to agent definition names.
var roleAliases = map[string]string{
	"developer":   "implementer",
	"dev":         "implementer",
	"implementer": "implementer",
	"explore":     "explore",
	"explorer":    "explore",
	"reviewer":    "reviewer",
	"tester":      "solver",
	"researcher":  "explore",
	"planner":     "plan",
	"plan":        "plan",
	"general":     "general",
	"solver":      "solver",
}

// LoadFromDirs scans agent markdown files from dirs in order. Later
// directories override earlier ones for the same agent name.
func LoadFromDirs(dirs []string) (map[string]Agent, error) {
	out := map[string]Agent{}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("agents: read %s: %w", dir, err)
		}
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, ent.Name())
			ag, err := parseAgentFile(path)
			if err != nil {
				return nil, err
			}
			if ag.Name == "" {
				ag.Name = strings.TrimSuffix(ent.Name(), ".md")
			}
			out[ag.Name] = ag
		}
	}
	return out, nil
}

// Resolve picks an agent for a spawn_agent role string. Falls back to
// general, then the first alias target that exists.
func Resolve(role string, catalog map[string]Agent) (Agent, bool) {
	if len(catalog) == 0 {
		return Agent{}, false
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = "developer"
	}
	if name, ok := roleAliases[role]; ok {
		role = name
	}
	if ag, ok := catalog[role]; ok {
		return ag, true
	}
	if ag, ok := catalog["general"]; ok {
		return ag, true
	}
	for _, ag := range catalog {
		return ag, true
	}
	return Agent{}, false
}

// ParseAgentContent parses agent markdown (frontmatter + body).
func ParseAgentContent(raw, source string) (Agent, error) {
	meta, body, err := splitFrontmatter(raw)
	if err != nil {
		return Agent{}, fmt.Errorf("agents: %s: %w", source, err)
	}
	var fm struct {
		Name     string `yaml:"name"`
		Model    string `yaml:"model"`
		Tools    string `yaml:"tools"`
		MaxTurns int    `yaml:"max_turns"`
	}
	if err := yaml.Unmarshal([]byte(meta), &fm); err != nil {
		return Agent{}, fmt.Errorf("agents: %s frontmatter: %w", source, err)
	}
	tools := splitCSV(fm.Tools)
	if fm.MaxTurns <= 0 {
		fm.MaxTurns = 25
	}
	return Agent{
		Name:         fm.Name,
		Model:        fm.Model,
		Tools:        tools,
		MaxTurns:     fm.MaxTurns,
		SystemPrompt: strings.TrimSpace(body),
		Source:       source,
	}, nil
}

func parseAgentFile(path string) (Agent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Agent{}, fmt.Errorf("agents: read %s: %w", path, err)
	}
	return ParseAgentContent(string(data), path)
}

func splitFrontmatter(raw string) (meta, body string, err error) {
	raw = strings.TrimPrefix(raw, "\ufeff")
	if !strings.HasPrefix(raw, "---") {
		return "", strings.TrimSpace(raw), nil
	}
	rest := strings.TrimPrefix(raw, "---")
	rest = strings.TrimLeft(rest, " \t\r\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", fmt.Errorf("unclosed frontmatter")
	}
	meta = strings.TrimSpace(rest[:end])
	body = strings.TrimSpace(rest[end+len("\n---"):])
	body = strings.TrimLeft(body, " \t\r\n")
	return meta, body, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// FormatSystemPrompt substitutes runtime variables in an agent prompt.
func FormatSystemPrompt(prompt, workdir string) string {
	prompt = strings.ReplaceAll(prompt, "$(working_directory)", workdir)
	return prompt
}