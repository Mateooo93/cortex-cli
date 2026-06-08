// Package tools implements the agent's tool set. Each tool is a small
// function the LLM can invoke. The schema is converted into both a
// human-readable description (for system prompts) and OpenAI-style
// function-calling schema (for providers that support native tool use).
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Tool is one callable tool.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]Param
	// Run executes the tool. args is a map of validated parameter values.
	// ctx holds execution context (cwd, permissions).
	Run(ctx Context, args map[string]any) (Result, error)
}

// Param describes a single parameter of a tool.
type Param struct {
	Type        string // "string" | "number" | "boolean" | "array" | "object"
	Description string
	Required    bool
	// Nested schema support (Pi-style tools). Used by
	// edit_file.edits so providers can see a real
	// array-of-objects schema instead of a JSON string.
	Items      *Param
	Properties map[string]Param
}

// Context is the per-call context for a tool.
type Context struct {
	CWD        string
	AllowShell bool
	AllowWrite bool
	AllowGit   bool
}

// Result is the output of a tool execution.
type Result struct {
	OK     bool
	Output string
	Error  string
	// Details holds structured sidecar data for the UI (e.g. diffs,
	// patches, structured info). This is *not* sent to the LLM in the
	// tool result content — only Output/Error are. Ported/adapted from
	// Pi agent mechanics for rich edit and tool feedback.
	Details map[string]any
}

// Registry holds the available tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry constructs a Registry with the default toolset.
func NewRegistry() *Registry {
	r := &Registry{tools: map[string]Tool{}}
	for _, t := range defaultTools() {
		r.tools[t.Name()] = t
	}
	return r
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Names returns all registered tool names.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.tools))
	for n := range r.tools {
		out = append(out, n)
	}
	return out
}

// ToProviderTools converts the registry into a slice of provider.Tool for
// the LLM API.
func (r *Registry) ToProviderTools() []ProviderTool {
	out := make([]ProviderTool, 0, len(r.tools))
	for _, t := range r.tools {
		params := map[string]ParamInfo{}
		required := []string{}
		for k, p := range t.Parameters() {
			params[k] = paramToInfo(p)
			if p.Required {
				required = append(required, k)
			}
		}
		out = append(out, ProviderTool{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: ProviderSchema{
				Type:       "object",
				Properties: params,
				Required:   required,
			},
		})
	}
	return out
}

// ProviderTool and ProviderSchema are provider-agnostic mirrors of the
// provider.Tool shape, used to avoid an import cycle.
type ProviderTool struct {
	Name        string
	Description string
	Parameters  ProviderSchema
}

type ProviderSchema struct {
	Type       string
	Properties map[string]ParamInfo
	Required   []string
}

type ParamInfo struct {
	Type        string
	Description string
	Items       *ParamInfo
	Properties  map[string]ParamInfo
}

func paramToInfo(p Param) ParamInfo {
	info := ParamInfo{Type: p.Type, Description: p.Description}
	if p.Items != nil {
		item := paramToInfo(*p.Items)
		info.Items = &item
	}
	if len(p.Properties) > 0 {
		info.Properties = make(map[string]ParamInfo, len(p.Properties))
		for k, v := range p.Properties {
			info.Properties[k] = paramToInfo(v)
		}
	}
	return info
}

// ToSystemPrompt renders the registry as a markdown block for the system
// prompt. Cortex-style tool_call block format.
func (r *Registry) ToSystemPrompt() string {
	var lines []string
	lines = append(lines, "You have access to the following tools. To use one, respond with a JSON block in this exact format:")
	lines = append(lines, "")
	lines = append(lines, "```tool_call")
	lines = append(lines, `{"name": "<tool_name>", "arguments": { ... }}`)
	lines = append(lines, "```")
	lines = append(lines, "")
	lines = append(lines, "Available tools:")
	for _, t := range r.tools {
		lines = append(lines, fmt.Sprintf("- **%s**: %s", t.Name(), t.Description()))
		var paramDescs []string
		for k, p := range t.Parameters() {
			req := ""
			if p.Required {
				req = ", required"
			}
			paramDescs = append(paramDescs, fmt.Sprintf("%s (%s%s)", k, p.Type, req))
		}
		if len(paramDescs) > 0 {
			lines = append(lines, fmt.Sprintf("  Parameters: %s", strings.Join(paramDescs, ", ")))
		}
	}
	return strings.Join(lines, "\n")
}

// ── Built-in tools ──────────────────────────────────────────────────────

type ReadFileTool struct{}

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Description() string {
	return "Read the contents of a file. Returns up to maxBytes (default 16384)."
}
func (t *ReadFileTool) Parameters() map[string]Param {
	return map[string]Param{
		"path":     {Type: "string", Description: "Absolute or relative path to the file", Required: true},
		"maxBytes": {Type: "number", Description: "Maximum bytes to read (default 16384)"},
	}
}
func (t *ReadFileTool) Run(ctx Context, args map[string]any) (Result, error) {
	p, _ := args["path"].(string)
	if p == "" {
		return Result{OK: false, Error: "path is required"}, nil
	}
	maxBytes := 16384
	if v, ok := args["maxBytes"]; ok {
		if f, ok := v.(float64); ok {
			maxBytes = int(f)
		}
	}
	full, _ := resolvePath(ctx.CWD, p)
	data, err := os.ReadFile(full)
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	if len(data) > maxBytes {
		trunc := append(data[:maxBytes], []byte(fmt.Sprintf("\n... (truncated, %d more chars)", len(data)-maxBytes))...)
		return Result{OK: true, Output: string(trunc)}, nil
	}
	return Result{OK: true, Output: string(data)}, nil
}

type WriteFileTool struct{}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Write content to a file. Overwrites if it exists. Requires allowWrite."
}
func (t *WriteFileTool) Parameters() map[string]Param {
	return map[string]Param{
		"path":    {Type: "string", Description: "Path to write to", Required: true},
		"content": {Type: "string", Description: "Content to write", Required: true},
	}
}
func (t *WriteFileTool) Run(ctx Context, args map[string]any) (Result, error) {
	if !ctx.AllowWrite {
		return Result{OK: false, Error: "File writing is disabled in config."}, nil
	}
	p, _ := args["path"].(string)
	c, _ := args["content"].(string)
	if p == "" || c == "" {
		return Result{OK: false, Error: "path and content are required"}, nil
	}
	// Auto-correct paths that LOOK like an
	// absolute path missing the leading slash
	// (the user-reported bug: the agent wrote
	// "home/ubuntu/foo.py" expecting
	// "/home/ubuntu/foo.py" but the tool
	// created "{cwd}/home/ubuntu/foo.py"
	// instead). The corrected path + a note
	// is returned in the tool result so the
	// model can see what actually happened.
	full, corrected := resolvePath(ctx.CWD, p)
	res := withFileMutationQueue(full, func() Result {
		if err := mkdirAllForFile(full); err != nil {
			return Result{OK: false, Error: err.Error()}
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			return Result{OK: false, Error: err.Error()}
		}
		lineCount := countContentLines(c)
		output := fmt.Sprintf("Wrote %d bytes to %s", len(c), full)
		if corrected {
			output += fmt.Sprintf(" (auto-corrected from %q — you forgot the leading slash on an absolute path; remember to always start absolute paths with /)", p)
		}
		return Result{
			OK:     true,
			Output: output,
			Details: map[string]any{
				"preview": firstNLines(c, 5),
				"lines":   lineCount,
			},
		}
	})
	return res, nil
}

// countContentLines returns the number of logical lines in s.
func countContentLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// firstNLines returns up to n complete lines from s (without a trailing newline).
func firstNLines(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

type EditFileTool struct{}

func (t *EditFileTool) Name() string { return "edit_file" }
func (t *EditFileTool) Description() string {
	return "Edit a file by replacing a small exact string. Put path and oldString before newString in the JSON. Do NOT use this for huge rewrites; use smaller chunks. Requires allowWrite."
}
func (t *EditFileTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {Type: "string", Description: "Path to file. IMPORTANT: include this first. Absolute paths must start with /.", Required: true},
		// Legacy single-edit fields. Kept for
		// provider compatibility.
		"oldString": {Type: "string", Description: "Legacy single edit: small exact string/block to replace. Include before newString.", Required: false},
		"newString": {Type: "string", Description: "Legacy single edit replacement. Keep small; for large changes split into multiple edits.", Required: false},
		// Pi-style multi-edit input. Providers now
		// receive a real nested array schema; parser
		// still accepts a JSON string for backwards
		// compatibility with v0.2.31.
		"edits": {
			Type:        "array",
			Description: "Optional Pi-style array of replacements. Prefer this for multiple small disjoint edits in one file.",
			Required:    false,
			Items: &Param{Type: "object", Properties: map[string]Param{
				"oldText": {Type: "string", Description: "Exact text for one targeted replacement. Must match a unique, non-overlapping region.", Required: true},
				"newText": {Type: "string", Description: "Replacement text for this edit.", Required: true},
			}},
		},
	}
}
func (t *EditFileTool) Run(ctx Context, args map[string]any) (Result, error) {
	if !ctx.AllowWrite {
		return Result{OK: false, Error: "File editing is disabled in config."}, nil
	}
	p, _ := args["path"].(string)
	edits, parseErr := parseEditFileEdits(args)
	if p == "" || parseErr != nil {
		msg := "path and an edit are required. Prefer Pi-style ordered args: path, edits where edits is a JSON array string like [{\"oldText\":\"exact text\",\"newText\":\"replacement\"}]. Legacy path, oldString, newString also works. Keep each oldText small and copied verbatim from read_file output."
		if parseErr != nil {
			msg += " Parse error: " + parseErr.Error()
		}
		return Result{OK: false, Error: msg}, nil
	}
	full, corrected := resolvePath(ctx.CWD, p)
	res := withFileMutationQueue(full, func() Result {
		data, err := os.ReadFile(full)
		if err != nil {
			return Result{OK: false, Error: err.Error()}
		}
		content := string(data)
		repls, err := planEditReplacements(content, edits)
		if err != nil {
			return Result{OK: false, Error: err.Error()}
		}
		newContent := applyEditReplacements(content, repls)
		if err := os.WriteFile(full, []byte(newContent), 0o644); err != nil {
			return Result{OK: false, Error: err.Error()}
		}
		replaced := 0
		fallbacks := []string{}
		for _, r := range repls {
			replaced += r.end - r.start
			if r.mode != "exact" {
				fallbacks = append(fallbacks, r.mode)
			}
		}
		output := fmt.Sprintf("Edited %s (replaced %d block(s), %d chars", full, len(repls), replaced)
		if len(fallbacks) > 0 {
			output += ", fallback=" + strings.Join(fallbacks, ",")
		}
		output += ")"
		if corrected {
			output += fmt.Sprintf(" (auto-corrected from %q)", p)
		}

		// Pi-style rich details: provide diff + unified patch for UI
		// rendering and for SDK consumers. Diff is human-oriented;
		// patch is standard unified diff.
		details := map[string]any{}
		if diff, patch := computeEditDiffAndPatch(content, newContent, full); diff != "" || patch != "" {
			details["diff"] = diff
			details["patch"] = patch
		}
		return Result{OK: true, Output: output, Details: details}
	})
	return res, nil
}

type editReplacementInput struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

type editReplacementPlan struct {
	start   int
	end     int
	newText string
	mode    string
}

func parseEditReplacementJSON(raw string) ([]editReplacementInput, error) {
	var edits []editReplacementInput
	if err := json.Unmarshal([]byte(raw), &edits); err != nil {
		return nil, fmt.Errorf("invalid edits JSON: %w", err)
	}
	if len(edits) == 0 {
		return nil, fmt.Errorf("edits must contain at least one replacement")
	}
	for i, e := range edits {
		if e.OldText == "" {
			return nil, fmt.Errorf("edits[%d].oldText is required", i)
		}
	}
	return edits, nil
}

func parseEditFileEdits(args map[string]any) ([]editReplacementInput, error) {
	if raw, ok := args["edits"].(string); ok && strings.TrimSpace(raw) != "" {
		return parseEditReplacementJSON(raw)
	}
	if arr, ok := args["edits"].([]any); ok {
		buf, err := json.Marshal(arr)
		if err != nil {
			return nil, fmt.Errorf("invalid edits array: %w", err)
		}
		return parseEditReplacementJSON(string(buf))
	}
	if arr, ok := args["edits"].([]map[string]any); ok {
		buf, err := json.Marshal(arr)
		if err != nil {
			return nil, fmt.Errorf("invalid edits array: %w", err)
		}
		return parseEditReplacementJSON(string(buf))
	}
	oldStr, _ := args["oldString"].(string)
	newStr, _ := args["newString"].(string)
	if oldStr == "" {
		return nil, fmt.Errorf("oldString or edits is required")
	}
	return []editReplacementInput{{OldText: oldStr, NewText: newStr}}, nil
}

func planEditReplacements(content string, edits []editReplacementInput) ([]editReplacementPlan, error) {
	plans := make([]editReplacementPlan, 0, len(edits))
	for i, edit := range edits {
		start, end, mode, ok := findReplacementRange(content, edit.OldText)
		if !ok {
			return nil, fmt.Errorf("edit %d: oldText not found in file. Retry by first calling read_file, then use a smaller exact oldText copied verbatim. If this is a large rewrite, split it into multiple small edit_file calls or write a skeleton then patch sections", i)
		}
		for _, prev := range plans {
			if start < prev.end && end > prev.start {
				return nil, fmt.Errorf("edit %d overlaps another edit. Merge nearby edits into one replacement or use separate non-overlapping oldText blocks", i)
			}
		}
		plans = append(plans, editReplacementPlan{start: start, end: end, newText: edit.NewText, mode: mode})
	}
	// Sort descending so byte offsets remain valid as we apply.
	sort.Slice(plans, func(i, j int) bool { return plans[i].start > plans[j].start })
	return plans, nil
}

func applyEditReplacements(content string, plans []editReplacementPlan) string {
	out := content
	for _, p := range plans {
		out = out[:p.start] + p.newText + out[p.end:]
	}
	return out
}

// findReplacementRange returns the byte range in
// content to replace for oldStr. It is exact-first,
// then applies a few safe fallbacks for common LLM
// mistakes. Fallbacks are conservative and require a
// unique match to avoid editing the wrong location.
func findReplacementRange(content, oldStr string) (start, end int, mode string, ok bool) {
	if oldStr == "" {
		return 0, 0, "", false
	}
	if idx := strings.Index(content, oldStr); idx >= 0 {
		return idx, idx + len(oldStr), "exact", true
	}
	// Normalize line endings. If this matches, replace
	// the normalized oldStr length in the original LF
	// content. This is safe for the common CRLF/LF
	// mismatch case.
	normOld := strings.ReplaceAll(oldStr, "\r\n", "\n")
	normOld = strings.ReplaceAll(normOld, "\r", "\n")
	if normOld != oldStr {
		if idx := strings.Index(content, normOld); idx >= 0 {
			return idx, idx + len(normOld), "line-ending-normalized", true
		}
	}
	trimmed := strings.TrimSpace(normOld)
	if trimmed != "" && trimmed != normOld {
		if idx := strings.Index(content, trimmed); idx >= 0 {
			return idx, idx + len(trimmed), "trimmed", true
		}
	}
	// Indentation-insensitive block match: compare
	// trimmed lines, require one unique window.
	oldLinesRaw := strings.Split(trimmed, "\n")
	var oldLines []string
	for _, l := range oldLinesRaw {
		if strings.TrimSpace(l) != "" {
			oldLines = append(oldLines, strings.TrimSpace(l))
		}
	}
	if len(oldLines) == 0 {
		return 0, 0, "", false
	}
	contentLines := strings.SplitAfter(content, "\n")
	// Precompute byte offsets for each line.
	offsets := make([]int, len(contentLines)+1)
	for i, l := range contentLines {
		offsets[i+1] = offsets[i] + len(l)
	}
	matches := 0
	matchStart, matchEnd := 0, 0
	for i := 0; i+len(oldLines) <= len(contentLines); i++ {
		matched := true
		for j := range oldLines {
			if strings.TrimSpace(contentLines[i+j]) != oldLines[j] {
				matched = false
				break
			}
		}
		if matched {
			matches++
			matchStart = offsets[i]
			matchEnd = offsets[i+len(oldLines)]
			if matches > 1 {
				return 0, 0, "", false
			}
		}
	}
	if matches == 1 {
		return matchStart, matchEnd, "indentation-insensitive", true
	}
	return 0, 0, "", false
}

// computeEditDiffAndPatch produces a human diff and a standard unified patch
// for the edit result details (Pi-style). Keeps it self-contained without
// extra deps. Only includes changed hunks with small context.
func computeEditDiffAndPatch(oldContent, newContent, path string) (string, string) {
	if oldContent == newContent {
		return "", ""
	}
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Simple Myers-ish but actually a greedy LCS for small files is overkill;
	// use line-by-line walk with run-length for changed regions. Good enough
	// for agent edit feedback and matches the spirit of Pi details.
	var diffLines []string
	var patchLines []string
	patchLines = append(patchLines, fmt.Sprintf("--- a/%s", path))
	patchLines = append(patchLines, fmt.Sprintf("+++ b/%s", path))

	i, j := 0, 0
	for i < len(oldLines) || j < len(newLines) {
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			i++
			j++
			continue
		}
		// Start of a change hunk. Collect a small window.
		startI := i
		startJ := j
		// context before (1 line if possible)
		ctxBefore := 0
		if i > 0 {
			ctxBefore = 1
		}
		// advance to collect the differing region
		for i < len(oldLines) && j < len(newLines) && oldLines[i] != newLines[j] {
			i++
			j++
		}
		// Also consume runs of deletes or inserts
		for i < len(oldLines) && (j >= len(newLines) || oldLines[i] != newLines[j]) {
			i++
		}
		for j < len(newLines) && (i >= len(oldLines) || oldLines[i] != newLines[j]) {
			j++
		}
		// context after
		ctxAfter := 0
		if i < len(oldLines) {
			ctxAfter = 1
		}

		// Emit hunk header (approximate; 0-based converted)
		hunkStartOld := startI - ctxBefore
		if hunkStartOld < 0 {
			hunkStartOld = 0
		}
		hunkStartNew := startJ - ctxBefore
		if hunkStartNew < 0 {
			hunkStartNew = 0
		}
		oldCount := (i - startI) + ctxBefore + ctxAfter
		newCount := (j - startJ) + ctxBefore + ctxAfter
		if oldCount < 0 {
			oldCount = 0
		}
		if newCount < 0 {
			newCount = 0
		}
		patchLines = append(patchLines, fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunkStartOld+1, oldCount, hunkStartNew+1, newCount))

		// context before
		for k := 0; k < ctxBefore && (startI-ctxBefore+k) >= 0; k++ {
			ln := oldLines[startI-ctxBefore+k]
			diffLines = append(diffLines, " "+ln)
			patchLines = append(patchLines, " "+ln)
		}
		// removed
		for k := startI; k < i; k++ {
			ln := oldLines[k]
			diffLines = append(diffLines, "-"+ln)
			patchLines = append(patchLines, "-"+ln)
		}
		// added
		for k := startJ; k < j; k++ {
			ln := newLines[k]
			diffLines = append(diffLines, "+"+ln)
			patchLines = append(patchLines, "+"+ln)
		}
		// context after
		for k := 0; k < ctxAfter && i+k < len(oldLines); k++ {
			ln := oldLines[i+k]
			diffLines = append(diffLines, " "+ln)
			patchLines = append(patchLines, " "+ln)
		}
	}

	diffStr := strings.Join(diffLines, "\n")
	if len(diffLines) > 0 {
		diffStr += "\n"
	}
	patchStr := strings.Join(patchLines, "\n")
	if len(patchLines) > 0 {
		patchStr += "\n"
	}
	return diffStr, patchStr
}

type ListDirTool struct{}

func (t *ListDirTool) Name() string        { return "list_dir" }
func (t *ListDirTool) Description() string { return "List files in a directory." }
func (t *ListDirTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {Type: "string", Description: "Directory path (default: cwd)"},
	}
}
func (t *ListDirTool) Run(ctx Context, args map[string]any) (Result, error) {
	p, _ := args["path"].(string)
	if p == "" {
		p = "."
	}
	full := filepath.Join(ctx.CWD, p)
	entries, err := os.ReadDir(full)
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	var lines []string
	for _, e := range entries {
		if e.IsDir() {
			lines = append(lines, e.Name()+"/")
		} else {
			lines = append(lines, e.Name())
		}
	}
	return Result{OK: true, Output: strings.Join(lines, "\n")}, nil
}

type SearchTool struct{}

func (t *SearchTool) Name() string { return "search" }
func (t *SearchTool) Description() string {
	return "Search for a string in files (case-insensitive substring)."
}
func (t *SearchTool) Parameters() map[string]Param {
	return map[string]Param{
		"query":     {Type: "string", Description: "String to search for", Required: true},
		"path":      {Type: "string", Description: "Directory to search (default: cwd)"},
		"extension": {Type: "string", Description: "Filter by file extension (e.g. 'ts')"},
	}
}
func (t *SearchTool) Run(ctx Context, args map[string]any) (Result, error) {
	q, _ := args["query"].(string)
	if q == "" {
		return Result{OK: false, Error: "query is required"}, nil
	}
	p, _ := args["path"].(string)
	if p == "" {
		p = "."
	}
	ext, _ := args["extension"].(string)
	root := filepath.Join(ctx.CWD, p)
	var matches []string
	var walk func(path string, depth int) error
	walk = func(path string, depth int) error {
		if depth > 8 {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			if e.IsDir() {
				if e.Name() == "node_modules" || e.Name() == ".git" || e.Name() == "dist" {
					continue
				}
				walk(filepath.Join(path, e.Name()), depth+1)
			} else {
				if ext != "" && !strings.HasSuffix(e.Name(), "."+ext) {
					continue
				}
				data, err := os.ReadFile(filepath.Join(path, e.Name()))
				if err != nil {
					continue
				}
				if strings.Contains(strings.ToLower(string(data)), strings.ToLower(q)) {
					matches = append(matches, filepath.Join(path, e.Name()))
				}
			}
		}
		return nil
	}
	_ = walk(root, 0)
	if len(matches) > 100 {
		matches = matches[:100]
	}
	if len(matches) == 0 {
		return Result{OK: true, Output: "No matches"}, nil
	}
	return Result{OK: true, Output: strings.Join(matches, "\n")}, nil
}

// ShellTool executes an arbitrary shell command. It prefers bash
// over dash/sh so that bash-only features (arrays, [[ ]], ${var//}
// expansions, process substitution) keep working. On systems where
// bash is not installed, it falls back to sh so the user does not
// get a hard failure — bash is just the default that lines up with
// how the user usually authors shell snippets.
type ShellTool struct{}

func (t *ShellTool) Name() string { return "run_shell" }
func (t *ShellTool) Description() string {
	return "Run a shell command. Defaults to bash, falls back to sh if bash is not installed. Requires allowShell."
}
func (t *ShellTool) Parameters() map[string]Param {
	return map[string]Param{
		"command": {Type: "string", Description: "Shell command to execute", Required: true},
	}
}
func (t *ShellTool) Run(ctx Context, args map[string]any) (Result, error) {
	if !ctx.AllowShell {
		return Result{OK: false, Error: "Shell execution is disabled in config."}, nil
	}
	cmd, _ := args["command"].(string)
	if cmd == "" {
		return Result{OK: false, Error: "command is required"}, nil
	}
	c := exec.CommandContext(context.Background(), shellCommand(), "-c", cmd)
	c.Dir = ctx.CWD
	done := make(chan struct{})
	var stdout, stderr strings.Builder
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Start(); err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	go func() {
		_ = c.Wait()
		close(done)
	}()
	select {
	case <-done:
		out := stdout.String()
		if stderr.Len() > 0 {
			out += "\nSTDERR:\n" + stderr.String()
		}
		return Result{OK: true, Output: out}, nil
	case <-time.After(60 * time.Second):
		_ = c.Process.Kill()
		return Result{OK: false, Error: "command timed out after 60s", Output: stdout.String()}, nil
	}
}

// shellCommand returns the shell to use for run_shell. We prefer
// bash because it gives consistent behaviour across macOS/Linux
// (dash on Debian/Ubuntu and sh on macOS differ in ways that
// silently break common one-liners). If bash is not on PATH we
// fall back to sh so the tool still works on minimal containers.
func shellCommand() string {
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash"
	}
	return "sh"
}

func defaultTools() []Tool {
	return []Tool{
		&ReadFileTool{},
		&WriteFileTool{},
		&EditFileTool{},
		&ListDirTool{},
		&SearchTool{},
		&ShellTool{},
		&SpawnAgentTool{},
		&TaskOutputTool{},
		&AskUserQuestionTool{},
		&TodoWriteTool{},
	}
}

// SpawnAgentTool dispatches a sub-agent to work in the
// background. The main agent stays interactive; the sub-agent
// runs in a goroutine and reports back via a task ID the main
// agent can poll with task_output.
//
// The tool is intentionally minimal: it returns immediately
// with a task ID and a one-line "started" message. The actual
// sub-agent execution is handled by the session/UI layer.
// The tool side only produces the descriptor the LLM uses to
// learn about its sub-agents.
type SpawnAgentTool struct{}

func (t *SpawnAgentTool) Name() string { return "spawn_agent" }
func (t *SpawnAgentTool) Description() string {
	return "Dispatch a sub-agent to work in the background. The sub-agent runs in parallel to your own context and reports back when done. " +
		"Use this for tasks that would otherwise blow up your context window — codebase exploration, " +
		"running a long test suite, refactoring a large file, or any task that produces a lot of intermediate output. " +
		"Returns a task_id you can poll with task_output. The main conversation stays responsive while the sub-agent works."
}
func (t *SpawnAgentTool) Parameters() map[string]Param {
	return map[string]Param{
		"task":  {Type: "string", Description: "What the sub-agent should do. Be specific — include file paths, function names, and the success criteria.", Required: true},
		"role":  {Type: "string", Description: "Specialist role for the sub-agent: 'explore' (read-only investigation), 'developer' (writes code), 'tester' (runs tests), 'reviewer' (code review), 'researcher' (docs lookup). Default: 'developer'.", Required: false},
		"model": {Type: "string", Description: "Override the model spec (e.g. 'openai:o3'). Default: same as the main agent.", Required: false},
	}
}
func (t *SpawnAgentTool) Run(ctx Context, args map[string]any) (Result, error) {
	task, _ := args["task"].(string)
	if task == "" {
		return Result{OK: false, Error: "task is required"}, nil
	}
	role, _ := args["role"].(string)
	if role == "" {
		role = "developer"
	}
	// Generate a task ID. The UI/session layer tracks the
	// sub-agent; here we just return the descriptor so the
	// LLM learns it can keep going.
	taskID := "subagent-" + time.Now().Format("150405.000000")
	return Result{
		OK:     true,
		Output: fmt.Sprintf("sub-agent dispatched: task_id=%s role=%s\n\nThe sub-agent will work in the background. Use task_output(task_id=\"%s\") to check on it.\n\nYou can continue working on other tasks — the sub-agent will report back when done.", taskID, role, taskID),
	}, nil
}

// TaskOutputTool polls a background sub-agent for its result.
// The sub-agent is identified by the task_id returned from
// spawn_agent. Returns the latest output the sub-agent has
// produced, or a "still running" message.
type TaskOutputTool struct{}

func (t *TaskOutputTool) Name() string { return "task_output" }
func (t *TaskOutputTool) Description() string {
	return "Poll a background sub-agent for its result. Pass the task_id returned from spawn_agent. " +
		"Returns the sub-agent's latest output (full or partial). If the sub-agent is still running, the tool returns a status line and you can call again later."
}
func (t *TaskOutputTool) Parameters() map[string]Param {
	return map[string]Param{
		"task_id": {Type: "string", Description: "The sub-agent's task_id from spawn_agent.", Required: true},
		"wait":    {Type: "boolean", Description: "If true, block until the sub-agent finishes (up to 60s). If false (default), return immediately with current status.", Required: false},
	}
}
func (t *TaskOutputTool) Run(ctx Context, args map[string]any) (Result, error) {
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return Result{OK: false, Error: "task_id is required"}, nil
	}
	// The actual implementation lives in the session/UI layer.
	// Here we just confirm the tool is wired so the LLM knows
	// the call shape.
	return Result{
		OK:     true,
		Output: fmt.Sprintf("Polling task_id=%s. (Status: the sub-agent is tracked by the session. Its output will be available when it finishes.)", taskID),
	}, nil
}

// AskUserQuestionTool is the structured question tool. The
// UI layer renders a multi-choice prompt and the answer
// comes back as a normal user message.
type AskUserQuestionTool struct{}

func (t *AskUserQuestionTool) Name() string { return "ask_user_question" }
func (t *AskUserQuestionTool) Description() string {
	return "Ask the user structured question(s) with 2-4 multiple-choice options each. " +
		"The TUI always adds a final 'Type something.' option for a custom answer — do not include that in your options. " +
		"When you have several related decisions, pass them all in one call via `questions` (do not call this tool multiple times in a row). " +
		"Use when requirements are ambiguous or depend on user preferences; skip when you can infer the answer from the codebase or prior messages."
}
func (t *AskUserQuestionTool) Parameters() map[string]Param {
	return map[string]Param{
		"questions": {Type: "string", Description: "Preferred for multiple decisions: JSON array of {id, question, header, options} objects. Each options entry is 2-4 {label, description?} objects or plain strings. Ask every related question in this one array.", Required: false},
		"question":  {Type: "string", Description: "Single-question mode: the prompt (under 80 chars). Omit when using `questions`.", Required: false},
		"options":   {Type: "string", Description: "Single-question mode: JSON array of 2-4 {label, description?} objects. Omit when using `questions`.", Required: false},
		"header":    {Type: "string", Description: "Single-question mode: short header shown above the options. Default: 'Question'.", Required: false},
	}
}
func (t *AskUserQuestionTool) Run(ctx Context, args map[string]any) (Result, error) {
	questions, _ := args["questions"].(string)
	if questions != "" {
		return Result{
			OK:     true,
			Output: fmt.Sprintf("questions queued: %s\n\nThe user will answer all of them in one TUI panel.", questions),
		}, nil
	}
	question, _ := args["question"].(string)
	if question == "" {
		return Result{OK: false, Error: "question or questions is required"}, nil
	}
	options, _ := args["options"].(string)
	if options == "" {
		return Result{OK: false, Error: "options is required for single-question mode"}, nil
	}
	return Result{
		OK:     true,
		Output: fmt.Sprintf("question queued: %q\noptions: %s\n\nThe user will see this in the TUI. Their choice will be returned as a normal message.", question, options),
	}, nil
}

// TodoWriteTool is the structured todo list. The UI layer
// renders the list in the right panel; the model receives
// the list shape on every step and uses it to drive the
// todo panel.
type TodoWriteTool struct{}

func (t *TodoWriteTool) Name() string { return "todo_write" }
func (t *TodoWriteTool) Description() string {
	return "Set the structured todo list for the current task. The list shows up in the right panel so the user can see what you're working on. " +
		"Call this at the start of a non-trivial task with 3-7 items, then update the status (pending / in_progress / completed) as you go. " +
		"Mark items 'in_progress' when you start them, 'completed' when done. Keep the list under 10 items; use the parent's status to collapse completed sub-tasks."
}
func (t *TodoWriteTool) Parameters() map[string]Param {
	return map[string]Param{
		"todos": {Type: "string", Description: "JSON array of {content, status, activeForm} items. status is 'pending' | 'in_progress' | 'completed'. activeForm is the present-continuous form shown in the spinner (e.g. 'Running tests').", Required: true},
	}
}
func (t *TodoWriteTool) Run(ctx Context, args map[string]any) (Result, error) {
	todos, _ := args["todos"].(string)
	if todos == "" {
		return Result{OK: false, Error: "todos is required"}, nil
	}
	return Result{
		OK:     true,
		Output: fmt.Sprintf("todo list updated: %s", todos),
	}, nil
}

// Avoid unused-import warning
var _ fs.FileInfo
var _ = json.Marshal
