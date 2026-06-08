package workflow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	llmprovider "github.com/Mateooo93/cortex-cli/internal/provider"
)

// ToolRunner executes tools on behalf of workflow agents. The engine
// holds a reference to a ToolRunner; when callRole receives a tool_calls
// response, it dispatches to the runner and feeds results back.
//
// The default runner (OSRunner) provides filesystem and shell access,
// which covers the common workflow roles (developer, tester, fixer).
// Callers can supply a custom runner for sandboxed or remote execution.
type ToolRunner interface {
	// Run executes a named tool with the given arguments and returns
	// the result string. If the tool is not known, it returns an
	// error message (the model uses this to self-correct).
	Run(ctx context.Context, name string, args map[string]any) string
	// Tools returns the tool definitions for the LLM provider layer.
	Tools() []llmprovider.Tool
}

// OSRunner is the default ToolRunner that gives workflow agents
// filesystem and shell access. It's bound to a working directory.
type OSRunner struct {
	workdir  string
	allowShell bool // shell commands allowed?
}

// NewOSRunner creates a tool runner bound to workdir. If workdir is
// empty, the current working directory is used.
func NewOSRunner(workdir string, allowShell bool) *OSRunner {
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	return &OSRunner{workdir: workdir, allowShell: allowShell}
}

func (r *OSRunner) Tools() []llmprovider.Tool {
	tools := []llmprovider.Tool{
		{
			Name:        "read_file",
			Description: "Read the contents of a file. Returns the file content as text.",
			Parameters: map[string]llmprovider.ToolParam{
				"path": {Type: "string", Description: "Path to the file to read (relative or absolute)", Required: true},
			},
		},
		{
			Name:        "write_file",
			Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does.",
			Parameters: map[string]llmprovider.ToolParam{
				"path":    {Type: "string", Description: "Path to the file to write", Required: true},
				"content": {Type: "string", Description: "Content to write to the file", Required: true},
			},
		},
		{
			Name:        "list_files",
			Description: "List files in a directory. Returns file names, one per line.",
			Parameters: map[string]llmprovider.ToolParam{
				"path": {Type: "string", Description: "Directory to list (default: current working directory)", Required: false},
			},
		},
	}
	if r.allowShell {
		tools = append(tools, llmprovider.Tool{
			Name:        "run_shell",
			Description: "Run a shell command and return its stdout+stderr. Max 30 second timeout.",
			Parameters: map[string]llmprovider.ToolParam{
				"command": {Type: "string", Description: "Shell command to execute", Required: true},
			},
		})
	}
	return tools
}

func (r *OSRunner) Run(ctx context.Context, name string, args map[string]any) string {
	switch name {
	case "read_file":
		return r.readFile(args)
	case "write_file":
		return r.writeFile(args)
	case "list_files":
		return r.listFiles(args)
	case "run_shell":
		return r.runShell(ctx, args)
	default:
		return fmt.Sprintf("Error: unknown tool %q. Available tools: read_file, write_file, list_files%s",
			name, map[bool]string{true: ", run_shell", false: ""}[r.allowShell])
	}
}

func (r *OSRunner) readFile(args map[string]any) string {
	path, _ := args["path"].(string)
	if path == "" {
		return "Error: path is required"
	}
	if !strings.HasPrefix(path, "/") {
		path = r.workdir + "/" + path
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error reading %s: %v", path, err)
	}
	// Truncate to avoid blowing up context
	content := string(data)
	if len(content) > 32000 {
		content = content[:32000] + "\n... (truncated)"
	}
	return content
}

func (r *OSRunner) writeFile(args map[string]any) string {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return "Error: path is required"
	}
	if !strings.HasPrefix(path, "/") {
		path = r.workdir + "/" + path
	}
	// Ensure parent directory exists
	if idx := strings.LastIndex(path, "/"); idx > 0 {
		os.MkdirAll(path[:idx], 0755)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Sprintf("Error writing %s: %v", path, err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(content), path)
}

func (r *OSRunner) listFiles(args map[string]any) string {
	path, _ := args["path"].(string)
	if path == "" {
		path = r.workdir
	}
	if !strings.HasPrefix(path, "/") {
		path = r.workdir + "/" + path
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Sprintf("Error listing %s: %v", path, err)
	}
	var lines []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	if len(lines) == 0 {
		return "(empty directory)"
	}
	// Limit output
	if len(lines) > 200 {
		remaining := len(lines) - 200
		lines = lines[:200]
		lines = append(lines, fmt.Sprintf("... (%d more entries)", remaining))
	}
	return strings.Join(lines, "\n")
}

func (r *OSRunner) runShell(ctx context.Context, args map[string]any) string {
	if !r.allowShell {
		return "Error: shell access is disabled for this workflow agent"
	}
	command, _ := args["command"].(string)
	if command == "" {
		return "Error: command is required"
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "sh", "-c", command)
	cmd.Dir = r.workdir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Command failed (exit: %v):\n%s", err, string(output))
	}
	result := string(output)
	if len(result) > 16000 {
		result = result[:16000] + "\n... (truncated)"
	}
	if result == "" {
		result = "(no output)"
	}
	return result
}

// maxToolIterations caps the number of tool-calling rounds per step
// to prevent infinite loops.
const maxToolIterations = 10
