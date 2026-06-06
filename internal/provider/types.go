// Package provider is the LLM client layer. It supports multiple backends
// (Cortex, OpenAI, Anthropic, Ollama) via a common interface. The Cortex
// and OpenAI/Ollama backends share an OpenAI-compatible HTTP shape; the
// Anthropic backend uses a similar shape pointed at Anthropic's gateway.
//
// Streaming is the primary mode. The session layer converts each chunk
// into a protocol.EventStreamChunk.
package provider

import (
	"context"
	"errors"
)

// Message represents one turn in a conversation.
type Message struct {
	Role       string    // "system" | "user" | "assistant" | "tool"
	Content    string
	ToolName   string // for role=="tool"
	ToolCallID string // for role=="tool"
	// ToolCalls is populated on assistant messages that invoked tools.
	ToolCalls []ToolCall
	// ToolCallID is the id of the assistant tool-call turn, used to link
	// tool results back to the original call.
}

// ToolCall is a model-issued tool invocation.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// Tool describes a function the model may invoke.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]ToolParam
}

// ToolParam is one parameter of a tool.
type ToolParam struct {
	Type        string // "string" | "number" | "boolean"
	Description string
	Required    bool
}

// ToolChoice controls how the model picks a tool.
//   - "auto"  — model decides
//   - "any"   — model must call at least one tool
//   - "none"  — model is told not to use tools
//   - specific tool name — model must call that tool
type ToolChoice struct {
	Mode string
	Name string
}

// Request is the input to a chat call.
type Request struct {
	Model        string
	Messages     []Message
	Tools        []Tool
	ToolChoice   ToolChoice
	Temperature  float64
	MaxTokens    int
	Stream       bool
	// Cortex-specific overrides (forwarded to the model server)
	ReasoningEffort string
	CortexPromptMode string
}

// Usage is the token accounting returned by the model.
type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	// Cortex-specific
	CortexUsage map[string]int64
}

// FinishReason is why the model stopped.
type FinishReason string

const (
	FinishStop   FinishReason = "stop"
	FinishLength FinishReason = "length"
	FinishTool   FinishReason = "tool_calls"
)

// Response is the result of a non-streaming chat.
type Response struct {
	Content      string
	ToolCalls    []ToolCall
	Usage        Usage
	FinishReason FinishReason
}

// Chunk is one delta in a streaming response.
type Chunk struct {
	Content   string
	ToolCalls []ToolCall // only on the final chunk where finish_reason=tool_calls
	Usage     Usage      // only on the final chunk
	// FinishReason is "stop" or "tool_calls" on the last chunk; empty
	// otherwise.
	FinishReason FinishReason
}

// Provider is the interface every LLM backend implements.
type Provider interface {
	Name() string
	// Chat sends a non-streaming request.
	Chat(ctx context.Context, req Request) (Response, error)
	// Stream sends a streaming request, calling onChunk for each delta.
	// Returns the final accumulated Response.
	Stream(ctx context.Context, req Request, onChunk func(Chunk)) (Response, error)
}

// ErrUnsupported is returned when a feature isn't supported by the backend.
var ErrUnsupported = errors.New("unsupported by this provider")
