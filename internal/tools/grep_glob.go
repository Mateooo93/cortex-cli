package tools

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var skipSearchDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
}

// GrepTool searches file contents. Prefers ripgrep when installed.
type GrepTool struct{}

func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return "Search file contents for a pattern (regex). Returns matching lines as path:line:content. " +
		"Prefer this over shell grep/rg. Supports optional glob filter and case_insensitive."
}
func (t *GrepTool) Parameters() map[string]Param {
	return map[string]Param{
		"pattern":           {Type: "string", Description: "Regex or literal pattern to search for", Required: true},
		"path":              {Type: "string", Description: "Directory or file to search (default: cwd)"},
		"glob":              {Type: "string", Description: "Optional file glob filter (e.g. '*.go')"},
		"case_insensitive":  {Type: "boolean", Description: "Case-insensitive search (default false)"},
		"head_limit":        {Type: "number", Description: "Maximum matches to return (default 100)"},
	}
}
func (t *GrepTool) Run(ctx Context, args map[string]any) (Result, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		if q, _ := args["query"].(string); q != "" {
			pattern = q
		}
	}
	if strings.TrimSpace(pattern) == "" {
		return Result{OK: false, Error: "pattern is required"}, nil
	}
	root, _ := args["path"].(string)
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	root, _ = resolvePath(ctx.CWD, root)
	glob, _ := args["glob"].(string)
	caseInsensitive := parseBoolArg(args, "case_insensitive")
	limit := parseHeadLimit(args, 100)

	if out, ok := runRipgrep(ctx.CWD, pattern, root, glob, caseInsensitive, limit); ok {
		if out == "" {
			return Result{OK: true, Output: "No matches"}, nil
		}
		return Result{OK: true, Output: out}, nil
	}

	out, err := grepWalk(pattern, root, glob, caseInsensitive, limit)
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	if out == "" {
		return Result{OK: true, Output: "No matches"}, nil
	}
	return Result{OK: true, Output: out}, nil
}

// GlobFileSearchTool finds files by glob pattern under a directory.
type GlobFileSearchTool struct{}

func (t *GlobFileSearchTool) Name() string { return "glob_file_search" }
func (t *GlobFileSearchTool) Description() string {
	return "Find files by glob pattern (e.g. '*.go', '**/*.ts'). Returns paths relative to the search root. " +
		"Prefer this over find/ls in bash."
}
func (t *GlobFileSearchTool) Parameters() map[string]Param {
	return map[string]Param{
		"glob_pattern":      {Type: "string", Description: "Glob pattern (e.g. '**/*.go', 'src/*.ts')", Required: true},
		"target_directory":  {Type: "string", Description: "Directory to search under (default: cwd)"},
		"path":              {Type: "string", Description: "Alias for target_directory"},
		"head_limit":        {Type: "number", Description: "Maximum paths to return (default 200)"},
	}
}
func (t *GlobFileSearchTool) Run(ctx Context, args map[string]any) (Result, error) {
	pattern, _ := args["glob_pattern"].(string)
	if pattern == "" {
		if p, _ := args["pattern"].(string); p != "" {
			pattern = p
		}
	}
	if strings.TrimSpace(pattern) == "" {
		return Result{OK: false, Error: "glob_pattern is required"}, nil
	}
	root, _ := args["target_directory"].(string)
	if root == "" {
		root, _ = args["path"].(string)
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	root, _ = resolvePath(ctx.CWD, root)
	limit := parseHeadLimit(args, 200)

	if matches, ok := globWithRipgrep(root, pattern, limit); ok {
		if len(matches) == 0 {
			return Result{OK: true, Output: "No files matched"}, nil
		}
		return Result{OK: true, Output: strings.Join(matches, "\n")}, nil
	}

	matches, err := globWalk(root, pattern, limit)
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	if len(matches) == 0 {
		return Result{OK: true, Output: "No files matched"}, nil
	}
	return Result{OK: true, Output: strings.Join(matches, "\n")}, nil
}

type namedTool struct {
	toolName string
	inner    Tool
}

func (t *namedTool) Name() string        { return t.toolName }
func (t *namedTool) Description() string { return t.inner.Description() }
func (t *namedTool) Parameters() map[string]Param {
	return t.inner.Parameters()
}
func (t *namedTool) Run(ctx Context, args map[string]any) (Result, error) {
	return t.inner.Run(ctx, args)
}

func parseHeadLimit(args map[string]any, defaultLimit int) int {
	for _, key := range []string{"head_limit", "max_results", "limit"} {
		if v, ok := args[key]; ok {
			if f, ok := v.(float64); ok && f > 0 {
				return int(f)
			}
		}
	}
	return defaultLimit
}

func runRipgrep(cwd, pattern, root, glob string, caseInsensitive bool, limit int) (string, bool) {
	if _, err := exec.LookPath("rg"); err != nil {
		return "", false
	}
	args := []string{"--no-heading", "--line-number", "--color=never", "--max-count", fmt.Sprintf("%d", limit)}
	if caseInsensitive {
		args = append(args, "-i")
	}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, pattern, root)
	cmd := exec.Command("rg", args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stdout.Len() == 0 {
			return "", true
		}
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) > limit {
		lines = lines[:limit]
	}
	return strings.Join(lines, "\n"), true
}

func globWithRipgrep(root, pattern string, limit int) ([]string, bool) {
	if _, err := exec.LookPath("rg"); err != nil {
		return nil, false
	}
	args := []string{"--files", "-g", pattern, root}
	cmd := exec.Command("rg", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return nil, true
	}
	var matches []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		matches = append(matches, line)
		if len(matches) >= limit {
			break
		}
	}
	sort.Strings(matches)
	return matches, true
}

func compilePattern(pattern string, caseInsensitive bool) (*regexp.Regexp, error) {
	if caseInsensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		lit := regexp.QuoteMeta(pattern)
		if caseInsensitive {
			lit = "(?i)" + lit
		}
		return regexp.Compile(lit)
	}
	return re, nil
}

func grepWalk(pattern, root, glob string, caseInsensitive bool, limit int) (string, error) {
	re, err := compilePattern(pattern, caseInsensitive)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}
	glob = strings.TrimSpace(glob)
	var matches []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if skipSearchDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if glob != "" {
			if ok, _ := filepath.Match(glob, filepath.Base(path)); !ok {
				if ok2, _ := filepath.Match(glob, path); !ok2 {
					return nil
				}
			}
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			if re.FindStringIndex(line) != nil {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", path, lineNo, line))
				if len(matches) >= limit {
					return errStopWalk
				}
			}
		}
		return nil
	})
	if err == errStopWalk {
		err = nil
	}
	return strings.Join(matches, "\n"), err
}

var errStopWalk = fmt.Errorf("stop walk")

func globWalk(root, pattern string, limit int) ([]string, error) {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	var matches []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skipSearchDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if matchGlobPattern(pattern, rel) {
			matches = append(matches, rel)
			if len(matches) >= limit {
				return errStopWalk
			}
		}
		return nil
	})
	if err == errStopWalk {
		err = nil
	}
	sort.Strings(matches)
	return matches, err
}

func matchGlobPattern(pattern, rel string) bool {
	if pattern == "" {
		return false
	}
	if strings.Contains(pattern, "**") {
		re, err := globPatternToRegex(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(rel)
	}
	if ok, _ := filepath.Match(pattern, rel); ok {
		return true
	}
	ok, _ := filepath.Match(pattern, filepath.Base(rel))
	return ok
}

func globPatternToRegex(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		if i+1 < len(pattern) && pattern[i] == '*' && pattern[i+1] == '*' {
			b.WriteString(".*")
			i++
			continue
		}
		switch pattern[i] {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '[', ']', '{', '}', '|', '^', '$', '\\':
			b.WriteByte('\\')
			b.WriteByte(pattern[i])
		default:
			b.WriteByte(pattern[i])
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}