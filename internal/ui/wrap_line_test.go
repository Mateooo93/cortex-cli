package ui

import "testing"

func TestWrapLine_LongUserMessageNoPanic(t *testing.T) {
	line := "This is a very long user message with many words and unicode — and it used to panic during resize rerender because start advanced beyond the current loop index, causing slice bounds like [182:137]. Keep wrapping safely even with lots of content and enough words to trigger many wraps."
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("wrapLine panicked: %v", r)
		}
	}()
	wrapped := wrapLine(line, 69)
	if len(wrapped) == 0 {
		t.Fatalf("expected wrapped lines")
	}
}

func TestWrapLine_LongUnbrokenNoPanic(t *testing.T) {
	line := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("wrapLine panicked: %v", r)
		}
	}()
	wrapped := wrapLine(line, 17)
	if len(wrapped) < 2 {
		t.Fatalf("expected hard-wrapped lines, got %d", len(wrapped))
	}
}
