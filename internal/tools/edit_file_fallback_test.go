package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindReplacementRange_Exact(t *testing.T) {
	content := "alpha\nbeta\ngamma\n"
	start, end, mode, ok := findReplacementRange(content, "beta\n")
	if !ok || mode != "exact" || content[start:end] != "beta\n" {
		t.Fatalf("unexpected match: ok=%v mode=%s range=%d:%d text=%q", ok, mode, start, end, content[start:end])
	}
}

func TestFindReplacementRange_IndentationInsensitiveUnique(t *testing.T) {
	content := "func x() {\n    return 1\n}\n"
	old := "func x() {\nreturn 1\n}"
	start, end, mode, ok := findReplacementRange(content, old)
	if !ok || mode != "indentation-insensitive" {
		t.Fatalf("expected indentation-insensitive match, got ok=%v mode=%s", ok, mode)
	}
	if !strings.Contains(content[start:end], "return 1") {
		t.Fatalf("wrong range: %q", content[start:end])
	}
}

func TestFindReplacementRange_IndentationInsensitiveAmbiguousFails(t *testing.T) {
	content := "x() {\n  return 1\n}\nx() {\n  return 1\n}\n"
	_, _, _, ok := findReplacementRange(content, "x() {\nreturn 1\n}")
	if ok {
		t.Fatalf("ambiguous indentation-insensitive match should fail")
	}
}

func TestEditFileTool_MissingPathOldStringGivesRetryHint(t *testing.T) {
	res, err := (&EditFileTool{}).Run(Context{AllowWrite: true, CWD: t.TempDir()}, map[string]any{"newString": "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatalf("expected failure")
	}
	if !strings.Contains(res.Error, "Prefer Pi-style ordered args") {
		t.Fatalf("missing retry hint: %q", res.Error)
	}
}

func TestEditFileTool_IndentationInsensitiveFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.py")
	if err := os.WriteFile(path, []byte("def x():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (&EditFileTool{}).Run(Context{AllowWrite: true, CWD: dir}, map[string]any{
		"path":      "x.py",
		"oldString": "def x():\nreturn 1",
		"newString": "def x():\n    return 2\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected edit ok, got error=%q output=%q", res.Error, res.Output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "return 2") {
		t.Fatalf("file not edited: %q", string(data))
	}
	if !strings.Contains(res.Output, "fallback=indentation-insensitive") {
		t.Fatalf("expected fallback note, got %q", res.Output)
	}
}
