package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenAICompat is a Provider that speaks the OpenAI /v1/chat/completions
// shape. It works for Cortex, OpenAI, Ollama, OpenRouter, and any other
// OpenAI-compatible gateway. The base URL is configurable so a single
// implementation can target any backend.
type OpenAICompat struct {
	name    string
	apiKey  string
	baseURL string
	client  *http.Client
	// headerHook, if non-nil, is invoked after the standard headers
	// (Authorization, Content-Type) have been set, giving the caller a
	// chance to add provider-specific headers such as
	// `chatgpt-account-id` for ChatGPT-subscription logins.
	headerHook func(*http.Request)
}

// NewOpenAICompat constructs a provider. baseURL is the full base URL
// (e.g. "http://127.0.0.1:8000/v1") and apiKey may be empty for backends
// that don't require one (e.g. local Ollama).
func NewOpenAICompat(name, apiKey, baseURL string) *OpenAICompat {
	return &OpenAICompat{
		name:    name,
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 10 * time.Minute},
	}
}

// WithHeaderHook installs a per-request header mutator. Used by the
// codex provider to inject `chatgpt-account-id`.
func (p *OpenAICompat) WithHeaderHook(fn func(*http.Request)) *OpenAICompat {
	p.headerHook = fn
	return p
}

func (p *OpenAICompat) Name() string { return p.name }

// oaiMessage is the JSON shape for a chat message.
type oaiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	Name       string        `json:"name,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
}

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON-encoded
	} `json:"function"`
}

type oaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

type oaiRequest struct {
	Model            string         `json:"model"`
	Messages         []oaiMessage   `json:"messages"`
	Tools            []oaiTool      `json:"tools,omitempty"`
	ToolChoice       any            `json:"tool_choice,omitempty"`
	Temperature      float64        `json:"temperature,omitempty"`
	MaxTokens        int            `json:"max_tokens,omitempty"`
	Stream           bool           `json:"stream"`
	StreamOptions    map[string]any `json:"stream_options,omitempty"`
	ReasoningEffort  string         `json:"reasoning_effort,omitempty"`
	CortexPromptMode string         `json:"cortex_prompt_mode,omitempty"`
}

type oaiResponse struct {
	Choices []struct {
		Message      oaiMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
		Delta        oaiMessage `json:"delta"`
	} `json:"choices"`
	Usage *oaiUsage `json:"usage,omitempty"`
}

type oaiUsage struct {
	PromptTokens     int64            `json:"prompt_tokens"`
	CompletionTokens int64            `json:"completion_tokens"`
	TotalTokens      int64            `json:"total_tokens"`
	CortexUsage      map[string]int64 `json:"cortex_usage,omitempty"`
}

func toOaiMessages(msgs []Message) []oaiMessage {
	out := make([]oaiMessage, 0, len(msgs))
	for _, m := range msgs {
		om := oaiMessage{
			Role:       m.Role,
			Content:    m.Content,
			Name:       m.ToolName,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				oc := oaiToolCall{ID: tc.ID, Type: "function"}
				oc.Function.Name = tc.Name
				// Arguments must be a JSON-encoded string
				if buf, err := json.Marshal(tc.Arguments); err == nil {
					oc.Function.Arguments = string(buf)
				} else {
					oc.Function.Arguments = "{}"
				}
				om.ToolCalls = append(om.ToolCalls, oc)
			}
		}
		out = append(out, om)
	}
	return out
}

func toOaiTools(tools []Tool) []oaiTool {
	out := make([]oaiTool, 0, len(tools))
	for _, t := range tools {
		ot := oaiTool{Type: "function"}
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		props := map[string]interface{}{}
		required := []string{}
		for k, p := range t.Parameters {
			props[k] = oaiParamSchema(p)
			if p.Required {
				required = append(required, k)
			}
		}
		params := map[string]interface{}{
			"type":       "object",
			"properties": props,
		}
		if len(required) > 0 {
			params["required"] = required
		}
		ot.Function.Parameters = params
		out = append(out, ot)
	}
	return out
}

func oaiParamSchema(p ToolParam) map[string]interface{} {
	schema := map[string]interface{}{"type": p.Type}
	if p.Description != "" {
		schema["description"] = p.Description
	}
	if p.Items != nil {
		schema["items"] = oaiParamSchema(*p.Items)
	}
	if len(p.Properties) > 0 {
		props := map[string]interface{}{}
		required := []string{}
		for k, child := range p.Properties {
			props[k] = oaiParamSchema(child)
			if child.Required {
				required = append(required, k)
			}
		}
		schema["properties"] = props
		if len(required) > 0 {
			schema["required"] = required
		}
	}
	return schema
}

func toOaiToolChoice(tc ToolChoice) any {
	switch tc.Mode {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "none":
		return "none"
	case "":
		return nil
	default:
		return map[string]any{"type": "function", "function": map[string]any{"name": tc.Name}}
	}
}

func (p *OpenAICompat) buildRequest(req Request, stream bool) (*http.Request, error) {
	requestBaseURL, requestModel := p.routeAwareBaseURLAndModel(req.Model)
	oaiReq := oaiRequest{
		Model:    requestModel,
		Messages: toOaiMessages(req.Messages),
		Tools:    toOaiTools(req.Tools),
		Stream:   stream,
	}
	if tc := toOaiToolChoice(req.ToolChoice); tc != nil {
		oaiReq.ToolChoice = tc
	}
	if req.Temperature > 0 {
		oaiReq.Temperature = req.Temperature
	}
	if req.MaxTokens > 0 {
		oaiReq.MaxTokens = req.MaxTokens
	}
	if stream {
		oaiReq.StreamOptions = map[string]any{"include_usage": true}
	}
	if req.ReasoningEffort != "" {
		oaiReq.ReasoningEffort = req.ReasoningEffort
	}
	if req.CortexPromptMode != "" {
		oaiReq.CortexPromptMode = req.CortexPromptMode
	}
	body, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest("POST", requestBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	if p.headerHook != nil {
		p.headerHook(httpReq)
	}
	return httpReq, nil
}

func (p *OpenAICompat) routeAwareBaseURLAndModel(model string) (string, string) {
	baseURL := strings.TrimRight(p.baseURL, "/")
	model = strings.TrimSpace(model)
	if !p.isOpenGateway() {
		return baseURL, model
	}
	upstream, rawModel, ok := strings.Cut(model, "/")
	upstream = strings.ToLower(strings.TrimSpace(upstream))
	rawModel = strings.TrimSpace(rawModel)
	if !ok || upstream == "" || rawModel == "" {
		if inferred := inferOpenGatewayUpstream(model); inferred != "" {
			upstream = inferred
			rawModel = model
		} else {
			return baseURL, model
		}
	}
	if !strings.HasSuffix(strings.ToLower(baseURL), "/"+upstream) {
		baseURL += "/" + upstream
	}
	return baseURL, rawModel
}

func inferOpenGatewayUpstream(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(model, "minimax"):
		return "minimax"
	case strings.Contains(model, "mimo"):
		return "xiaomi"
	case strings.Contains(model, "gemini"):
		return "google"
	case strings.Contains(model, "qwen"):
		return "qwen"
	case strings.Contains(model, "nemotron") || strings.Contains(model, "nvidia"):
		return "nvidia"
	default:
		return ""
	}
}

func (p *OpenAICompat) shouldUseNonStreaming(model string) bool {
	return p.isOpenGateway()
}

func (p *OpenAICompat) isOpenGateway() bool {
	return strings.EqualFold(strings.TrimSpace(p.name), "opengateway") || isOpenGatewayBaseURL(p.baseURL)
}

func isOpenGatewayBaseURL(baseURL string) bool {
	return strings.Contains(strings.ToLower(baseURL), "opengateway")
}

func (p *OpenAICompat) Chat(ctx context.Context, req Request) (Response, error) {
	httpReq, err := p.buildRequest(req, false)
	if err != nil {
		return Response{}, err
	}
	httpReq = httpReq.WithContext(ctx)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		return Response{}, fmt.Errorf("chat: HTTP %d: %s", resp.StatusCode, string(buf))
	}
	var oai oaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&oai); err != nil {
		return Response{}, err
	}
	if len(oai.Choices) == 0 {
		return Response{}, fmt.Errorf("chat: no choices in response")
	}
	c := oai.Choices[0]
	r := Response{
		Content:      c.Message.Content,
		FinishReason: FinishReason(c.FinishReason),
	}
	for _, tc := range c.Message.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]any{"_raw": tc.Function.Arguments}
		}
		r.ToolCalls = append(r.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: args})
	}
	if oai.Usage != nil {
		r.Usage = Usage{
			PromptTokens:     oai.Usage.PromptTokens,
			CompletionTokens: oai.Usage.CompletionTokens,
			TotalTokens:      oai.Usage.TotalTokens,
			CortexUsage:      oai.Usage.CortexUsage,
		}
	}
	return r, nil
}

// sseEvent represents a single Server-Sent Event line(s).
type sseEvent struct {
	Event string
	Data  string
}

// readSSE reads SSE events from the response body. It splits on \n\n and
// yields each (event, data) pair.
func readSSE(r io.Reader, onEvent func(sseEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var ev sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			ev.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			ev.Data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if ev.Data != "" {
				if err := onEvent(ev); err != nil {
					return err
				}
			}
			ev = sseEvent{}
		}
	}
	return scanner.Err()
}

func (p *OpenAICompat) Stream(ctx context.Context, req Request, onChunk func(Chunk)) (Response, error) {
	if p.shouldUseNonStreaming(req.Model) {
		resp, err := p.Chat(ctx, req)
		if err != nil {
			return resp, err
		}
		if resp.Content != "" || resp.FinishReason != "" || resp.Usage.TotalTokens > 0 || resp.Usage.PromptTokens > 0 || len(resp.ToolCalls) > 0 {
			onChunk(Chunk{
				Content:      resp.Content,
				ToolCalls:    resp.ToolCalls,
				Usage:        resp.Usage,
				FinishReason: resp.FinishReason,
			})
		}
		return resp, nil
	}
	httpReq, err := p.buildRequest(req, true)
	if err != nil {
		return Response{}, err
	}
	httpReq = httpReq.WithContext(ctx)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		return Response{}, fmt.Errorf("stream: HTTP %d: %s", resp.StatusCode, string(buf))
	}

	var full Response
	toolArgs := map[string]*oaiToolCall{} // by id, accumulates JSON args
	toolArgsStr := map[string]string{}    // raw arg strings

	err = readSSE(resp.Body, func(ev sseEvent) error {
		if ev.Data == "[DONE]" {
			return nil
		}
		var oai oaiResponse
		if err := json.Unmarshal([]byte(ev.Data), &oai); err != nil {
			return nil // skip malformed lines
		}
		for _, c := range oai.Choices {
			chunk := Chunk{}
			if c.Delta.Content != "" {
				chunk.Content = c.Delta.Content
				full.Content += c.Delta.Content
			}
			for _, tc := range c.Delta.ToolCalls {
				// The ID is only on the first chunk; subsequent chunks just append
				// to the same tool call's arguments.
				if tc.ID != "" {
					tt := tc
					toolArgs[tc.ID] = &tt
					toolArgsStr[tc.ID] = tt.Function.Arguments
				} else if tc.Function.Name != "" && len(c.Delta.ToolCalls) > 0 {
					// Some backends omit the id on continuation chunks but send
					// the function name + arguments to append.
					// We need to find the in-progress tool call. Use the index
					// hint: usually only one tool call is active at a time, so
					// the last one in toolArgs is the active one.
					var lastID string
					for id := range toolArgs {
						lastID = id
					}
					if lastID != "" {
						toolArgsStr[lastID] += tc.Function.Arguments
						toolArgs[lastID].Function.Arguments = toolArgsStr[lastID]
					}
				} else {
					var lastID string
					for id := range toolArgs {
						lastID = id
					}
					if lastID != "" {
						toolArgsStr[lastID] += tc.Function.Arguments
						toolArgs[lastID].Function.Arguments = toolArgsStr[lastID]
					}
				}
			}
			if c.FinishReason != "" {
				chunk.FinishReason = FinishReason(c.FinishReason)
				full.FinishReason = chunk.FinishReason
			}
			if oai.Usage != nil {
				chunk.Usage = Usage{
					PromptTokens:     oai.Usage.PromptTokens,
					CompletionTokens: oai.Usage.CompletionTokens,
					TotalTokens:      oai.Usage.TotalTokens,
					CortexUsage:      oai.Usage.CortexUsage,
				}
				full.Usage = chunk.Usage
			}
			if chunk.Content != "" || chunk.FinishReason != "" || chunk.Usage.TotalTokens > 0 {
				onChunk(chunk)
			}
		}
		// Usage-only chunks have no choices
		if oai.Usage != nil && len(oai.Choices) == 0 {
			chunk := Chunk{Usage: Usage{
				PromptTokens:     oai.Usage.PromptTokens,
				CompletionTokens: oai.Usage.CompletionTokens,
				TotalTokens:      oai.Usage.TotalTokens,
				CortexUsage:      oai.Usage.CortexUsage,
			}}
			full.Usage = chunk.Usage
			onChunk(chunk)
		}
		return nil
	})
	if err != nil {
		return full, err
	}

	// Build the final ToolCalls list
	for id, tc := range toolArgs {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]any{"_raw": tc.Function.Arguments}
		}
		full.ToolCalls = append(full.ToolCalls, ToolCall{ID: id, Name: tc.Function.Name, Arguments: args})
	}
	return full, nil
}

// FetchModels lists model IDs from an OpenAI-compatible /models endpoint.
func FetchModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	models, err := fetchModelsOnce(ctx, baseURL, apiKey)
	if err != nil {
		return nil, err
	}
	return models, nil
}

// FetchModelsFromCandidates tries multiple OpenAI-compatible base URLs and
// returns model IDs from the first successful /models endpoint. If the first
// endpoint returns a gateway-routing 404, it keeps trying provider-scoped
// candidates and aggregates every successful response. This supports gateways
// that require /v1/<provider>/models rather than plain /v1/models.
func FetchModelsFromCandidates(ctx context.Context, apiKey string, baseURLs ...string) ([]string, string, error) {
	seen := map[string]bool{}
	var candidates []string
	for _, candidate := range baseURLs {
		candidate = strings.TrimRight(strings.TrimSpace(candidate), "/")
		if candidate == "" || seen[strings.ToLower(candidate)] {
			continue
		}
		seen[strings.ToLower(candidate)] = true
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("models: empty base URL")
	}

	models, err := fetchModelsOnce(ctx, candidates[0], apiKey)
	if err == nil {
		return qualifyOpenGatewayModels(candidates[0], models), candidates[0], nil
	}
	if isModelsInsufficientCreditsError(err) {
		if fallback := openGatewayPublicModels(candidates); len(fallback) > 0 {
			return fallback, candidates[0], nil
		}
	}
	if !isModelsNotFoundError(err) {
		return nil, candidates[0], err
	}
	lastErr := err

	modelSeen := map[string]bool{}
	var out []string
	usedBaseURL := ""
	hadSuccess := false
	var firstNonNotFoundErr error
	var firstNonNotFoundBaseURL string
	for _, candidate := range candidates[1:] {
		models, err := fetchModelsOnce(ctx, candidate, apiKey)
		if err != nil {
			lastErr = err
			if !isModelsNotFoundError(err) && firstNonNotFoundErr == nil {
				firstNonNotFoundErr = err
				firstNonNotFoundBaseURL = candidate
			}
			continue
		}
		hadSuccess = true
		if usedBaseURL == "" {
			usedBaseURL = candidate
		}
		for _, model := range qualifyOpenGatewayModels(candidate, models) {
			model = strings.TrimSpace(model)
			if model == "" || modelSeen[model] {
				continue
			}
			modelSeen[model] = true
			out = append(out, model)
		}
	}
	if len(out) > 0 || hadSuccess {
		return out, usedBaseURL, nil
	}
	if firstNonNotFoundErr != nil {
		if isModelsInsufficientCreditsError(firstNonNotFoundErr) {
			if fallback := openGatewayPublicModels(candidates); len(fallback) > 0 {
				return fallback, candidates[0], nil
			}
		}
		return nil, firstNonNotFoundBaseURL, firstNonNotFoundErr
	}
	return nil, "", lastErr
}

func fetchModelsOnce(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	models, err := fetchModelsOnceWithAPIKey(ctx, baseURL, apiKey)
	if err != nil && apiKey != "" && isModelsAuthHeaderRecoverableError(err) {
		modelsWithoutAuth, unauthErr := fetchModelsOnceWithAPIKey(ctx, baseURL, "")
		if unauthErr == nil {
			return modelsWithoutAuth, nil
		}
	}
	return models, err
}

// IsModelsInsufficientCreditsError reports whether a model-listing request was
// rejected for account credits/quota rather than because the models route is
// unavailable.
func IsModelsInsufficientCreditsError(err error) bool {
	return isModelsInsufficientCreditsError(err)
}

func isModelsNotFoundError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "HTTP 404")
}

func isModelsAuthHeaderRecoverableError(err error) bool {
	return isModelsUnauthorizedError(err) || isModelsInsufficientCreditsError(err)
}

func isModelsUnauthorizedError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "HTTP 401")
}

func isModelsInsufficientCreditsError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "http 402") || strings.Contains(msg, "insufficient_credits") || strings.Contains(msg, "insufficient_quota")
}

func openGatewayPublicModels(baseURLs []string) []string {
	isOpenGateway := false
	for _, baseURL := range baseURLs {
		if isOpenGatewayBaseURL(baseURL) {
			isOpenGateway = true
			break
		}
	}
	if !isOpenGateway {
		return nil
	}
	return []string{
		"xiaomi/mimo-v2.5-pro",
		"xiaomi/mimo-v2.5",
		"xiaomi/mimo-v2-flash",
		"google/gemini-3.1-flash-lite",
		"minimax/minimax-m3",
		"qwen/qwen3.7-max",
		"nvidia/nemotron-3-ultra-550b-a55b:free",
	}
}

func qualifyOpenGatewayModels(baseURL string, models []string) []string {
	scope := openGatewayScopeFromBaseURL(baseURL)
	if scope == "" {
		return models
	}
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if strings.Contains(model, "/") {
			out = append(out, model)
			continue
		}
		out = append(out, scope+"/"+model)
	}
	return out
}

func openGatewayScopeFromBaseURL(baseURL string) string {
	if !isOpenGatewayBaseURL(baseURL) {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, part := range parts {
		if strings.EqualFold(part, "v1") && i+1 < len(parts) {
			return strings.ToLower(strings.TrimSpace(parts[i+1]))
		}
	}
	return ""
}

func fetchModelsOnceWithAPIKey(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, fmt.Errorf("models: empty base URL")
	}
	httpReq, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("models: HTTP %d: %s", resp.StatusCode, string(buf))
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id != "" && !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	for _, item := range payload.Models {
		id := strings.TrimSpace(item.ID)
		if id != "" && !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out, nil
}
