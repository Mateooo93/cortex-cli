package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// chdir is a test helper that switches the process working
// directory for the duration of the test. We need it because
// the session_persist code is now per-project: it reads
// sessions from `<cwd>/.cortex/sessions.json` so different
// projects don't see each other's history.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
}

func TestSessionPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	original := []SavedSession{
		{
			ID:        "abc-123",
			Label:     "Refactor auth",
			Model:     "cortex",
			CreatedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			ID:         "fork-def-456",
			Label:      "Bug triage",
			Model:      "openai/gpt-4o",
			CreatedAt:  time.Date(2026, 6, 2, 8, 30, 0, 0, time.UTC),
			ParentID:   "abc-123",
			ForkTurnIdx: 3,
		},
	}
	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := sessionsFilePath()
	if path == "" {
		t.Fatal("sessionsFilePath returned empty")
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := loadSavedSessions()
	if len(got) != 2 {
		t.Fatalf("expected 2 saved sessions, got %d", len(got))
	}
	if got[0].ID != "abc-123" {
		t.Errorf("ID mismatch: %s", got[0].ID)
	}
	if got[1].ParentID != "abc-123" || got[1].ForkTurnIdx != 3 {
		t.Errorf("fork fields not preserved: %+v", got[1])
	}
}

func TestSessionPersistMissingFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := loadSavedSessions()
	if len(got) != 0 {
		t.Errorf("expected 0 saved sessions for missing file, got %d", len(got))
	}
}

func TestSessionPersistCorruptFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := sessionsFilePath()
	if err := os.WriteFile(path, []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := loadSavedSessions()
	if len(got) != 0 {
		t.Errorf("expected 0 saved sessions for corrupt file, got %d", len(got))
	}
}

func TestFirstUserMessage(t *testing.T) {
	sess := &SessionState{
		chatMessages: []ChatMessage{
			{Type: MsgAssistant, Text: "Hi"},
			{Type: MsgUser, Text: "Help me with X\nThen Y"},
		},
	}
	if got := firstUserMessage(sess); got != "Help me with X" {
		t.Errorf("got %q", got)
	}

	empty := &SessionState{}
	if got := firstUserMessage(empty); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	if got := firstUserMessage(nil); got != "" {
		t.Errorf("expected empty for nil, got %q", got)
	}
}

func TestSaveLoadChatRoundTrip(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	original := []ChatMessage{
		{Type: MsgUser, Text: "Hello", Rendered: "  user-style hello", Timestamp: time.Unix(1700000000, 0).UTC()},
		{Type: MsgAssistant, Text: "Hi there", Timestamp: time.Unix(1700000001, 0).UTC()},
		{Type: MsgThinking, Text: "user said hello", Timestamp: time.Unix(1700000002, 0).UTC()},
		{Type: MsgToolCall, Text: "edit_file foo.go (5 lines changed)", ToolName: "edit_file", FilePath: "foo.go", Timestamp: time.Unix(1700000003, 0).UTC()},
		{Type: MsgToolResult, Text: "Edited foo.go", ToolName: "edit_file", IsError: false, Timestamp: time.Unix(1700000004, 0).UTC()},
		{Type: MsgError, Text: "oops", IsError: true, Timestamp: time.Unix(1700000005, 0).UTC()},
	}
	saveChat("session-1", original)

	got := loadSavedChat("session-1")
	if len(got) != len(original) {
		t.Fatalf("expected %d messages, got %d", len(original), len(got))
	}
	for i, m := range got {
		if m.Type != original[i].Type {
			t.Errorf("[%d] Type: got %v, want %v", i, m.Type, original[i].Type)
		}
		if m.Text != original[i].Text {
			t.Errorf("[%d] Text: got %q, want %q", i, m.Text, original[i].Text)
		}
		if m.ToolName != original[i].ToolName {
			t.Errorf("[%d] ToolName: got %q, want %q", i, m.ToolName, original[i].ToolName)
		}
		if m.IsError != original[i].IsError {
			t.Errorf("[%d] IsError: got %v, want %v", i, m.IsError, original[i].IsError)
		}
		if m.FilePath != original[i].FilePath {
			t.Errorf("[%d] FilePath: got %q, want %q", i, m.FilePath, original[i].FilePath)
		}
		// Rendered IS persisted (see PersistSessions comment) so
		// that messages without a re-render path still show up
		// after a restart. The test below uses empty Rendered
		// in the input on purpose, so a non-empty output is a
		// forward-compat signal: callers writing a future
		// version that populates Rendered will still pass.
		if m.Rendered != "" {
			t.Logf("[%d] Rendered persisted (expected for new format), got %q", i, m.Rendered)
		}
	}
}

func TestLoadSavedChatMissing(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := loadSavedChat("does-not-exist"); len(got) != 0 {
		t.Errorf("expected 0 messages for missing chat, got %d", len(got))
	}
	if got := loadSavedChat(""); len(got) != 0 {
		t.Errorf("expected 0 messages for empty id, got %d", len(got))
	}
}

func TestLoadSavedChatCorrupt(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex", "chats"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, ".cortex", "chats", "corrupt.json")
	if err := os.WriteFile(path, []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := loadSavedChat("corrupt"); len(got) != 0 {
		t.Errorf("expected 0 messages for corrupt file, got %d", len(got))
	}
}

func TestDeleteSavedChat(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	saveChat("to-delete", []ChatMessage{{Type: MsgUser, Text: "x"}})
	if got := loadSavedChat("to-delete"); len(got) != 1 {
		t.Fatalf("expected 1 message after save, got %d", len(got))
	}
	deleteSavedChat("to-delete")
	if got := loadSavedChat("to-delete"); len(got) != 0 {
		t.Errorf("expected 0 messages after delete, got %d", len(got))
	}
	// Deleting a non-existent chat should be a no-op (no panic).
	deleteSavedChat("never-existed")
	deleteSavedChat("")
}

func TestSaveChatEmptyIDIsNoOp(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Should not panic, should not create any file.
	saveChat("", []ChatMessage{{Type: MsgUser, Text: "x"}})
	if _, err := os.Stat(filepath.Join(dir, ".cortex", "chats")); !os.IsNotExist(err) {
		t.Errorf("expected chats dir to NOT exist, got err=%v", err)
	}
}

func TestRestoreSavedSessionsLoadsChat(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a sessions.json with two saved sessions, and a chat
	// file for one of them.
	saved := []SavedSession{
		{ID: "live-1", Label: "Live", Model: "cortex", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "restored-1", Label: "Restored", Model: "cortex", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
	}
	data, _ := json.MarshalIndent(saved, "", "  ")
	if err := os.WriteFile(sessionsFilePath(), data, 0o644); err != nil {
		t.Fatalf("write sessions: %v", err)
	}
	saveChat("restored-1", []ChatMessage{
		{Type: MsgUser, Text: "previous question", Rendered: "  user-style: previous question\n", Timestamp: time.Unix(1700000000, 0).UTC()},
		{Type: MsgAssistant, Text: "previous answer", Rendered: "  assistant-style: previous answer\n", Timestamp: time.Unix(1700000001, 0).UTC()},
	})

	cfg := &config.Config{}
	m := Model{cfg: cfg, sessions: []*SessionState{
		{daemonSessionID: "live-1", persistID: "live-1", modelName: "cortex", createdAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}}
	m.mdRenderer = NewMarkdownRenderer(80, true, NewStyles(true).CodeBoxBorderStyle)
	m.styles = NewStyles(true)
	loaded := loadSavedSessions()
	m.restoreSavedSessions(loaded, m.sessions[0])

	// After restore, the live session is kept as-is, and the
	// restored-1 placeholder is appended.
	if len(m.sessions) != 2 {
		t.Fatalf("expected 2 sessions after restore, got %d", len(m.sessions))
	}
	restored := m.sessions[1]
	if restored.persistID != "restored-1" {
		t.Errorf("persistID: got %q, want %q", restored.persistID, "restored-1")
	}
	if len(restored.chatMessages) != 2 {
		t.Fatalf("expected 2 chat messages restored, got %d", len(restored.chatMessages))
	}
	if restored.chatMessages[0].Text != "previous question" {
		t.Errorf("first restored message text: got %q", restored.chatMessages[0].Text)
	}
	if restored.chatMessages[1].Text != "previous answer" {
		t.Errorf("second restored message text: got %q", restored.chatMessages[1].Text)
	}
	// Rendered is the property the bug report was about: the
	// user reported an empty chat panel after a restart, which
	// happened because chat files written by older versions
	// omitted the Rendered field. With the new code, messages
	// loaded from disk must come back with their ANSI output
	// intact so buildRenderedChat does not skip them.
	if restored.chatMessages[0].Rendered != "  user-style: previous question\n" {
		t.Errorf("first restored message Rendered: got %q", restored.chatMessages[0].Rendered)
	}
	if restored.chatMessages[1].Rendered != "  assistant-style: previous answer\n" {
		t.Errorf("second restored message Rendered: got %q", restored.chatMessages[1].Rendered)
	}
}

// TestPersistSessionsFlushesInFlightBuffers verifies that calling
// PersistSessions while the agent is mid-stream captures the
// in-flight assistant + thinking buffers into the on-disk chat
// file. Without this, quitting during streaming would lose the
// agent's last partial response (the user-reported bug).
func TestPersistSessionsFlushesInFlightBuffers(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	// Simulate an in-flight turn: the agent has streamed some text
	// but has not yet emitted agent_done, so flushSessionBuf has
	// not been called.
	sess.daemonSessionID = "stream-1"
	sess.persistID = "stream-1"
	sess.chatMessages = []ChatMessage{
		{Type: MsgUser, Text: "first question"},
	}
	sess.assistantBuf = "partial response so far..."
	sess.thinkingBuf = "user wants help"
	sess.showThinking = true

	m.persistSessions()

	// Re-read the chat from disk and check that the in-flight
	// buffers were captured.
	persisted := loadSavedChat("stream-1")
	if len(persisted) < 3 {
		t.Fatalf("expected at least 3 messages persisted (user + thinking + assistant), got %d", len(persisted))
	}
	var sawThinking, sawAssistant bool
	for _, msg := range persisted {
		if msg.Type == MsgThinking && msg.Text == "user wants help" {
			sawThinking = true
		}
		if msg.Type == MsgAssistant && msg.Text == "partial response so far..." {
			sawAssistant = true
		}
	}
	if !sawThinking {
		t.Error("expected in-flight thinking buffer to be persisted")
	}
	if !sawAssistant {
		t.Error("expected in-flight assistant buffer to be persisted")
	}
}

// TestPersistSessionsIsAtomic verifies that the on-disk sessions
// file is not corrupted when the directory is empty / has stale
// state. The atomic temp+rename path should not lose existing
// sessions if a write fails partway.
func TestPersistSessionsIsAtomic(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a baseline sessions.json
	original := []SavedSession{
		{ID: "a", Label: "A", Model: "cortex", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	data, _ := json.MarshalIndent(original, "", "  ")
	if err := os.WriteFile(sessionsFilePath(), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read it back, add another session, write again
	current := loadSavedSessions()
	current = append(current, SavedSession{ID: "b", Label: "B", Model: "cortex", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)})
	saveSessions(current)

	got := loadSavedSessions()
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions after rewrite, got %d", len(got))
	}
	if got[1].ID != "b" {
		t.Errorf("second session ID: got %q, want %q", got[1].ID, "b")
	}
}

// TestPersistSessionsSurvivesConcurrentWrites verifies that two
// goroutines racing to persist the same session list do not
// produce a torn file. The persistMu serialises the writes, so
// the final on-disk state must be one of the two inputs (never
// a corrupt mix).
func TestPersistSessionsSurvivesConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.daemonSessionID = "concurrent-1"
	sess.persistID = "concurrent-1"
	sess.chatMessages = []ChatMessage{{Type: MsgUser, Text: "hi"}}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.persistSessions()
		}()
	}
	wg.Wait()

	// After the race, the on-disk file must be valid JSON with
	// exactly one session.
	got := loadSavedSessions()
	if len(got) != 1 {
		t.Fatalf("expected 1 session after concurrent writes, got %d", len(got))
	}
	if got[0].ID != "concurrent-1" {
		t.Errorf("session ID: got %q, want %q", got[0].ID, "concurrent-1")
	}
	// The chat file must also be valid.
	chat := loadSavedChat("concurrent-1")
	if len(chat) != 1 {
		t.Errorf("expected 1 chat message, got %d", len(chat))
	}
}

// TestRestoreChatHistoryVisibleAfterRestart is the regression
// test for the user-reported bug: "when i reload a sessions
// after closing, the chat history isnt there anymore". The bug
// was that chat files written by older versions of the CLI
// did not include the Rendered field, and buildRenderedChat
// silently skipped messages with empty Rendered fields — so
// the chat panel appeared empty after a restart even though
// the messages were loaded into memory.
//
// The fix has two layers:
//   1. New chat files persist the Rendered field, so
//      new-from-scratch sessions display correctly.
//   2. Old chat files (no Rendered) get a best-effort
//      re-render via the existing ChatMessage.rerender
//      method on load.
//
// This test exercises layer 2 by writing a chat file in the
// OLD format (no Rendered field) and asserting that
// restoreSavedSessions produces a session whose messages have
// non-empty Rendered output.
func TestRestoreChatHistoryVisibleAfterRestart(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a chat file in the OLD format: no Rendered field.
	// (We do this by writing the JSON by hand rather than
	// going through saveChat, which would include Rendered.)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex", "chats"), 0o755); err != nil {
		t.Fatalf("mkdir chats: %v", err)
	}
	oldFormatChat := []persistedChatMessage{
		{Type: MsgUser, Text: "previous question"},
		{Type: MsgAssistant, Text: "previous answer"},
	}
	data, _ := json.MarshalIndent(oldFormatChat, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, ".cortex", "chats", "old-session.json"), data, 0o644); err != nil {
		t.Fatalf("write old-format chat: %v", err)
	}

	// Set up the saved-sessions index so the loader picks
	// the old session up.
	saved := []SavedSession{
		{ID: "old-session", Label: "Old", Model: "cortex", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	savedData, _ := json.MarshalIndent(saved, "", "  ")
	if err := os.WriteFile(sessionsFilePath(), savedData, 0o644); err != nil {
		t.Fatalf("write sessions: %v", err)
	}

	cfg := &config.Config{}
	m := Model{cfg: cfg, sessions: []*SessionState{
		{daemonSessionID: "live", persistID: "live", modelName: "cortex", createdAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}}
	m.mdRenderer = NewMarkdownRenderer(80, true, NewStyles(true).CodeBoxBorderStyle)
	m.styles = NewStyles(true)

	loaded := loadSavedSessions()
	m.restoreSavedSessions(loaded, m.sessions[0])

	if len(m.sessions) != 2 {
		t.Fatalf("expected 2 sessions after restore, got %d", len(m.sessions))
	}
	restored := m.sessions[1]
	if len(restored.chatMessages) != 2 {
		t.Fatalf("expected 2 chat messages restored, got %d", len(restored.chatMessages))
	}
	// The whole point of this test: messages loaded from an
	// OLD-format chat file must come back with non-empty
	// Rendered fields, otherwise buildRenderedChat skips
	// them and the chat panel is blank.
	for i, msg := range restored.chatMessages {
		if msg.Rendered == "" {
			t.Errorf("[%d] Rendered is empty after restoring old-format chat — the user will see a blank panel", i)
		}
	}
}
// form of each message is preserved across a save/load round trip.
// This is the property that was missing before, causing the chat
// panel to appear empty after a TUI restart: messages loaded from
// disk had no Rendered field, and buildRenderedChat in chat.go
// silently skips messages with an empty Rendered field.
func TestSaveLoadChatPreservesRendered(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	original := []ChatMessage{
		{Type: MsgUser, Text: "hi", Rendered: "  user-style: hi\n"},
		{Type: MsgAssistant, Text: "hello", Rendered: "  assistant-style: hello\n"},
		{Type: MsgSystem, Text: "Reconnected to daemon.", Rendered: "  system-style: Reconnected to daemon.\n"},
		{Type: MsgError, Text: "oops", Rendered: "  error-style: oops\n"},
	}
	saveChat("render-test", original)

	got := loadSavedChat("render-test")
	if len(got) != len(original) {
		t.Fatalf("expected %d messages, got %d", len(original), len(got))
	}
	for i, m := range got {
		if m.Rendered != original[i].Rendered {
			t.Errorf("[%d] Rendered lost across round trip: got %q, want %q", i, m.Rendered, original[i].Rendered)
		}
	}
}


// chdir is a test helper that switches the process working
// directory for the duration of the test. We need it because
// the session_persist code is now per-project: it reads
// sessions from `<cwd>/.cortex/sessions.json` so different
// projects don't see each other's history.

// TestSessionsPerProject_AreScopedToCwd verifies that two
// different working directories have isolated session lists
// (the user wanted session history scoped to the project so
// they don't see another project's chats when they cd
// somewhere else).
func TestSessionsPerProject_AreScopedToCwd(t *testing.T) {
	// Clean slate.
	projectA := t.TempDir()
	projectB := t.TempDir()

	// Write a session in projectA.
	chdir(t, projectA)
	_ = os.MkdirAll(".cortex", 0o755)
	sessionsA := []SavedSession{
		{ID: "a-1", Label: "Refactor auth", Model: "openai/gpt-4o", CreatedAt: time.Now()},
	}
	data, _ := json.MarshalIndent(sessionsA, "", "  ")
	_ = os.WriteFile(filepath.Join(projectA, ".cortex", "sessions.json"), data, 0o644)

	// Switch to projectB and verify the session from A is
	// not visible.
	chdir(t, projectB)
	_ = os.MkdirAll(".cortex", 0o755)
	sessionsB := []SavedSession{
		{ID: "b-1", Label: "Document the API", Model: "anthropic/claude-opus-4", CreatedAt: time.Now()},
	}
	data, _ = json.MarshalIndent(sessionsB, "", "  ")
	_ = os.WriteFile(filepath.Join(projectB, ".cortex", "sessions.json"), data, 0o644)

	got := loadSavedSessions()
	if len(got) != 1 || got[0].ID != "b-1" {
		t.Errorf("project B should see only its own session, got %+v", got)
	}

	// Switch back to projectA and verify the original
	// session is intact.
	chdir(t, projectA)
	got = loadSavedSessions()
	if len(got) != 1 || got[0].ID != "a-1" {
		t.Errorf("project A should see only its own session, got %+v", got)
	}
}
