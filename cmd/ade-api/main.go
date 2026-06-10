// ade-api is a small local HTTP server used by the ade desktop app
// to read and write cortex-cli config with the same logic as the TUI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/contextbreakdown"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	cortexdaemon "github.com/Mateooo93/cortex-cli/internal/daemon"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
	"github.com/Mateooo93/cortex-cli/internal/provider"
	"github.com/Mateooo93/cortex-cli/internal/provider/codex"
	"github.com/Mateooo93/cortex-cli/internal/provider/xaisub"
	"github.com/google/uuid"
)

type providerView struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"displayName"`
	BaseURL     string `json:"baseURL"`
	KeyPrefix   string `json:"keyPrefix,omitempty"`
	EnvVar      string `json:"envVar,omitempty"`
	NeedsAPIKey bool   `json:"needsAPIKey"`
	AuthKind    string `json:"authKind"`
	AuthLabel   string `json:"authLabel"`
	HelpURL     string `json:"helpURL,omitempty"`
	IsCustom    bool   `json:"isCustom"`
	HasAPIKey   bool   `json:"hasAPIKey"`
	Model       string `json:"model,omitempty"`
}

type otherSettingsView struct {
	Theme         string `json:"theme"`
	ShowThinking  bool   `json:"showThinking"`
	ShowUsage     bool   `json:"showUsage"`
	AutoCompact   bool   `json:"autoCompact"`
	ProjectMemory bool   `json:"projectMemory"`
	Streaming     bool   `json:"streaming"`
	AllowShell    bool   `json:"allowShell"`
	AllowWrite    bool   `json:"allowWrite"`
	AllowGit      bool   `json:"allowGit"`
}

type configView struct {
	DefaultModel string            `json:"defaultModel"`
	Providers    []providerView    `json:"providers"`
	Other        otherSettingsView `json:"other"`
	Models       []modelOption     `json:"models"`
}

type modelOption struct {
	Spec           string `json:"spec"`
	DisplayName    string `json:"displayName"`
	Provider       string `json:"provider"`
	ContextWindow  int64  `json:"contextWindow"`
}

type updateProviderReq struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

type updateOtherReq struct {
	Theme         *string `json:"theme"`
	ShowThinking  *bool   `json:"showThinking"`
	ShowUsage     *bool   `json:"showUsage"`
	AutoCompact   *bool   `json:"autoCompact"`
	ProjectMemory *bool   `json:"projectMemory"`
	DefaultModel  *string `json:"defaultModel"`
}

type historyMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Prompt      string                `json:"prompt"`
	Model       string                `json:"model"`
	Workdir     string                `json:"workdir"`
	History     []historyMessage      `json:"history,omitempty"`
	Attachments []protocol.Attachment `json:"attachments,omitempty"`
}

type chatResp struct {
	Text string `json:"text"`
}

type contextBreakdownReq struct {
	Workdir       string           `json:"workdir"`
	UsedTokens    int              `json:"usedTokens"`
	MaxTokens     int              `json:"maxTokens"`
	ProjectMemory bool             `json:"projectMemory"`
	History       []historyMessage `json:"history,omitempty"`
}

func init() {
	codex.Register()
	xaisub.Register()
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9477", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	srv := &server{startedAt: time.Now()}
	mux.HandleFunc("GET /api/health", srv.handleHealth)
	mux.HandleFunc("GET /api/status", srv.handleStatus)
	mux.HandleFunc("GET /api/config", srv.handleGetConfig)
	mux.HandleFunc("PUT /api/providers/{name}", srv.handleUpdateProvider)
	mux.HandleFunc("GET /api/providers/{name}/auth-url", srv.handleProviderAuthURL)
	mux.HandleFunc("POST /api/providers/{name}/authenticate", srv.handleProviderAuthenticate)
	mux.HandleFunc("POST /api/providers/{name}/test", srv.handleProviderTest)
	mux.HandleFunc("POST /api/providers/{name}/models/refresh", srv.handleProviderModelsRefresh)
	mux.HandleFunc("POST /api/providers", srv.handleAddProvider)
	mux.HandleFunc("PUT /api/other", srv.handleUpdateOther)
	mux.HandleFunc("POST /api/chat", srv.handleChat)
	mux.HandleFunc("POST /api/chat/stream", srv.handleChatStream)
	mux.HandleFunc("POST /api/chat/answer", srv.handleChatAnswer)
	mux.HandleFunc("POST /api/chat/cancel", srv.handleChatCancel)
	mux.HandleFunc("POST /api/context/breakdown", srv.handleContextBreakdown)
	registerExtensionRoutes(mux, srv)

	log.Printf("ade-api listening on http://%s", *addr)
	if err := http.ListenAndServe(*addr, withCORS(mux)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type server struct {
	streamsMu     sync.Mutex
	streamAnswers map[string]chan userAnswerPayload
	streamHandles map[string]*streamHandle
	startedAt     time.Time
}

type streamHandle struct {
	cancel context.CancelFunc
	client *cortexdaemon.SessionClient
}

type userAnswerPayload struct {
	Answer  string
	Text    string
	Answers map[string]string
}

type chatAnswerReq struct {
	StreamID string            `json:"stream_id"`
	Answer   string            `json:"answer"`
	Text     string            `json:"text"`
	Answers  map[string]string `json:"answers"`
}

func (s *server) registerStreamAnswer(id string, ch chan userAnswerPayload) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	if s.streamAnswers == nil {
		s.streamAnswers = make(map[string]chan userAnswerPayload)
	}
	s.streamAnswers[id] = ch
}

func (s *server) unregisterStreamAnswer(id string) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	delete(s.streamAnswers, id)
}

func (s *server) registerStreamHandle(id string, cancel context.CancelFunc) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	if s.streamHandles == nil {
		s.streamHandles = make(map[string]*streamHandle)
	}
	s.streamHandles[id] = &streamHandle{cancel: cancel}
}

func (s *server) attachStreamClient(id string, client *cortexdaemon.SessionClient) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	if h := s.streamHandles[id]; h != nil {
		h.client = client
	}
}

func (s *server) unregisterStreamHandle(id string) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	delete(s.streamHandles, id)
}

func (s *server) cancelStream(id string) bool {
	s.streamsMu.Lock()
	h := s.streamHandles[id]
	s.streamsMu.Unlock()
	if h == nil {
		return false
	}
	if h.cancel != nil {
		h.cancel()
	}
	if h.client != nil {
		h.client.SendCancel()
	}
	return true
}

func (s *server) deliverStreamAnswer(id string, ans userAnswerPayload) error {
	s.streamsMu.Lock()
	ch := s.streamAnswers[id]
	s.streamsMu.Unlock()
	if ch == nil {
		return fmt.Errorf("stream not found or question already answered")
	}
	select {
	case ch <- ans:
		return nil
	default:
		return fmt.Errorf("no pending question on this stream")
	}
}

var orchestrationTools = map[string]bool{
	"ask_user_question": true,
	"todo_write":        true,
}

func isOrchestrationTool(name string) bool {
	return orchestrationTools[name]
}

func (s *server) paths() config.CortexPaths {
	cwd, _ := os.Getwd()
	return config.NewCortexPaths("", config.HomeCortexDir(), cwd)
}

func (s *server) load() (*cortexconfig.Config, error) {
	cfg, err := cortexconfig.Load()
	if err != nil {
		return nil, err
	}
	cfg.EnsureProviderPresets()
	return cfg, nil
}

func authLabel(kind string) string {
	switch kind {
	case "oauth":
		return "OAuth (subscription)"
	case "apikey":
		return "API key"
	case "env":
		return "env var"
	case "none":
		return "no key"
	default:
		return "API key"
	}
}

func (s *server) buildView(cfg *cortexconfig.Config) configView {
	paths := s.paths()
	names := cfg.ProviderNames()
	providers := make([]providerView, 0, len(names))
	for _, name := range names {
		pc, _ := cfg.ProviderConfig(name)
		baseURL := pc.BaseURL
		if baseURL == "" {
			baseURL = cortexconfig.DefaultBaseURL(name)
		}
		authKind := cortexconfig.ProviderAuthKind(name)
		envVar := cortexconfig.ProviderEnvVar(name)
		key := pc.APIKey
		if key == "" && envVar != "" {
			key = os.Getenv(envVar)
		}
		if key == "" && (authKind == "apikey" || authKind == "env") {
			key, _ = config.ResolveProviderKey(name, false)
		}
		hasCredential := key != ""
		if authKind == "oauth" && !hasCredential {
			hasCredential = config.OAuthProviderSignedIn(name)
		}
		if authKind == "none" {
			hasCredential = true
		}
		prefix := ""
		if authKind == "oauth" {
			prefix = config.OAuthProviderStatusPrefix(name)
		} else if key != "" {
			prefix = key
			if len(prefix) > 10 {
				prefix = prefix[:10]
			}
		}
		providers = append(providers, providerView{
			Provider:    name,
			DisplayName: cortexconfig.ProviderDisplayName(name),
			BaseURL:     baseURL,
			KeyPrefix:   prefix,
			EnvVar:      envVar,
			NeedsAPIKey: cortexconfig.ProviderNeedsAPIKey(name),
			AuthKind:    authKind,
			AuthLabel:   authLabel(authKind),
			HelpURL:     cortexconfig.ProviderHelpURL(name),
			IsCustom:    cortexconfig.IsCustomProvider(name),
			HasAPIKey:   hasCredential,
			Model:       pc.Model,
		})
	}

	other := otherSettingsView{
		Theme:         cfg.Theme,
		ShowThinking:  config.ShowThinking(),
		ShowUsage:     cfg.ShowUsage,
		AutoCompact:   cfg.AutoCompact,
		ProjectMemory: config.ProjectMemoryEnabled(paths),
		Streaming:     cfg.Streaming,
		AllowShell:    cfg.Tools.AllowShell,
		AllowWrite:    cfg.Tools.AllowWrite,
		AllowGit:      cfg.Tools.AllowGit,
	}

	models := make([]modelOption, 0)
	seenSpec := map[string]bool{}
	addModel := func(provider, model string) {
		spec := cortexconfig.ModelSpec(provider, model)
		if spec == "" || seenSpec[spec] {
			return
		}
		seenSpec[spec] = true
		display := cortexconfig.ProviderDisplayName(provider)
		if model != "" {
			display = display + " / " + model
		}
		models = append(models, modelOption{
			Spec:          spec,
			DisplayName:   display,
			Provider:      provider,
			ContextWindow: cortexconfig.EffectiveModelContextWindow(spec),
		})
	}

	// Every model the user has configured (populated by /models/refresh or
	// manual provider edits) shows up directly in the picker.
	for _, mc := range cfg.Models {
		provider := mc.Provider
		if provider == "" {
			provider, _, _ = cortexconfig.SplitModelSpec(mc.Model)
		}
		addModel(provider, mc.Model)
	}

	// Ensure each provider's default model is always selectable, even before
	// a model refresh has run.
	for _, name := range names {
		pc, _ := cfg.ProviderConfig(name)
		model := pc.Model
		if model == "" {
			for _, preset := range cortexconfig.BuiltinProviderPresets {
				if preset.Name == name {
					model = preset.DefaultModel
					break
				}
			}
		}
		if model != "" {
			addModel(name, model)
		}
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].DisplayName < models[j].DisplayName
	})

	return configView{
		DefaultModel: cfg.DefaultModel,
		Providers:    providers,
		Other:        other,
		Models:       models,
	}
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, s.buildView(cfg))
}

func (s *server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("provider name required"))
		return
	}
	var req updateProviderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := s.load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if req.BaseURL != "" {
		cfg.SetProviderBaseURL(name, strings.TrimSpace(req.BaseURL))
	}
	if req.APIKey != "" {
		key := strings.TrimSpace(req.APIKey)
		cfg.SetProviderAPIKey(name, key)
		if err := config.StoreProviderKey(name, key); err != nil {
			log.Printf("ade-api: keychain store for %s: %v", name, err)
		}
	}
	if req.Model != "" {
		cfg.EnsureProviderModel(name, strings.TrimSpace(req.Model))
	}
	if err := cortexconfig.Save(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"status": "saved"})
}

func (s *server) handleProviderAuthURL(w http.ResponseWriter, r *http.Request) {
	name := cortexconfig.NormalizeProviderName(strings.TrimSpace(r.PathValue("name")))
	if name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("provider name required"))
		return
	}
	switch name {
	case "codex":
		writeJSON(w, map[string]string{"url": codex.AuthURL()})
		return
	case "xai-sub":
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		authURL, err := xaisub.AuthURL(ctx)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, map[string]string{"url": authURL})
		return
	default:
		if url := cortexconfig.ProviderHelpURL(name); url != "" {
			writeJSON(w, map[string]string{"url": url})
			return
		}
		writeErr(w, http.StatusNotFound, fmt.Errorf("auth URL not available for %s", name))
	}
}

func (s *server) handleProviderAuthenticate(w http.ResponseWriter, r *http.Request) {
	name := cortexconfig.NormalizeProviderName(strings.TrimSpace(r.PathValue("name")))
	if name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("provider name required"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	switch name {
	case "codex":
		res, err := codex.Login(ctx)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		if res == nil || res.Token == nil {
			writeErr(w, http.StatusBadGateway, fmt.Errorf("codex login returned no token"))
			return
		}
		if err := codex.Save(res.Token); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]string{
			"status": "authenticated",
			"url":    res.AuthorizeURL,
			"email":  res.Token.Email,
		})
		return
	case "xai-sub":
		res, err := xaisub.Login(ctx)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		if res == nil || res.Token == nil {
			writeErr(w, http.StatusBadGateway, fmt.Errorf("xAI login returned no token"))
			return
		}
		if err := xaisub.Save(res.Token); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]string{
			"status": "authenticated",
			"url":    res.AuthorizeURL,
			"email":  res.Token.Email,
		})
		return
	case "claude-sub":
		writeErr(w, http.StatusNotImplemented, fmt.Errorf("Claude subscription auth uses CLAUDE_CODE_OAUTH_TOKEN"))
		return
	case "copilot":
		writeErr(w, http.StatusNotImplemented, fmt.Errorf("GitHub Copilot auth uses COPILOT_OAUTH_TOKEN"))
		return
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("authentication is not available for %s", name))
	}
}

func (s *server) handleAddProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		BaseURL string `json:"baseURL"`
		APIKey  string `json:"apiKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	name := cortexconfig.NormalizeProviderName(req.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid provider name"))
		return
	}
	cfg, err := s.load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	baseURL := strings.TrimSpace(req.BaseURL)
	apiKey := strings.TrimSpace(req.APIKey)
	cfg.Models[name] = cortexconfig.ModelConfig{
		Provider: name,
		BaseURL:  baseURL,
		APIKey:   apiKey,
	}
	if baseURL != "" {
		cfg.SetProviderBaseURL(name, baseURL)
	}
	if apiKey != "" {
		cfg.SetProviderAPIKey(name, apiKey)
		if err := config.StoreProviderKey(name, apiKey); err != nil {
			log.Printf("ade-api: keychain store for %s: %v", name, err)
		}
	}
	if err := cortexconfig.Save(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"provider": name})
}

func (s *server) handleUpdateOther(w http.ResponseWriter, r *http.Request) {
	var req updateOtherReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := s.load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if req.Theme != nil {
		cfg.Theme = strings.TrimSpace(*req.Theme)
	}
	if req.ShowUsage != nil {
		cfg.ShowUsage = *req.ShowUsage
	}
	if req.AutoCompact != nil {
		cfg.AutoCompact = *req.AutoCompact
	}
	if req.DefaultModel != nil {
		cfg.DefaultModel = strings.TrimSpace(*req.DefaultModel)
	}
	if err := cortexconfig.Save(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if req.ShowThinking != nil {
		if err := config.SetShowThinking(*req.ShowThinking); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	}
	if req.ProjectMemory != nil {
		if err := config.SetProjectMemory(s.paths(), *req.ProjectMemory); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, map[string]string{"status": "saved"})
}

func respondHeadlessUserQuestion(client *cortexdaemon.SessionClient, data any) {
	q, ok := data.(protocol.EventUserQuestion)
	if !ok {
		_ = client.SendUserAnswer("Continue", "")
		return
	}
	if len(q.Questions) > 0 {
		answers := make(map[string]string, len(q.Questions))
		for _, item := range q.Questions {
			answers[item.ID] = firstQuestionOption(item.RichOptions, item.Options, "Continue")
		}
		_ = client.SendUserAnswerBatch(answers)
		return
	}
	answer := firstQuestionOption(q.RichOptions, q.Options, "Continue with your best judgment")
	_ = client.SendUserAnswer(answer, "")
}

func firstQuestionOption(
	rich []protocol.EventQuestionOption,
	simple []string,
	fallback string,
) string {
	if len(rich) > 0 && strings.TrimSpace(rich[0].Title) != "" {
		return rich[0].Title
	}
	if len(simple) > 0 && strings.TrimSpace(simple[0]) != "" {
		return simple[0]
	}
	return fallback
}

type preparedChat struct {
	client *cortexdaemon.SessionClient
}

func (s *server) prepareChat(req chatReq) (*preparedChat, error) {
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt required")
	}
	cfg, err := s.load()
	if err != nil {
		return nil, err
	}
	cortexdaemon.SetGlobalConfigLoader(func() *cortexconfig.Config {
		latest, loadErr := cortexconfig.Load()
		if loadErr != nil {
			return cfg
		}
		latest.EnsureProviderPresets()
		return latest
	})

	workdir := strings.TrimSpace(req.Workdir)
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = cfg.DefaultModel
	}
	if _, _, err := cfg.GetModel(model); err != nil {
		return nil, fmt.Errorf("unknown model %q: %w", model, err)
	}

	client := cortexdaemon.NewSessionClient("")
	if err := client.Connect(workdir, "", model, false, true, true, true); err != nil {
		return nil, err
	}
	if history := buildHistory(req.History); len(history) > 0 {
		if err := client.SendRestoreHistory(history); err != nil {
			log.Printf("ade-api: restore history: %v", err)
		}
	}
	attachments := validAttachments(req.Attachments)
	if err := client.SendInput(req.Prompt, attachments); err != nil {
		client.SendClose()
		return nil, err
	}
	return &preparedChat{client: client}, nil
}

// buildHistory converts the UI's prior chat turns into provider messages so
// a fresh per-request session still sees the whole conversation.
func buildHistory(history []historyMessage) []provider.Message {
	out := make([]provider.Message, 0, len(history))
	for _, m := range history {
		role := strings.TrimSpace(m.Role)
		content := strings.TrimSpace(m.Content)
		if content == "" || (role != "user" && role != "assistant") {
			continue
		}
		out = append(out, provider.Message{Role: role, Content: content})
	}
	return out
}

func validAttachments(atts []protocol.Attachment) []protocol.Attachment {
	out := make([]protocol.Attachment, 0, len(atts))
	for _, att := range atts {
		if err := protocol.ValidateAttachment(att); err != nil {
			log.Printf("ade-api: dropping attachment: %v", err)
			continue
		}
		out = append(out, att)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type chatCallbacks struct {
	onChunk     func(text string)
	onThinking  func(text string)
	onStatus    func(message string)
	onToolStart func(toolID, name, summary, reason string)
	onToolEnd   func(toolID string, isError bool, errorHint, output, detail string)
	onUsage     func(done protocol.EventStreamDone)
	onQuestion  func(q protocol.EventUserQuestion) error
	waitAnswer  func(ctx context.Context) (userAnswerPayload, error)
	onDone      func(text string) error
	onError     func(err error)
}

func toolStatusMessage(m protocol.EventToolCall) string {
	name := strings.TrimSpace(m.Name)
	if name == "" {
		name = "tool"
	}
	summary := strings.TrimSpace(m.Summary)
	if summary != "" {
		return fmt.Sprintf("Running %s — %s", name, summary)
	}
	return fmt.Sprintf("Running %s…", name)
}

func formatToolCommand(m protocol.EventToolCall) string {
	name := strings.TrimSpace(m.Name)
	switch name {
	case "run_shell", "bash":
		if cmd, ok := m.Arguments["command"].(string); ok && strings.TrimSpace(cmd) != "" {
			return strings.TrimSpace(cmd)
		}
	case "list_dir":
		if path, ok := m.Arguments["path"].(string); ok && strings.TrimSpace(path) != "" {
			return "ls " + strings.TrimSpace(path)
		}
	case "read_file", "write_file", "edit_file", "delete_file", "glob_files", "grep":
		if path, ok := m.Arguments["path"].(string); ok && strings.TrimSpace(path) != "" {
			return fmt.Sprintf("%s %s", name, strings.TrimSpace(path))
		}
	case "web_fetch":
		if rawURL, ok := m.Arguments["url"].(string); ok && strings.TrimSpace(rawURL) != "" {
			return "fetch " + strings.TrimSpace(rawURL)
		}
	}
	summary := strings.TrimSpace(m.Summary)
	if summary != "" {
		return summary
	}
	return name
}

func briefToolErrorHint(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "failed"
	}
	line := trimmed
	if idx := strings.Index(line, "\n"); idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	if len(line) > 96 {
		return line[:93] + "..."
	}
	return line
}

// maxToolOutputChars caps the tool output forwarded over SSE so a huge
// build log doesn't blow up the renderer.
const maxToolOutputChars = 4000

func truncateToolOutput(s string) string {
	s = strings.ToValidUTF8(strings.TrimSpace(s), "")
	if len(s) <= maxToolOutputChars {
		return s
	}
	return s[:maxToolOutputChars] + "\n… (output truncated)"
}

func finishToolResult(cb chatCallbacks, m protocol.EventToolResult) {
	if isOrchestrationTool(m.Name) || cb.onToolEnd == nil {
		return
	}
	hint := ""
	if m.IsError {
		hint = briefToolErrorHint(m.Output)
		if hint == "" || hint == "failed" {
			hint = briefToolErrorHint(m.Detail)
		}
	}
	cb.onToolEnd(m.ToolID, m.IsError, hint, truncateToolOutput(m.Output), truncateToolOutput(m.Detail))
}

func runChatSession(
	ctx context.Context,
	client *cortexdaemon.SessionClient,
	cb chatCallbacks,
) error {
	var out strings.Builder
	events := client.Events()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("chat timed out")
		case ev, ok := <-events:
			if !ok {
				text := strings.TrimSpace(out.String())
				if text == "" {
					return fmt.Errorf("session ended without a response")
				}
				if cb.onDone != nil {
					return cb.onDone(text)
				}
				return nil
			}
			switch ev.Type {
			case "stream_chunk":
				if m, ok := ev.Data.(protocol.EventStreamChunk); ok && m.Text != "" {
					out.WriteString(m.Text)
					if cb.onChunk != nil {
						cb.onChunk(m.Text)
					}
				}
			case "thinking_chunk":
				if m, ok := ev.Data.(protocol.EventThinkingChunk); ok && m.Text != "" {
					if cb.onThinking != nil {
						cb.onThinking(m.Text)
					}
				}
			case "tool_call":
				if m, ok := ev.Data.(protocol.EventToolCall); ok {
					if isOrchestrationTool(m.Name) {
						break
					}
					summary := strings.TrimSpace(m.Summary)
					if summary == "" {
						summary = formatToolCommand(m)
					}
					if cb.onToolStart != nil {
						cb.onToolStart(m.ToolID, m.Name, summary, strings.TrimSpace(m.Reason))
					} else if cb.onStatus != nil {
						cb.onStatus(toolStatusMessage(m))
					}
				}
			case "tool_result":
				if m, ok := ev.Data.(protocol.EventToolResult); ok {
					finishToolResult(cb, m)
				}
			case "stream_done":
				if m, ok := ev.Data.(protocol.EventStreamDone); ok {
					if cb.onUsage != nil {
						cb.onUsage(m)
					}
					if m.FinishReason == "tool_calls" && cb.onStatus != nil {
						cb.onStatus("Planning next step…")
					}
				}
			case "user_question":
				q, ok := ev.Data.(protocol.EventUserQuestion)
				if !ok {
					break
				}
				if cb.onQuestion != nil {
					if err := cb.onQuestion(q); err != nil {
						return err
					}
				}
				if cb.waitAnswer != nil {
					ans, err := cb.waitAnswer(ctx)
					if err != nil {
						return err
					}
					if len(ans.Answers) > 0 {
						_ = client.SendUserAnswerBatch(ans.Answers)
					} else {
						_ = client.SendUserAnswer(ans.Answer, ans.Text)
					}
				} else {
					respondHeadlessUserQuestion(client, ev.Data)
				}
			case "agent_done":
				text := strings.TrimSpace(out.String())
				if cb.onDone != nil {
					return cb.onDone(text)
				}
				return nil
			case "error":
				if m, ok := ev.Data.(protocol.EventError); ok {
					return fmt.Errorf("%s", m.Message)
				}
				return fmt.Errorf("chat failed")
			}
		}
	}
}

func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	prepared, err := s.prepareChat(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	defer prepared.client.SendClose()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	err = runChatSession(ctx, prepared.client, chatCallbacks{
		onDone: func(text string) error {
			writeJSON(w, chatResp{Text: text})
			return nil
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			writeErr(w, http.StatusGatewayTimeout, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
	}
}

func (s *server) handleChatAnswer(w http.ResponseWriter, r *http.Request) {
	var req chatAnswerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	streamID := strings.TrimSpace(req.StreamID)
	if streamID == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("stream_id required"))
		return
	}
	ans := userAnswerPayload{
		Answer:  strings.TrimSpace(req.Answer),
		Text:    strings.TrimSpace(req.Text),
		Answers: req.Answers,
	}
	if ans.Answer == "" && len(ans.Answers) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("answer or answers required"))
		return
	}
	if err := s.deliverStreamAnswer(streamID, ans); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *server) handleChatCancel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StreamID string `json:"stream_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	streamID := strings.TrimSpace(req.StreamID)
	if streamID == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("stream_id required"))
		return
	}
	if !s.cancelStream(streamID) {
		writeErr(w, http.StatusNotFound, fmt.Errorf("stream not found or already finished"))
		return
	}
	writeJSON(w, map[string]string{"status": "cancelling"})
}

func (s *server) handleContextBreakdown(w http.ResponseWriter, r *http.Request) {
	var req contextBreakdownReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	workdir := strings.TrimSpace(req.Workdir)
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	cfg, err := s.load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	paths := config.NewCortexPaths("", config.HomeCortexDir(), workdir)
	history := make([]contextbreakdown.HistoryMessage, 0, len(req.History))
	for _, m := range req.History {
		history = append(history, contextbreakdown.HistoryMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	result := contextbreakdown.Compute(contextbreakdown.Input{
		Workdir:       workdir,
		Paths:         paths,
		Config:        cfg,
		UsedTokens:    req.UsedTokens,
		MaxTokens:     req.MaxTokens,
		ProjectMemory: req.ProjectMemory,
		History:       history,
	})
	writeJSON(w, result)
}

func (s *server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	cwd, _ := os.Getwd()
	writeJSON(w, map[string]any{
		"status":        "ok",
		"cwd":           cwd,
		"homeConfigDir": config.HomeCortexDir(),
		"uptimeMs":      time.Since(s.startedAt).Milliseconds(),
	})
}

// handleProviderTest issues a minimal completion against the provider's
// default model to verify that credentials and base URL actually work.
func (s *server) handleProviderTest(w http.ResponseWriter, r *http.Request) {
	name := cortexconfig.NormalizeProviderName(strings.TrimSpace(r.PathValue("name")))
	if name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("provider name required"))
		return
	}
	cfg, err := s.load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	pc, ok := cfg.ProviderConfig(name)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("unknown provider %q", name))
		return
	}
	model := pc.Model
	if model == "" {
		for _, preset := range cortexconfig.BuiltinProviderPresets {
			if preset.Name == name {
				model = preset.DefaultModel
				break
			}
		}
	}
	if model == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no model configured for %s", name))
		return
	}
	baseURL := pc.BaseURL
	if baseURL == "" {
		baseURL = cortexconfig.DefaultBaseURL(name)
	}
	apiKey := pc.APIKey
	if apiKey == "" {
		if envVar := cortexconfig.ProviderEnvVar(name); envVar != "" {
			if v, found := config.ResolveEnvVar(envVar); found {
				apiKey = v
			}
		}
	}
	if apiKey == "" {
		apiKey, _ = config.ResolveProviderKey(name, true)
	}

	prov, err := provider.New(provider.ModelConfig{
		Provider: name,
		Model:    model,
		BaseURL:  baseURL,
		APIKey:   apiKey,
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	start := time.Now()
	_, err = prov.Chat(ctx, provider.Request{
		Model: model,
		Messages: []provider.Message{
			{Role: "user", Content: "Reply with the single word: OK"},
		},
		MaxTokens: 8,
	})
	if err != nil {
		writeJSON(w, map[string]any{
			"status": "failed",
			"model":  model,
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, map[string]any{
		"status":    "ok",
		"model":     model,
		"latencyMs": time.Since(start).Milliseconds(),
	})
}

// modelRefreshCandidates mirrors the TUI's base-URL candidate logic so a
// model refresh works for gateways that scope models under /v1/<provider>.
func modelRefreshCandidates(cfg *cortexconfig.Config, name, baseURL string) []string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(candidate string) {
		candidate = strings.TrimRight(strings.TrimSpace(candidate), "/")
		if candidate == "" || seen[strings.ToLower(candidate)] {
			return
		}
		seen[strings.ToLower(candidate)] = true
		out = append(out, candidate)
	}
	addScoped := func(scope string) {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope == "" || strings.HasSuffix(strings.ToLower(trimmed), "/"+scope) {
			return
		}
		add(trimmed + "/" + scope)
	}

	add(trimmed)
	// Scopes derived from already-configured models for this provider.
	scopeSeen := map[string]bool{}
	for _, mc := range cfg.Models {
		if cortexconfig.NormalizeProviderName(mc.Provider) != name {
			continue
		}
		if prefix, _, ok := cortexconfig.SplitModelSpec(mc.Model); ok {
			prefix = strings.ToLower(strings.TrimSpace(prefix))
			if prefix != "" && prefix != name && !scopeSeen[prefix] {
				scopeSeen[prefix] = true
				addScoped(prefix)
			}
		}
	}
	if name == "opengateway" || strings.Contains(strings.ToLower(trimmed), "opengateway") {
		for _, scope := range []string{"xiaomi", "google", "minimax", "qwen", "nvidia"} {
			addScoped(scope)
		}
	} else if cortexconfig.DefaultBaseURL(name) == "" {
		addScoped(name)
	}
	return out
}

// handleProviderModelsRefresh fetches the provider's full model catalogue and
// stores every model directly in the config so they appear in the picker.
func (s *server) handleProviderModelsRefresh(w http.ResponseWriter, r *http.Request) {
	name := cortexconfig.NormalizeProviderName(strings.TrimSpace(r.PathValue("name")))
	if name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("provider name required"))
		return
	}
	cfg, err := s.load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	pc, ok := cfg.ProviderConfig(name)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("unknown provider %q", name))
		return
	}

	// Subscription providers expose a fixed model set rather than /models.
	if name == "xai-sub" {
		var ids []string
		for spec := range cfg.Models {
			if p, model, ok := cortexconfig.SplitModelSpec(spec); ok &&
				cortexconfig.NormalizeProviderName(p) == name {
				ids = append(ids, model)
			}
		}
		sort.Strings(ids)
		writeJSON(w, map[string]any{"provider": name, "models": ids, "count": len(ids)})
		return
	}

	baseURL := pc.BaseURL
	if baseURL == "" {
		baseURL = cortexconfig.DefaultBaseURL(name)
	}
	apiKey := pc.APIKey
	if apiKey == "" {
		if envVar := cortexconfig.ProviderEnvVar(name); envVar != "" {
			if v, found := config.ResolveEnvVar(envVar); found {
				apiKey = v
			}
		}
	}
	if apiKey == "" {
		apiKey, _ = config.ResolveProviderKey(name, true)
	}

	candidates := modelRefreshCandidates(cfg, name, baseURL)
	if len(candidates) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no base URL configured for %s", name))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	ids, _, err := provider.FetchModelsFromCandidates(ctx, apiKey, candidates...)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}

	sort.Strings(ids)
	added := 0
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if spec := cfg.EnsureProviderModel(name, id); spec != "" {
			added++
		}
	}
	if err := cortexconfig.Save(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"provider": name, "models": ids, "count": added})
}

func (s *server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	streamID := uuid.NewString()
	w.Header().Set("X-Stream-Id", streamID)
	w.WriteHeader(http.StatusOK)

	writeSSE := func(event string, data any) {
		payload, _ := json.Marshal(data)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
		flusher.Flush()
	}

	baseCtx, baseCancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer baseCancel()
	streamCtx, streamCancel := context.WithCancel(baseCtx)
	s.registerStreamHandle(streamID, streamCancel)
	defer s.unregisterStreamHandle(streamID)

	answerCh := make(chan userAnswerPayload, 1)
	s.registerStreamAnswer(streamID, answerCh)
	defer s.unregisterStreamAnswer(streamID)

	writeSSE("stream_id", map[string]string{"stream_id": streamID})
	writeSSE("status", map[string]string{"message": "Thinking…"})

	prepared, err := s.prepareChat(req)
	if err != nil {
		writeSSE("error", map[string]string{"message": err.Error()})
		return
	}
	defer prepared.client.SendClose()
	s.attachStreamClient(streamID, prepared.client)

	go func() {
		<-streamCtx.Done()
		prepared.client.SendCancel()
	}()

	var partial strings.Builder
	err = runChatSession(streamCtx, prepared.client, chatCallbacks{
		onChunk: func(text string) {
			partial.WriteString(text)
			writeSSE("chunk", map[string]string{"text": text})
		},
		onThinking: func(text string) {
			writeSSE("thinking", map[string]string{"text": text})
		},
		onStatus: func(message string) {
			writeSSE("status", map[string]string{"message": message})
		},
		onToolStart: func(toolID, name, summary, reason string) {
			writeSSE("tool_start", map[string]string{
				"tool_id": toolID,
				"name":    name,
				"summary": summary,
				"reason":  reason,
			})
		},
		onToolEnd: func(toolID string, isError bool, errorHint, output, detail string) {
			writeSSE("tool_end", map[string]any{
				"tool_id":    toolID,
				"is_error":   isError,
				"error_hint": errorHint,
				"output":     output,
				"detail":     detail,
			})
		},
		onUsage: func(done protocol.EventStreamDone) {
			writeSSE("usage", map[string]any{
				"input_tokens":  done.InputTokens,
				"output_tokens": done.OutputTokens,
				"elapsed_ms":    done.ElapsedMs,
				"finish_reason": done.FinishReason,
			})
		},
		onQuestion: func(q protocol.EventUserQuestion) error {
			writeSSE("question", q)
			return nil
		},
		waitAnswer: func(ctx context.Context) (userAnswerPayload, error) {
			select {
			case ans := <-answerCh:
				return ans, nil
			case <-ctx.Done():
				return userAnswerPayload{}, ctx.Err()
			}
		},
		onDone: func(text string) error {
			writeSSE("done", map[string]string{"text": text})
			return nil
		},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			writeSSE("done", map[string]string{"text": strings.TrimSpace(partial.String())})
			return
		}
		writeSSE("error", map[string]string{"message": err.Error()})
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		w.Header().Set("Access-Control-Expose-Headers", "X-Stream-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
