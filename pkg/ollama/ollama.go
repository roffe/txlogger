// Package ollama is a minimal client for the Ollama /api/chat endpoint with
// tool-calling support.
//
// ponytail: net/http + encoding/json instead of a vendored SDK. The endpoint is
// one POST with an NDJSON response; a dependency would be more code than this.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultURL = "http://localhost:11434"

// Message is one entry in the conversation. Tool results are sent back with
// Role "tool" and ToolName set to the function that produced them.
type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	Thinking  string     `json:"thinking,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

// Tool describes a function the model may call. Parameters is a JSON Schema
// object.
type Tool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

// NewTool builds a function tool from a name, description and JSON Schema.
func NewTool(name, description string, parameters any) Tool {
	var t Tool
	t.Type = "function"
	t.Function.Name = name
	t.Function.Description = description
	t.Function.Parameters = parameters
	return t
}

// Object is a helper for writing small JSON Schema objects inline.
func Object(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

type Client struct {
	// BaseURL of the Ollama server, e.g. http://localhost:11434.
	BaseURL string
	// Model tag, e.g. qwen3.6:27b-q4_K_M.
	Model string
	// Think enables the model's reasoning pass. Off by default: on a local
	// 27B it roughly doubles time-to-answer for little gain on lookups.
	Think bool
	// ContextSize maps to num_ctx. Tool results (map tables, log slices) are
	// bulky, so the default 4k is too small.
	ContextSize int
	HTTP        *http.Client
}

func New(baseURL, model string) *Client {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Client{
		BaseURL:     strings.TrimSuffix(baseURL, "/"),
		Model:       model,
		ContextSize: 16384,
		// No overall timeout: a local 27B can take minutes on a long answer.
		// Cancellation is the caller's ctx.
		HTTP: &http.Client{},
	}
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Tools    []Tool         `json:"tools,omitempty"`
	Stream   bool           `json:"stream"`
	Think    bool           `json:"think"`
	Options  map[string]any `json:"options,omitempty"`
}

type chatChunk struct {
	Message Message `json:"message"`
	Done    bool    `json:"done"`
	Error   string  `json:"error"`
}

// Chat streams one assistant turn and returns the accumulated message. onToken
// is called with each content delta as it arrives (may be nil). The returned
// message carries any tool calls the model made; the caller runs them and
// appends the results before calling Chat again.
func (c *Client) Chat(ctx context.Context, msgs []Message, tools []Tool, onToken func(string)) (Message, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.Model,
		Messages: msgs,
		Tools:    tools,
		Stream:   true,
		Think:    c.Think,
		Options:  map[string]any{"num_ctx": c.ContextSize},
	})
	if err != nil {
		return Message{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("ollama unreachable at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Message{}, fmt.Errorf("ollama %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	out := Message{Role: "assistant"}
	var content, thinking strings.Builder
	dec := json.NewDecoder(resp.Body)
	for {
		var chunk chatChunk
		if err := dec.Decode(&chunk); err == io.EOF {
			break
		} else if err != nil {
			return out, err
		}
		if chunk.Error != "" {
			return out, fmt.Errorf("ollama: %s", chunk.Error)
		}
		if chunk.Message.Content != "" {
			content.WriteString(chunk.Message.Content)
			if onToken != nil {
				onToken(chunk.Message.Content)
			}
		}
		thinking.WriteString(chunk.Message.Thinking)
		out.ToolCalls = append(out.ToolCalls, chunk.Message.ToolCalls...)
		if chunk.Done {
			break
		}
	}
	out.Content = content.String()
	out.Thinking = thinking.String()
	return out, nil
}

// Models lists the model tags installed on the server.
func (c *Client) Models(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(tags.Models))
	for _, m := range tags.Models {
		names = append(names, m.Name)
	}
	return names, nil
}
