package protocol

import "testing"

func TestMergeTodoList_PreservesContentOnStatusOnlyUpdate(t *testing.T) {
	prev := []TodoItem{
		{ID: "1", Content: "Read auth module", Status: TodoInProgress},
		{ID: "2", Content: "Write tests", Status: TodoPending},
	}
	next := []TodoItem{
		{ID: "1", Status: TodoCompleted},
		{ID: "2", Status: TodoInProgress},
	}
	got := MergeTodoList(prev, next)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Content != "Read auth module" || got[0].Status != TodoCompleted {
		t.Fatalf("item 0 = %+v, want completed with preserved content", got[0])
	}
	if got[1].Content != "Write tests" || got[1].Status != TodoInProgress {
		t.Fatalf("item 1 = %+v, want in_progress with preserved content", got[1])
	}
}

func TestTodoContentFromMap_AcceptsAliases(t *testing.T) {
	if got := TodoContentFromMap(map[string]any{"title": "Ship feature"}); got != "Ship feature" {
		t.Fatalf("title = %q", got)
	}
	if got := TodoContentFromMap(map[string]any{"activeForm": "Running tests"}); got != "Running tests" {
		t.Fatalf("activeForm = %q", got)
	}
}

func TestNormalizeTodoItem_FillsMissingContent(t *testing.T) {
	got := NormalizeTodoItem(TodoItem{ID: "abc", Status: TodoPending}, 0)
	if got.Content != "Todo abc" {
		t.Fatalf("content = %q, want Todo abc", got.Content)
	}
}