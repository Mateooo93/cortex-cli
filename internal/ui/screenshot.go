package ui

import (
	"strings"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/daemon"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

// RenderTestScreenshot returns one static TUI frame with demo chat content,
// right-panel context, todos, and a background process — for README screenshots.
func RenderTestScreenshot(width, height int) string {
	if width < 80 {
		width = 80
	}
	if height < 24 {
		height = 24
	}

	cfg := &config.Config{CWD: "/home/demo/project", Model: "openai/gpt-4o"}
	cortexCfg := cortexconfig.Default()
	client := daemon.NewSessionClient("")
	m := NewModel(cfg, cortexCfg, client, true, "", true, true)
	m.width = width
	m.height = height

	if sess := m.currentSession(); sess != nil {
		sess.modelName = "openai/gpt-4o"
		sess.todos = []protocol.TodoItem{
			{ID: "1", Content: "Read auth handler", Status: protocol.TodoCompleted},
			{ID: "2", Content: "Refactor session layer", Status: protocol.TodoInProgress},
			{ID: "3", Content: "Run tests", Status: protocol.TodoPending},
		}
		sess.backgroundProcesses = []protocol.BackgroundProcessItem{
			{
				ID:        "proc-1",
				PID:       4242,
				Command:   "go test ./...",
				StartedAt: time.Now().Add(-2 * time.Minute).Unix(),
				Running:   true,
			},
		}
	}
	m.updateChatWidth()

	content := strings.ReplaceAll(m.View().Content, "\r\n", "\n")
	return content
}