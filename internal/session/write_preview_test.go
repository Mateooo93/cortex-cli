package session

import "testing"

func TestSummarizeToolCall_WriteFile(t *testing.T) {
	got := summarizeToolCall("write_file", map[string]any{
		"path":    "internal/ui/chat.go",
		"content": "a\nb\nc\n",
	})
	want := "internal/ui/chat.go (3 lines)"
	if got != want {
		t.Fatalf("summarizeToolCall() = %q, want %q", got, want)
	}
}

func TestFormatWritePreviewDetail(t *testing.T) {
	got := formatWritePreviewDetail(3, "one\ntwo")
	want := "@@cortex-write:3@@\none\ntwo"
	if got != want {
		t.Fatalf("formatWritePreviewDetail() = %q, want %q", got, want)
	}
}