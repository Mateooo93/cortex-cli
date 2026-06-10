// Package contextbreakdown estimates how the model context window is used.
package contextbreakdown

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mateooo93/cortex-cli/internal/agents"
	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/session"
	"github.com/Mateooo93/cortex-cli/internal/tools"
)

// Child is a named sub-item (e.g. one skill or agent file).
type Child struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
}

// Item is one top-level breakdown row.
type Item struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Tokens   int     `json:"tokens"`
	Detail   string  `json:"detail,omitempty"`
	Children []Child `json:"children,omitempty"`
}

// HistoryMessage is a prior chat turn for conversation token estimates.
type HistoryMessage struct {
	Role    string
	Content string
}

// Input drives a breakdown computation.
type Input struct {
	Workdir       string
	Paths         config.CortexPaths
	Config        *cortexconfig.Config
	UsedTokens    int
	MaxTokens     int
	ProjectMemory bool
	History       []HistoryMessage
}

// Result is the structured context breakdown returned to clients.
type Result struct {
	Used  int    `json:"used"`
	Max   int    `json:"max"`
	Items []Item `json:"items"`
}

// EstimateTokens approximates token count with a len/4 heuristic.
func EstimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	n := len(text) / 4
	if n < 1 {
		return 1
	}
	return n
}

// Compute builds a context breakdown from session configuration and history.
func Compute(in Input) Result {
	workdir := strings.TrimSpace(in.Workdir)
	paths := in.Paths
	if workdir != "" {
		paths = config.NewCortexPaths("", config.HomeCortexDir(), workdir)
	}

	var items []Item

	basePrompt := session.BuildSystemPrompt(workdir)
	if tokens := EstimateTokens(basePrompt); tokens > 0 {
		items = append(items, Item{
			ID:     "system",
			Label:  "System prompt",
			Tokens: tokens,
			Detail: "Default cortex-cli instructions and working-directory context",
		})
	}

	if in.Config != nil {
		custom := strings.TrimSpace(in.Config.SystemPrompt)
		if tokens := EstimateTokens(custom); tokens > 0 {
			items = append(items, Item{
				ID:     "custom-prompt",
				Label:  "Custom system prompt",
				Tokens: tokens,
				Detail: "User-defined system prompt from config",
			})
		}
	}

	toolsPrompt := tools.NewRegistry().ToSystemPrompt()
	if tokens := EstimateTokens(toolsPrompt); tokens > 0 {
		items = append(items, Item{
			ID:     "tools",
			Label:  "Tools",
			Tokens: tokens,
			Detail: "Tool definitions injected into the system message",
		})
	}

	if skillItem := scanSkills(paths); skillItem.Tokens > 0 {
		items = append(items, skillItem)
	}

	if agentItem := scanAgents(paths); agentItem.Tokens > 0 {
		items = append(items, agentItem)
	}

	if agentsMDItem := scanAgentsMD(paths, workdir); agentsMDItem.Tokens > 0 {
		items = append(items, agentsMDItem)
	}

	if in.ProjectMemory {
		if memItem := scanMemory(paths); memItem.Tokens > 0 {
			items = append(items, memItem)
		}
	}

	convTokens := estimateConversation(in.History)
	staticTotal := sumTokens(items)

	used := in.UsedTokens
	if used <= 0 {
		used = staticTotal + convTokens
	} else if used > staticTotal {
		convTokens = used - staticTotal
	}

	if convTokens > 0 || len(in.History) > 0 {
		detail := "Messages in the current conversation"
		if len(in.History) > 0 {
			detail = formatHistoryDetail(len(in.History))
		}
		items = append(items, Item{
			ID:     "conversation",
			Label:  "Conversation",
			Tokens: convTokens,
			Detail: detail,
		})
	}

	if used <= 0 {
		used = sumTokens(items)
	}

	max := in.MaxTokens
	if max <= 0 {
		max = used
	}

	return Result{
		Used:  used,
		Max:   max,
		Items: items,
	}
}

func sumTokens(items []Item) int {
	total := 0
	for _, item := range items {
		total += item.Tokens
	}
	return total
}

func estimateConversation(history []HistoryMessage) int {
	total := 0
	for _, m := range history {
		role := strings.TrimSpace(m.Role)
		content := strings.TrimSpace(m.Content)
		if content == "" || (role != "user" && role != "assistant") {
			continue
		}
		// Small per-message overhead for role markers.
		total += EstimateTokens(content) + 4
	}
	return total
}

func formatHistoryDetail(count int) string {
	if count == 1 {
		return "1 message in the current conversation"
	}
	return fmt.Sprintf("%d messages in the current conversation", count)
}

func scanSkills(paths config.CortexPaths) Item {
	var children []Child
	seen := map[string]bool{}

	for _, dir := range paths.Skills() {
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".md") {
				return nil
			}
			rel, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				rel = name
			}
			if seen[rel] {
				return nil
			}
			seen[rel] = true
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			tokens := EstimateTokens(string(data))
			if tokens == 0 {
				return nil
			}
			label := strings.TrimSuffix(name, filepath.Ext(name))
			if name == "SKILL.md" {
				label = filepath.Base(filepath.Dir(path))
			}
			children = append(children, Child{Name: label, Tokens: tokens})
			return nil
		})
	}

	if len(children) == 0 {
		return Item{}
	}

	total := 0
	for _, c := range children {
		total += c.Tokens
	}
	return Item{
		ID:       "skills",
		Label:    "Skills",
		Tokens:   total,
		Detail:   "Skill definitions from .cortex/skills",
		Children: children,
	}
}

func scanAgents(paths config.CortexPaths) Item {
	catalog, err := agents.LoadFromDirs(paths.Agents())
	if err != nil || len(catalog) == 0 {
		return Item{}
	}

	var children []Child
	total := 0
	for name, ag := range catalog {
		tokens := EstimateTokens(ag.SystemPrompt)
		if tokens == 0 {
			continue
		}
		children = append(children, Child{Name: name, Tokens: tokens})
		total += tokens
	}
	if len(children) == 0 {
		return Item{}
	}
	return Item{
		ID:       "agents",
		Label:    "Agents",
		Tokens:   total,
		Detail:   "Sub-agent definitions from .cortex/agents",
		Children: children,
	}
}

func scanAgentsMD(paths config.CortexPaths, workdir string) Item {
	seen := map[string]bool{}
	var files []string

	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			return
		}
		if _, err := os.Stat(path); err != nil {
			return
		}
		seen[path] = true
		files = append(files, path)
	}

	if workdir != "" {
		add(filepath.Join(workdir, "AGENTS.md"))
	}
	for _, layer := range paths.Layers() {
		add(filepath.Join(layer, "AGENTS.md"))
	}

	if len(files) == 0 {
		return Item{}
	}

	total := 0
	var children []Child
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		tokens := EstimateTokens(string(data))
		if tokens == 0 {
			continue
		}
		total += tokens
		name := filepath.Base(path)
		if dir := filepath.Dir(path); dir != "." {
			name = filepath.Base(dir) + "/" + name
		}
		children = append(children, Child{Name: name, Tokens: tokens})
	}
	if total == 0 {
		return Item{}
	}
	return Item{
		ID:       "agents-md",
		Label:    "AGENTS.md",
		Tokens:   total,
		Detail:   "Project agent guidance files",
		Children: children,
	}
}

func scanMemory(paths config.CortexPaths) Item {
	path := paths.ContextMD()
	data, err := os.ReadFile(path)
	if err != nil {
		return Item{}
	}
	tokens := EstimateTokens(string(data))
	if tokens == 0 {
		return Item{}
	}
	return Item{
		ID:     "memory",
		Label:  "Project memory",
		Tokens: tokens,
		Detail: "context.md summary injected when project memory is enabled",
	}
}
