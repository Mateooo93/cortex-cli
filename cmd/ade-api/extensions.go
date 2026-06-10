package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type mcpServer struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Enabled   bool              `json:"enabled"`
	Status    string            `json:"status,omitempty"`
	ToolCount int               `json:"toolCount,omitempty"`
	Tools     []string          `json:"tools,omitempty"`
	Error     string            `json:"error,omitempty"`
}

type automationRecord struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Prompt     string `json:"prompt"`
	Schedule   string `json:"schedule"`
	Workdir    string `json:"workdir,omitempty"`
	Model      string `json:"model,omitempty"`
	Enabled    bool   `json:"enabled"`
	LastRunAt  int64  `json:"lastRunAt,omitempty"`
	LastStatus string `json:"lastStatus,omitempty"`
	LastOutput string `json:"lastOutput,omitempty"`
	NextRunAt  int64  `json:"nextRunAt,omitempty"`
}

type skillMeta struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Enabled     bool   `yaml:"enabled" json:"enabled"`
}

var extensionsMu sync.Mutex

func cortexHome() string {
	dir := config.HomeCortexDir()
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "cortex")
	}
	return dir
}

func mcpStorePath() string {
	return filepath.Join(cortexHome(), "mcp-servers.json")
}

func automationsStorePath() string {
	return filepath.Join(cortexHome(), "automations.json")
}

func skillsDir() string {
	return filepath.Join(cortexHome(), "skills")
}

func readJSONFile(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	return json.Unmarshal(data, dst)
}

func writeJSONFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func loadMcpServers() ([]mcpServer, error) {
	var servers []mcpServer
	if err := readJSONFile(mcpStorePath(), &servers); err != nil {
		return nil, err
	}
	if servers == nil {
		return []mcpServer{}, nil
	}
	for i := range servers {
		if !servers[i].Enabled {
			servers[i].Status = "disabled"
		} else if servers[i].Status == "" {
			servers[i].Status = "unknown"
		}
	}
	return servers, nil
}

func saveMcpServers(servers []mcpServer) error {
	return writeJSONFile(mcpStorePath(), servers)
}

func (s *server) handleListMcpServers(w http.ResponseWriter, _ *http.Request) {
	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	servers, err := loadMcpServers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"servers": servers})
}

func (s *server) handleCreateMcpServer(w http.ResponseWriter, r *http.Request) {
	var payload mcpServer
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("name required"))
		return
	}
	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	servers, err := loadMcpServers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	for _, existing := range servers {
		if existing.Name == payload.Name {
			writeErr(w, http.StatusConflict, fmt.Errorf("server %q already exists", payload.Name))
			return
		}
	}
	payload.Status = "unknown"
	if !payload.Enabled {
		payload.Status = "disabled"
	}
	servers = append(servers, payload)
	if err := saveMcpServers(servers); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, payload)
}

func (s *server) handleUpdateMcpServer(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("name required"))
		return
	}
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	servers, err := loadMcpServers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	idx := -1
	for i, server := range servers {
		if server.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeErr(w, http.StatusNotFound, fmt.Errorf("server %q not found", name))
		return
	}
	current := servers[idx]
	data, _ := json.Marshal(current)
	var merged map[string]any
	_ = json.Unmarshal(data, &merged)
	for key, raw := range patch {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid field %q", key))
			return
		}
		merged[key] = value
	}
	updated, _ := json.Marshal(merged)
	_ = json.Unmarshal(updated, &current)
	current.Name = name
	if !current.Enabled {
		current.Status = "disabled"
	} else if current.Status == "disabled" {
		current.Status = "unknown"
	}
	servers[idx] = current
	if err := saveMcpServers(servers); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, current)
}

func (s *server) handleDeleteMcpServer(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	servers, err := loadMcpServers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	next := servers[:0]
	found := false
	for _, server := range servers {
		if server.Name == name {
			found = true
			continue
		}
		next = append(next, server)
	}
	if !found {
		writeErr(w, http.StatusNotFound, fmt.Errorf("server %q not found", name))
		return
	}
	if err := saveMcpServers(next); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *server) handleTestMcpServer(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	extensionsMu.Lock()
	servers, err := loadMcpServers()
	extensionsMu.Unlock()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var target *mcpServer
	for i := range servers {
		if servers[i].Name == name {
			target = &servers[i]
			break
		}
	}
	if target == nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("server %q not found", name))
		return
	}

	start := time.Now()
	result, testErr := testMcpServer(r.Context(), *target)
	latency := time.Since(start).Milliseconds()

	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	servers, _ = loadMcpServers()
	for i := range servers {
		if servers[i].Name != name {
			continue
		}
		if testErr != nil {
			servers[i].Status = "error"
			servers[i].Error = testErr.Error()
			servers[i].ToolCount = 0
			servers[i].Tools = nil
		} else {
			servers[i].Status = "connected"
			servers[i].Error = ""
			servers[i].ToolCount = result.ToolCount
			servers[i].Tools = result.Tools
		}
		_ = saveMcpServers(servers)
		break
	}

	if testErr != nil {
		writeJSON(w, map[string]any{
			"status": "failed",
			"error":  testErr.Error(),
		})
		return
	}
	writeJSON(w, map[string]any{
		"status":    "ok",
		"tools":     result.Tools,
		"toolCount": result.ToolCount,
		"latencyMs": latency,
	})
}

type mcpTestResult struct {
	Tools     []string
	ToolCount int
}

func testMcpServer(ctx context.Context, server mcpServer) (mcpTestResult, error) {
	transport := strings.ToLower(strings.TrimSpace(server.Transport))
	switch transport {
	case "http", "sse":
		url := strings.TrimSpace(server.URL)
		if url == "" {
			return mcpTestResult{}, fmt.Errorf("url required for %s transport", transport)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return mcpTestResult{}, err
		}
		for k, v := range server.Headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return mcpTestResult{}, err
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return mcpTestResult{}, fmt.Errorf("HTTP %d from MCP endpoint", resp.StatusCode)
		}
		return mcpTestResult{Tools: []string{}, ToolCount: 0}, nil
	default:
		cmdName := strings.TrimSpace(server.Command)
		if cmdName == "" {
			return mcpTestResult{}, fmt.Errorf("command required for stdio transport")
		}
		if _, err := exec.LookPath(cmdName); err != nil {
			return mcpTestResult{}, fmt.Errorf("spawn %s: not found", cmdName)
		}
		args := append([]string{}, server.Args...)
		cmd := exec.CommandContext(ctx, cmdName, args...)
		if len(server.Env) > 0 {
			cmd.Env = os.Environ()
			for k, v := range server.Env {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}
		}
		if err := cmd.Start(); err != nil {
			return mcpTestResult{}, fmt.Errorf("spawn %s: %w", cmdName, err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return mcpTestResult{}, ctx.Err()
		case err := <-done:
			if err != nil {
				return mcpTestResult{}, fmt.Errorf("process exited: %w", err)
			}
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
		}
		return mcpTestResult{Tools: []string{}, ToolCount: 0}, nil
	}
}

func parseSkillFile(data []byte) (skillMeta, string, error) {
	text := string(data)
	var meta skillMeta
	body := strings.TrimSpace(text)
	if strings.HasPrefix(text, "---") {
		parts := strings.SplitN(text, "---", 3)
		if len(parts) >= 3 {
			if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
				return meta, "", err
			}
			body = strings.TrimSpace(parts[2])
		}
	}
	return meta, body, nil
}

func writeSkillFile(path string, meta skillMeta, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	front, err := yaml.Marshal(meta)
	if err != nil {
		return err
	}
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(front), strings.TrimSpace(content))
	return os.WriteFile(path, []byte(doc), 0o644)
}

func (s *server) handleListSkills(w http.ResponseWriter, _ *http.Request) {
	dir := skillsDir()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	skills := make([]map[string]any, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		meta, _, err := parseSkillFile(data)
		if err != nil {
			continue
		}
		if meta.Name == "" {
			meta.Name = entry.Name()
		}
		info, _ := entry.Info()
		var updated int64
		if info != nil {
			updated = info.ModTime().UnixMilli()
		}
		skills = append(skills, map[string]any{
			"name":        meta.Name,
			"description": meta.Description,
			"enabled":     meta.Enabled,
			"updatedAt":   updated,
		})
	}
	writeJSON(w, map[string]any{"skills": skills})
}

func (s *server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	path := filepath.Join(skillsDir(), name, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeErr(w, http.StatusNotFound, fmt.Errorf("skill %q not found", name))
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	meta, content, err := parseSkillFile(data)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if meta.Name == "" {
		meta.Name = name
	}
	info, _ := os.Stat(path)
	var updated int64
	if info != nil {
		updated = info.ModTime().UnixMilli()
	}
	writeJSON(w, map[string]any{
		"name":        meta.Name,
		"description": meta.Description,
		"enabled":     meta.Enabled,
		"content":     content,
		"updatedAt":   updated,
	})
}

func (s *server) handlePutSkill(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	var payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(payload.Description) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("description required"))
		return
	}
	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}
	meta := skillMeta{
		Name:        name,
		Description: strings.TrimSpace(payload.Description),
		Enabled:     enabled,
	}
	path := filepath.Join(skillsDir(), name, "SKILL.md")
	if err := writeSkillFile(path, meta, payload.Content); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"name":        meta.Name,
		"description": meta.Description,
		"enabled":     meta.Enabled,
		"content":     payload.Content,
		"updatedAt":   time.Now().UnixMilli(),
	})
}

func (s *server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	dir := filepath.Join(skillsDir(), name)
	if err := os.RemoveAll(dir); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func loadAutomations() ([]automationRecord, error) {
	var automations []automationRecord
	if err := readJSONFile(automationsStorePath(), &automations); err != nil {
		return nil, err
	}
	if automations == nil {
		return []automationRecord{}, nil
	}
	return automations, nil
}

func saveAutomations(records []automationRecord) error {
	return writeJSONFile(automationsStorePath(), records)
}

func estimateNextRun(schedule string, from time.Time) int64 {
	switch strings.ToLower(strings.TrimSpace(schedule)) {
	case "@hourly":
		return from.Add(time.Hour).UnixMilli()
	case "@daily":
		return from.Add(24 * time.Hour).UnixMilli()
	case "@weekly":
		return from.Add(7 * 24 * time.Hour).UnixMilli()
	default:
		return from.Add(time.Hour).UnixMilli()
	}
}

func (s *server) handleListAutomations(w http.ResponseWriter, _ *http.Request) {
	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	automations, err := loadAutomations()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"automations": automations})
}

func (s *server) handleCreateAutomation(w http.ResponseWriter, r *http.Request) {
	var payload automationRecord
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Prompt) == "" || strings.TrimSpace(payload.Schedule) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("name, prompt, and schedule required"))
		return
	}
	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	automations, err := loadAutomations()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	now := time.Now()
	record := automationRecord{
		ID:        uuid.NewString()[:8],
		Name:      strings.TrimSpace(payload.Name),
		Prompt:    strings.TrimSpace(payload.Prompt),
		Schedule:  strings.TrimSpace(payload.Schedule),
		Workdir:   strings.TrimSpace(payload.Workdir),
		Model:     strings.TrimSpace(payload.Model),
		Enabled:   payload.Enabled,
		NextRunAt: estimateNextRun(payload.Schedule, now),
	}
	automations = append(automations, record)
	if err := saveAutomations(automations); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, record)
}

func (s *server) handleUpdateAutomation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	automations, err := loadAutomations()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	idx := -1
	for i := range automations {
		if automations[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeErr(w, http.StatusNotFound, fmt.Errorf("automation %q not found", id))
		return
	}
	data, _ := json.Marshal(automations[idx])
	var merged map[string]any
	_ = json.Unmarshal(data, &merged)
	for key, raw := range patch {
		var value any
		_ = json.Unmarshal(raw, &value)
		merged[key] = value
	}
	updated, _ := json.Marshal(merged)
	_ = json.Unmarshal(updated, &automations[idx])
	automations[idx].ID = id
	if err := saveAutomations(automations); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, automations[idx])
}

func (s *server) handleDeleteAutomation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	automations, err := loadAutomations()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	next := automations[:0]
	found := false
	for _, record := range automations {
		if record.ID == id {
			found = true
			continue
		}
		next = append(next, record)
	}
	if !found {
		writeErr(w, http.StatusNotFound, fmt.Errorf("automation %q not found", id))
		return
	}
	if err := saveAutomations(next); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *server) runAutomationRecord(ctx context.Context, record *automationRecord) error {
	req := chatReq{
		Prompt:  record.Prompt,
		Model:   record.Model,
		Workdir: record.Workdir,
	}
	prepared, err := s.prepareChat(req)
	if err != nil {
		return err
	}
	defer prepared.client.SendClose()
	var out strings.Builder
	err = runChatSession(ctx, prepared.client, chatCallbacks{
		onChunk: func(text string) { out.WriteString(text) },
		onDone: func(text string) error {
			if strings.TrimSpace(text) != "" {
				out.Reset()
				out.WriteString(text)
			}
			return nil
		},
	})
	record.LastRunAt = time.Now().UnixMilli()
	record.NextRunAt = estimateNextRun(record.Schedule, time.Now())
	if err != nil {
		record.LastStatus = "failed"
		record.LastOutput = err.Error()
		return err
	}
	record.LastStatus = "ok"
	record.LastOutput = strings.TrimSpace(out.String())
	return nil
}

func (s *server) handleRunAutomation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	extensionsMu.Lock()
	automations, err := loadAutomations()
	if err != nil {
		extensionsMu.Unlock()
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	idx := -1
	for i := range automations {
		if automations[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		extensionsMu.Unlock()
		writeErr(w, http.StatusNotFound, fmt.Errorf("automation %q not found", id))
		return
	}
	automations[idx].LastStatus = "running"
	_ = saveAutomations(automations)
	record := automations[idx]
	extensionsMu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	_ = s.runAutomationRecord(ctx, &record)

	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	automations, _ = loadAutomations()
	for i := range automations {
		if automations[i].ID == id {
			automations[i] = record
			_ = saveAutomations(automations)
			writeJSON(w, automations[i])
			return
		}
	}
	writeErr(w, http.StatusNotFound, fmt.Errorf("automation %q not found", id))
}

func (s *server) startAutomationScheduler() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			extensionsMu.Lock()
			automations, err := loadAutomations()
			if err != nil {
				extensionsMu.Unlock()
				continue
			}
			var pending []int
			now := time.Now().UnixMilli()
			for i, record := range automations {
				if !record.Enabled || record.LastStatus == "running" {
					continue
				}
				if record.NextRunAt > 0 && record.NextRunAt > now {
					continue
				}
				pending = append(pending, i)
			}
			extensionsMu.Unlock()
			for _, idx := range pending {
				extensionsMu.Lock()
				automations, _ = loadAutomations()
				if idx >= len(automations) {
					extensionsMu.Unlock()
					continue
				}
				record := automations[idx]
				if !record.Enabled || record.LastStatus == "running" {
					extensionsMu.Unlock()
					continue
				}
				record.LastStatus = "running"
				automations[idx] = record
				_ = saveAutomations(automations)
				extensionsMu.Unlock()

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				_ = s.runAutomationRecord(ctx, &record)
				cancel()

				extensionsMu.Lock()
				automations, _ = loadAutomations()
				for i := range automations {
					if automations[i].ID == record.ID {
						automations[i] = record
						break
					}
				}
				_ = saveAutomations(automations)
				extensionsMu.Unlock()
			}
		}
	}()
}

func registerExtensionRoutes(mux *http.ServeMux, srv *server) {
	mux.HandleFunc("GET /api/mcp/servers", srv.handleListMcpServers)
	mux.HandleFunc("POST /api/mcp/servers", srv.handleCreateMcpServer)
	mux.HandleFunc("PUT /api/mcp/servers/{name}", srv.handleUpdateMcpServer)
	mux.HandleFunc("DELETE /api/mcp/servers/{name}", srv.handleDeleteMcpServer)
	mux.HandleFunc("POST /api/mcp/servers/{name}/test", srv.handleTestMcpServer)

	mux.HandleFunc("GET /api/skills", srv.handleListSkills)
	mux.HandleFunc("GET /api/skills/{name}", srv.handleGetSkill)
	mux.HandleFunc("PUT /api/skills/{name}", srv.handlePutSkill)
	mux.HandleFunc("DELETE /api/skills/{name}", srv.handleDeleteSkill)

	mux.HandleFunc("GET /api/automations", srv.handleListAutomations)
	mux.HandleFunc("POST /api/automations", srv.handleCreateAutomation)
	mux.HandleFunc("PUT /api/automations/{id}", srv.handleUpdateAutomation)
	mux.HandleFunc("DELETE /api/automations/{id}", srv.handleDeleteAutomation)
	mux.HandleFunc("POST /api/automations/{id}/run", srv.handleRunAutomation)

	srv.startAutomationScheduler()
}
