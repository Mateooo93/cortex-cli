package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// SavedSession is a small record of a session the user previously had
// open. We persist this so the Sessions tab can show the user's prior
// sessions across CLI restarts.
type SavedSession struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	Model       string    `json:"model"`
	CreatedAt   time.Time `json:"createdAt"`
	ParentID    string    `json:"parentId,omitempty"`
	ForkTurnIdx int       `json:"forkTurnIdx,omitempty"`
}

func chatsDirPath() string {
	dir := projectConfigDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "chats")
}

func sessionsFilePath() string {
	dir := projectConfigDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "sessions.json")
}

// projectConfigDir returns the per-project .cortex directory.
// Sessions are scoped to the project they're started in so the
// user doesn't see chat history from another project in a
// different folder. The directory is `<cwd>/.cortex`. We fall
// back to the global ~/.cortex dir if we can't determine cwd.
func projectConfigDir() string {
	wd, err := os.Getwd()
	if err != nil || wd == "" {
		return cortexconfig.Dir()
	}
	projectDir := filepath.Join(wd, ".cortex")
	// Make sure the directory exists. Sessions are written
	// lazily; we don't want a missing dir to look like a bug.
	_ = os.MkdirAll(projectDir, 0o755)
	return projectDir
}

func loadSavedSessions() []SavedSession {
	path := sessionsFilePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	var saved []SavedSession
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil
	}
	return saved
}

func saveSessions(saved []SavedSession) {
	path := sessionsFilePath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return
	}
	// Atomic write: temp file + rename so a crash mid-write does not
	// produce a torn JSON file (which would zero out the user's entire
	// session list and lose the restore path on next launch).
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "sessions.json.tmp.*")
	if err != nil {
		_ = os.WriteFile(path, data, 0o644)
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	_ = os.Rename(tmpName, path)
}

var persistMu sync.Mutex

// loadSavedChat reads a session's chat scrollback from disk.
// Returns nil (no error) if the file is missing or unreadable -- the
// session just opens with an empty chat.
//
// The on-disk format only persists the raw Text and metadata needed
// to re-render the scrollback; the ANSI-escape Rendered field is
// regenerated on load with the current terminal width.
func loadSavedChat(id string) []ChatMessage {
	if id == "" {
		return nil
	}
	dir := chatsDirPath()
	if dir == "" {
		return nil
	}
	path := filepath.Join(dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	var raw []persistedChatMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	out := make([]ChatMessage, 0, len(raw))
	for _, r := range raw {
		var elapsed time.Duration
		if r.TurnElapsed != "" {
			if d, err := time.ParseDuration(r.TurnElapsed); err == nil {
				elapsed = d
			}
		}
		out = append(out, ChatMessage{
			Type:         r.Type,
			Text:         r.Text,
			Rendered:     r.Rendered,
			Timestamp:    r.Timestamp,
			ToolName:     r.ToolName,
			IsError:      r.IsError,
			Detail:       r.Detail,
			FilePath:     r.FilePath,
			IsGrouped:    r.IsGrouped,
			GroupIndex:   r.GroupIndex,
			ShowToolName: r.ShowToolName,
			TurnModel:    r.TurnModel,
			TurnElapsed:  elapsed,
			TurnCost:     r.TurnCost,
		})
	}
	return out
}

// persistedChatMessage is the on-disk representation of a
// ChatMessage. We do NOT persist the ANSI-escape Rendered field
// because that would lock the saved chat to the terminal width it
// was captured at; re-rendering on load is both cheaper and
// width-correct.
type persistedChatMessage struct {
	Type ChatMessageType `json:"type"`
	Text string          `json:"text"`
	// Rendered is the pre-styled ANSI string for this message.
	// We persist it (rather than regenerating on load) because
	// not every ChatMessageType has a re-render path: system
	// success/retry messages, plan messages, workflow messages,
	// and error messages do not roundtrip through rerender().
	//
	// The trade-off is that on load, the text is locked to the
	// terminal width it was captured at. That is acceptable
	// because the renderer pads each line to a fixed inner width,
	// and lipgloss handles the right-edge truncation visually.
	// Without persisting Rendered, the chat panel would be empty
	// after a restart — the original bug the user reported.
	Rendered     string    `json:"rendered,omitempty"`
	Timestamp    time.Time `json:"timestamp,omitempty"`
	ToolName     string    `json:"toolName,omitempty"`
	IsError      bool      `json:"isError,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	FilePath     string    `json:"filePath,omitempty"`
	IsGrouped    bool      `json:"isGrouped,omitempty"`
	GroupIndex   int       `json:"groupIndex,omitempty"`
	ShowToolName bool      `json:"showToolName,omitempty"`
	TurnModel    string    `json:"turnModel,omitempty"`
	TurnElapsed  string    `json:"turnElapsed,omitempty"`
	TurnCost     float64   `json:"turnCost,omitempty"`
}

// saveChat writes a session's chat scrollback to disk using the
// compact persistedChatMessage representation. Writes go through a
// temp file + rename so a crash mid-write does not produce a torn
// JSON file (which would lose the entire scrollback for that
// session).
func saveChat(id string, msgs []ChatMessage) {
	if id == "" {
		return
	}
	dir := chatsDirPath()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	out := make([]persistedChatMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, persistedChatMessage{
			Type:         m.Type,
			Text:         m.Text,
			Rendered:     m.Rendered,
			Timestamp:    m.Timestamp,
			ToolName:     m.ToolName,
			IsError:      m.IsError,
			Detail:       m.Detail,
			FilePath:     m.FilePath,
			IsGrouped:    m.IsGrouped,
			GroupIndex:   m.GroupIndex,
			ShowToolName: m.ShowToolName,
			TurnModel:    m.TurnModel,
			TurnElapsed:  m.TurnElapsed.String(),
			TurnCost:     m.TurnCost,
		})
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(dir, id+".json")
	tmp, err := os.CreateTemp(dir, id+".json.tmp.*")
	if err != nil {
		_ = os.WriteFile(path, data, 0o644)
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	_ = os.Rename(tmpName, path)
}

// deleteSavedChat removes a session's chat scrollback file from
// disk. Used when a session is explicitly closed by the user.
func deleteSavedChat(id string) {
	if id == "" {
		return
	}
	dir := chatsDirPath()
	if dir == "" {
		return
	}
	path := filepath.Join(dir, id+".json")
	_ = os.Remove(path)
}

// sessionHasPersistableContent returns true when a
// session has meaningful chat content worth showing in
// the Sessions tab after restart. Empty placeholder
// sessions (Ctrl+T then quit, reconnect placeholders,
// model-only sessions, etc.) should NOT be saved — the
// user reported they bloat the Sessions tab.
//
// We count user/assistant/tool/workflow content, but
// ignore pure system/error/noise and empty thinking.
// In-flight assistant/thinking buffers only count if
// they contain non-whitespace text.
func sessionHasPersistableContent(sess *SessionState) bool {
	if sess == nil {
		return false
	}
	for _, msg := range sess.chatMessages {
		if strings.TrimSpace(msg.Text) == "" && strings.TrimSpace(msg.Rendered) == "" {
			continue
		}
		switch msg.Type {
		case MsgUser, MsgAssistant, MsgToolCall, MsgToolResult,
			MsgPlanProposal, MsgPlanTaskStart, MsgPlanTaskDone, MsgPlanSummary,
			MsgWorkflowStart, MsgWorkflowStepStart, MsgWorkflowStepDone, MsgWorkflowComplete:
			return true
		case MsgThinking:
			// Thinking alone should not keep an empty
			// session alive, especially now thinking is
			// hidden by default.
			continue
		}
	}
	if strings.TrimSpace(sess.assistantBuf) != "" {
		return true
	}
	// User input currently typed but not sent should
	// not create a saved session; it is not part of
	// chat history and would restore as an empty tab.
	return false
}

// firstUserMessage returns the first user message in a session,
// truncated, or "" if there is none. Used to derive a label for
// persisted sessions.
func firstUserMessage(sess *SessionState) string {
	if sess == nil {
		return ""
	}
	for _, msg := range sess.chatMessages {
		if msg.Type == MsgUser {
			line := strings.SplitN(msg.Text, "\n", 2)[0]
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// persistSessions writes the user's current session list (and each
// session's chat scrollback) to disk so it can be restored on the
// next CLI launch. We keep the session-list records small (no chat
// history) and stable-ordered by CreatedAt so the Sessions tab
// renders the same way on every launch.
//
// Each session's chat scrollback is persisted separately via
// saveChat, so this function only handles the session list (label,
// model, lineage) plus the chat flushes.
//
// IMPORTANT: chat scrollback files are written FIRST, then the
// sessions list is written. That way, if the process is killed in
// the middle of persistence, the worst that can happen is a stale
// session list (which is harmless — restoreSavedSessions will
// just keep the existing on-disk chat file and skip orphans). If
// we wrote the list first and the process died before the chat
// files, the user would see their session appear in the list with
// an empty chat — exactly the bug we are fixing.
func (m *Model) persistSessions() {
	persistMu.Lock()
	defer persistMu.Unlock()

	// Take snapshots of the chats while holding the lock so a
	// concurrent chat update does not race with our copy. We
	// flush the snapshots to disk after releasing the lock.
	type chatSnapshot struct {
		id   string
		msgs []ChatMessage
	}
	var chats []chatSnapshot

	saved := make([]SavedSession, 0, len(m.sessions))
	for _, sess := range m.sessions {
		if sess == nil {
			continue
		}
		label := sess.label
		if label == "" {
			label = firstUserMessage(sess)
		}
		if label == "" {
			label = sess.modelName
		}
		if label == "" {
			label = "session"
		}
		modelName := sess.modelName
		if modelName == "" && m.cortexCfg != nil {
			modelName = m.cortexCfg.DefaultModel
		}
		id := sess.daemonSessionID
		if id == "" {
			id = sess.persistID
		}
		if id == "" {
			continue
		}
		created := sess.createdAt
		if created.IsZero() {
			created = time.Now()
		}
		// Skip truly empty sessions. A new Ctrl+T
		// session with no user/assistant/tool content
		// should not be saved just because it has a
		// model name, placeholder daemonSessionID, or
		// reconnect state — those entries bloat the
		// Sessions tab. Also delete any stale chat file
		// with the same id in case an older version saved
		// it before this stricter filter existed.
		if !sessionHasPersistableContent(sess) {
			deleteSavedChat(id)
			continue
		}
		saved = append(saved, SavedSession{
			ID:          id,
			Label:       label,
			Model:       modelName,
			CreatedAt:   created,
			ParentID:    sess.parentID,
			ForkTurnIdx: sess.forkTurnIdx,
		})

		// Persist the chat scrollback for this session. We make
		// a defensive copy of the chat slice here so the writer
		// can iterate it without racing with future
		// chatMessages appends from the TUI's Update path.
		//
		// Also include the in-flight streaming buffers so a
		// quit-during-stream does not lose the agent's last
		// partial response. The buffers are captured as raw
		// ChatMessage values (we do NOT call the rendering
		// helpers here) so the persist path does not depend on
		// the renderer's internal state. The buffers will be
		// re-rendered on load via the existing msg.rerender
		// path.
		msgs := sess.chatMessages
		if sess.showThinking && sess.thinkingBuf != "" {
			msgs = append(msgs, ChatMessage{
				Type: MsgThinking,
				Text: sess.thinkingBuf,
			})
		}
		if sess.assistantBuf != "" {
			msgs = append(msgs, ChatMessage{
				Type: MsgAssistant,
				Text: sess.assistantBuf,
			})
		}
		if len(msgs) > 0 {
			msgsCopy := make([]ChatMessage, len(msgs))
			copy(msgsCopy, msgs)
			chats = append(chats, chatSnapshot{id: id, msgs: msgsCopy})
		}
	}
	sort.SliceStable(saved, func(i, j int) bool {
		return saved[i].CreatedAt.Before(saved[j].CreatedAt)
	})

	// Flush chat scrollback files FIRST so a crash between the
	// two writes leaves a stale session list (harmless) rather
	// than a session with a missing chat (the original bug).
	for _, snap := range chats {
		saveChat(snap.id, snap.msgs)
	}
	saveSessions(saved)
}

// PersistSessions is the exported wrapper around persistSessions.
// It exists so main.go's signal handler (and any other external
// caller) can flush the latest chat scrollback to disk before
// exiting — bubbletea normally tears down the model on quit, but
// a SIGINT delivered directly to the process bypasses the
// normal quit dialog and would otherwise lose the user's most
// recent messages.
func (m *Model) PersistSessions() { m.persistSessions() }

// reRenderLoadedChat re-renders any messages in `msgs` that have
// an empty Rendered field. This handles the backward-compat path:
// chat files written by older versions of the CLI did not
// persist the Rendered field, so the messages load as
// "invisible" until we regenerate them. We use a best-effort
// approach via the ChatMessage.rerender method for the message
// types it knows about, and skip the rest (a small visual
// regression for chat files written by very old versions, but
// strictly better than the empty chat panel the user reported).
func reRenderLoadedChat(msgs []ChatMessage, md *MarkdownRenderer, s Styles, width int) []ChatMessage {
	if md == nil {
		return msgs
	}
	for i, msg := range msgs {
		if msg.Rendered == "" {
			msgs[i] = msg.rerender(md, s, width)
		}
	}
	return msgs
}

// restoreSavedSessions builds *SessionState placeholders for the
// previously-saved sessions and prepends them to m.sessions (with
// the live session in main.go prepended first). Each placeholder
// has client=nil + reconnecting=true, and gets reconnected when the
// user opens it.
//
// The placeholder's chat scrollback is loaded from
// chats/<id>.json so the user can see (and scroll through) their
// prior conversation immediately, without waiting for the daemon
// to reconnect.
func (m *Model) restoreSavedSessions(saved []SavedSession, current *SessionState) {
	if len(saved) == 0 {
		return
	}
	// The current session was already created in NewModel and
	// (for live sessions) has a stable daemonSessionID, which we
	// mirror into persistID so subsequent persistSessions calls
	// reuse it.
	if current != nil && current.persistID == "" {
		current.persistID = current.daemonSessionID
	}
	for _, s := range saved {
		if current != nil && s.ID == current.persistID {
			// Same live session — merge in the saved
			// label/model/parent so the user keeps the
			// AI-generated name across restarts. The chat
			// scrollback stays in-memory (it was already
			// accumulated live this session).
			if current.label == "" {
				current.label = s.Label
			}
			if current.modelName == "" {
				current.modelName = s.Model
			}
			if current.parentID == "" {
				current.parentID = s.ParentID
			}
			if current.forkTurnIdx == 0 {
				current.forkTurnIdx = s.ForkTurnIdx
			}
			continue
		}
		sess := newSessionState(m.cfg, nil)
		sess.persistID = s.ID
		sess.label = s.Label
		sess.modelName = s.Model
		sess.createdAt = s.CreatedAt
		sess.parentID = s.ParentID
		// Restored sessions start with an empty workflow
		// engine — workflows are not persisted (they're
		// in-memory only).
		sess.EnsureWorkflowEngine(m.cortexCfg)
		sess.forkTurnIdx = s.ForkTurnIdx
		sess.reconnecting = true

		// Restore the chat scrollback from disk. The
		// on-disk format persists the pre-rendered ANSI
		// string for each message (the Rendered field), so
		// the chat panel shows content immediately on
		// restart without any re-render pass. We previously
		// tried to skip persisting Rendered and re-generate
		// it on load, but several message types (system
		// success/retry, plan, workflow, error) did not have
		// a re-render path, which left the chat panel empty
		// after a restart — the original bug.
		//
		// For backward compatibility with chat files written
		// by older versions of the CLI (which omitted the
		// Rendered field), we still call reRenderLoadedChat
		// to regenerate the missing renders. This is a no-op
		// for new files and a best-effort fix for old ones.
		// We guard on the renderer because some test
		// harnesses construct a Model without one.
		if msgs := loadSavedChat(s.ID); len(msgs) > 0 {
			if m.mdRenderer != nil {
				width := m.mdRenderer.width + 4
				msgs = reRenderLoadedChat(msgs, m.mdRenderer, m.styles, width)
			}
			sess.chatMessages = msgs
		}

		m.sessions = append(m.sessions, sess)
	}
}
