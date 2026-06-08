package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepGlobTools_RegisteredInDefaultTools(t *testing.T) {
	reg := NewRegistry()
	for _, name := range []string{"grep", "search", "glob_file_search", "glob_files"} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("%q not registered in default registry", name)
		}
	}
}

func TestGrepWalk_FindsMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	if err := os.WriteFile(path, []byte("package main\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := grepWalk("func Foo", dir, "*.go", false, 10)
	if err != nil {
		t.Fatalf("grepWalk: %v", err)
	}
	if !strings.Contains(out, "func Foo() {}") {
		t.Fatalf("output = %q, want match", out)
	}
	if !strings.Contains(out, ":2:") {
		t.Fatalf("output missing line number: %q", out)
	}
}

func TestGlobWalk_FindsNestedFiles(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "internal", "tools")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "grep.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	matches, err := globWalk(dir, "**/*.go", 10)
	if err != nil {
		t.Fatalf("globWalk: %v", err)
	}
	if len(matches) != 1 || matches[0] != "internal/tools/grep.go" {
		t.Fatalf("matches = %v, want [internal/tools/grep.go]", matches)
	}
}

func TestGrepTool_RequiresPattern(t *testing.T) {
	tool := &GrepTool{}
	res, err := tool.Run(Context{CWD: "."}, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected failure without pattern")
	}
}

func TestSearchTool_MapsLegacyQuery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("needle here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &SearchTool{}
	res, err := tool.Run(Context{CWD: dir}, map[string]any{"query": "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("result not OK: %s", res.Error)
	}
	if !strings.Contains(res.Output, "needle here") {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestGlobFileSearchTool_RequiresPattern(t *testing.T) {
	tool := &GlobFileSearchTool{}
	res, err := tool.Run(Context{CWD: "."}, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected failure without glob_pattern")
	}
}

func TestNamedTool_GlobFilesAlias(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	inner := &GlobFileSearchTool{}
	alias := &namedTool{toolName: "glob_files", inner: inner}
	if alias.Name() != "glob_files" {
		t.Fatalf("name = %q", alias.Name())
	}
	res, err := alias.Run(Context{CWD: dir}, map[string]any{"glob_pattern": "*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("result not OK: %s", res.Error)
	}
	if !strings.Contains(res.Output, "x.go") {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestNewFilteredRegistry_IncludesGrepAndGlob(t *testing.T) {
	reg := NewFilteredRegistry([]string{"grep", "glob_files"})
	if _, ok := reg.Get("grep"); !ok {
		t.Fatal("grep missing from filtered registry")
	}
	if _, ok := reg.Get("glob_files"); !ok {
		t.Fatal("glob_files missing from filtered registry")
	}
	if _, ok := reg.Get("glob_file_search"); ok {
		t.Fatal("glob_file_search should not be in filtered registry unless requested")
	}
}