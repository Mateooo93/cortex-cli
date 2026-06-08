package config

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/Mateooo93/cortex-cli/internal/agents"
)

// DefaultAgents returns the built-in agent catalog shipped with cortex-cli.
// User/project agents in ~/.cortex/agents and ./.cortex/agents override these.
func DefaultAgents() (map[string]agents.Agent, error) {
	out := map[string]agents.Agent{}
	err := fs.WalkDir(defaultFiles, "defaults/agents", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		data, err := defaultFiles.ReadFile(path)
		if err != nil {
			return err
		}
		ag, err := agents.ParseAgentContent(string(data), path)
		if err != nil {
			return err
		}
		if ag.Name == "" {
			ag.Name = strings.TrimSuffix(d.Name(), ".md")
		}
		out[ag.Name] = ag
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Normalize source paths for display (embed paths are not real files).
	for name, ag := range out {
		ag.Source = filepath.Join("defaults", "agents", name+".md")
		out[name] = ag
	}
	return out, nil
}