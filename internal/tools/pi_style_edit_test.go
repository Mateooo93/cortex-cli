package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFileTool_PiStyleEditsJSONString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.ts")
	if err := os.WriteFile(path, []byte("const a = 1;\nconst b = 2;\nconst c = 3;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (&EditFileTool{}).Run(Context{AllowWrite: true, CWD: dir}, map[string]any{
		"path":  "app.ts",
		"edits": `[{"oldText":"const a = 1;","newText":"const a = 10;"},{"oldText":"const c = 3;","newText":"const c = 30;"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok, got error=%q output=%q", res.Error, res.Output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "const a = 10;") || !strings.Contains(got, "const c = 30;") {
		t.Fatalf("expected both edits applied, got %q", got)
	}
	if !strings.Contains(res.Output, "replaced 2 block(s)") {
		t.Fatalf("expected multi-edit output, got %q", res.Output)
	}
}

func TestEditFileTool_PiStyleRejectsOverlaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.ts")
	if err := os.WriteFile(path, []byte("abcdef\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (&EditFileTool{}).Run(Context{AllowWrite: true, CWD: dir}, map[string]any{
		"path":  "app.ts",
		"edits": `[{"oldText":"abc","newText":"ABC"},{"oldText":"bcd","newText":"BCD"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatalf("expected overlap failure")
	}
	if !strings.Contains(res.Error, "overlaps") {
		t.Fatalf("expected overlap error, got %q", res.Error)
	}
}

func TestEditFileTool_PiStyleEditsArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.ts")
	if err := os.WriteFile(path, []byte("const a = 1;\nconst b = 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (&EditFileTool{}).Run(Context{AllowWrite: true, CWD: dir}, map[string]any{
		"path": "app.ts",
		"edits": []any{
			map[string]any{"oldText": "const a = 1;", "newText": "const a = 100;"},
			map[string]any{"oldText": "const b = 2;", "newText": "const b = 200;"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok, got error=%q", res.Error)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "const a = 100;") || !strings.Contains(got, "const b = 200;") {
		t.Fatalf("expected real-array edits applied, got %q", got)
	}
}

func TestEditFileTool_ParametersExposeNestedEditsSchema(t *testing.T) {
	params := (&EditFileTool{}).Parameters()
	edits := params["edits"]
	if edits.Type != "array" {
		t.Fatalf("expected edits to be array, got %q", edits.Type)
	}
	if edits.Items == nil || edits.Items.Type != "object" {
		t.Fatalf("expected edits.items object schema, got %#v", edits.Items)
	}
	if edits.Items.Properties["oldText"].Type != "string" || !edits.Items.Properties["oldText"].Required {
		t.Fatalf("expected required oldText string schema, got %#v", edits.Items.Properties["oldText"])
	}
	if edits.Items.Properties["newText"].Type != "string" || !edits.Items.Properties["newText"].Required {
		t.Fatalf("expected required newText string schema, got %#v", edits.Items.Properties["newText"])
	}
}

func TestEditFileTool_PiStyleInvalidJSON(t *testing.T) {
	res, err := (&EditFileTool{}).Run(Context{AllowWrite: true, CWD: t.TempDir()}, map[string]any{
		"path":  "x.go",
		"edits": `[{bad json]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatalf("expected invalid JSON failure")
	}
	if !strings.Contains(res.Error, "invalid edits JSON") {
		t.Fatalf("expected invalid JSON hint, got %q", res.Error)
	}
}
