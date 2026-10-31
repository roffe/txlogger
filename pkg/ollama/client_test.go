package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// New falls back to the local default server for an empty URL, drops a
// trailing slash so paths join cleanly, and sizes the context for tool output.
func TestNewDefaults(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", DefaultURL},
		{"http://gpu-box:11434/", "http://gpu-box:11434"},
		{"http://gpu-box:11434", "http://gpu-box:11434"},
	} {
		c := New(tc.in, "m")
		if c.BaseURL != tc.want {
			t.Errorf("New(%q).BaseURL = %q, want %q", tc.in, c.BaseURL, tc.want)
		}
		if c.Model != "m" || c.ContextSize != 16384 || c.HTTP == nil || c.Think {
			t.Errorf("New(%q) = %+v", tc.in, c)
		}
	}
}

// Object always emits a required array, never null, because Ollama rejects a
// schema with "required": null.
func TestObjectRequiredNeverNull(t *testing.T) {
	b, err := json.Marshal(Object(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"required":[]`) || !strings.Contains(string(b), `"type":"object"`) {
		t.Errorf("Object() = %s, want type object and an empty required array", b)
	}
}

// Chat POSTs JSON to /api/chat with the tools, the think flag and the
// configured context size in the request body.
func TestChatRequestShape(t *testing.T) {
	var (
		method, path, ctype string
		raw                 map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, ctype = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Write([]byte(`{"message":{"content":"ok"},"done":true}` + "\n"))
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "qwen")
	c.Think = true
	c.ContextSize = 32768
	tools := []Tool{NewTool("read_symbol", "Read one symbol", Object(nil, "name"))}
	history := []Message{
		{Role: "user", Content: "what is boost?"},
		{Role: "tool", ToolName: "read_symbol", Content: "1.2 bar"},
	}
	if _, err := c.Chat(context.Background(), history, tools, nil); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if method != http.MethodPost || path != "/api/chat" || ctype != "application/json" {
		t.Errorf("request = %s %s (%s), want POST /api/chat (application/json)", method, path, ctype)
	}
	if raw["think"] != true || raw["stream"] != true || raw["model"] != "qwen" {
		t.Errorf("think/stream/model = %v/%v/%v", raw["think"], raw["stream"], raw["model"])
	}
	if opts, _ := raw["options"].(map[string]any); opts["num_ctx"] != float64(32768) {
		t.Errorf("options = %v, want num_ctx 32768", raw["options"])
	}
	toolsJSON, _ := raw["tools"].([]any)
	if len(toolsJSON) != 1 {
		t.Fatalf("tools = %v, want one", raw["tools"])
	}
	tool, _ := toolsJSON[0].(map[string]any)
	fn, _ := tool["function"].(map[string]any)
	if tool["type"] != "function" || fn["name"] != "read_symbol" {
		t.Errorf("tool = %v, want a read_symbol function", tool)
	}
	msgs, _ := raw["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages = %v, want two", raw["messages"])
	}
	if m := msgs[1].(map[string]any); m["role"] != "tool" || m["tool_name"] != "read_symbol" {
		t.Errorf("tool result message = %v", m)
	}
	if _, ok := msgs[0].(map[string]any)["tool_name"]; ok {
		t.Error("tool_name sent on a user message, want it omitted when empty")
	}
}

// Chat accumulates the model's thinking separately from its answer, and only
// content deltas reach onToken.
func TestChatThinking(t *testing.T) {
	srv := ndjson(t, nil,
		`{"message":{"thinking":"the user "},"done":false}`,
		`{"message":{"thinking":"wants boost"},"done":false}`,
		`{"message":{"content":"1.2 bar"},"done":true}`,
	)
	defer srv.Close()

	var tokens []string
	msg, err := New(srv.URL, "m").Chat(context.Background(), nil, nil, func(s string) { tokens = append(tokens, s) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Role != "assistant" || msg.Thinking != "the user wants boost" || msg.Content != "1.2 bar" {
		t.Errorf("msg = %+v", msg)
	}
	if !slices.Equal(tokens, []string{"1.2 bar"}) {
		t.Errorf("onToken got %q, want only the content delta", tokens)
	}
}

// Chat stops reading at the chunk marked done and ignores anything after it.
func TestChatStopsAtDone(t *testing.T) {
	srv := ndjson(t, nil,
		`{"message":{"content":"a"},"done":true}`,
		`{"message":{"content":"b"},"done":false}`,
	)
	defer srv.Close()
	msg, err := New(srv.URL, "m").Chat(context.Background(), nil, nil, nil)
	if err != nil || msg.Content != "a" {
		t.Errorf("Chat = %q, %v; want a, nil", msg.Content, err)
	}
}

// A stream that ends without a done chunk returns what arrived, without error.
func TestChatEOFWithoutDone(t *testing.T) {
	srv := ndjson(t, nil, `{"message":{"content":"partial"},"done":false}`)
	defer srv.Close()
	msg, err := New(srv.URL, "m").Chat(context.Background(), nil, nil, nil)
	if err != nil || msg.Content != "partial" {
		t.Errorf("Chat = %q, %v; want partial, nil", msg.Content, err)
	}
}

// A non-200 reply is an error that carries the status and the server's body.
func TestChatHTTPStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"model requires more system memory"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := New(srv.URL, "m").Chat(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "more system memory") {
		t.Errorf("err = %v, want the status and body", err)
	}
}

// A chunk cut off mid-object aborts the turn with an unexpected-EOF decode
// error; the deltas before it have already been streamed to onToken.
func TestChatMalformedChunk(t *testing.T) {
	srv := ndjson(t, nil, `{"message":{"content":"half "},"done":false}`, `{"message":`)
	defer srv.Close()
	var tokens []string
	msg, err := New(srv.URL, "m").Chat(context.Background(), nil, nil, func(s string) { tokens = append(tokens, s) })
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
	}
	if msg.Role != "assistant" {
		t.Errorf("partial message role = %q", msg.Role)
	}
	if !slices.Equal(tokens, []string{"half "}) {
		t.Errorf("onToken got %q before the error, want [half ]", tokens)
	}
}

// Cancelling the context aborts a Chat whose request the server has received
// but not answered.
func TestChatContextCancel(t *testing.T) {
	release := make(chan struct{})
	arrived := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := New(srv.URL, "m").Chat(ctx, nil, nil, nil)
		done <- err
	}()
	select {
	case <-arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("request never reached the server")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Chat did not return after cancel")
	}
}

// An invalid base URL fails before any request is sent.
func TestChatBadURL(t *testing.T) {
	c := New("http://bad host", "m")
	if _, err := c.Chat(context.Background(), nil, nil, nil); err == nil {
		t.Error("Chat with an invalid URL succeeded")
	}
	if _, err := c.Models(context.Background()); err == nil {
		t.Error("Models with an invalid URL succeeded")
	}
}

// Models GETs /api/tags and returns the installed model names in server order.
func TestModels(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Write([]byte(`{"models":[{"name":"qwen3.6:27b","size":1},{"name":"llama3:8b"}]}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "").Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if method != http.MethodGet || path != "/api/tags" {
		t.Errorf("request = %s %s, want GET /api/tags", method, path)
	}
	if !slices.Equal(got, []string{"qwen3.6:27b", "llama3:8b"}) {
		t.Errorf("Models = %v", got)
	}
}

// Models returns an empty, non-nil list when nothing is installed, and works
// on a zero Client whose HTTP client is unset.
func TestModelsEmptyAndZeroClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	got, err := c.Models(context.Background())
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("Models = %#v, %v; want empty list, nil", got, err)
	}
}

// Models names the URL when the server cannot be reached and reports a body
// that is not JSON as an error.
func TestModelsErrors(t *testing.T) {
	down := httptest.NewServer(nil)
	addr := down.URL
	down.Close()
	if _, err := New(addr, "").Models(context.Background()); err == nil || !strings.Contains(err.Error(), addr) {
		t.Errorf("unreachable error = %v, want it to name %s", err, addr)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>not ollama</html>"))
	}))
	defer srv.Close()
	if got, err := New(srv.URL, "").Models(context.Background()); err == nil {
		t.Errorf("Models = %v, nil; want a decode error", got)
	}
}

// Models reports a non-200 reply as an error, like Chat does, so the settings
// page shows the server's error instead of "no models installed".
func TestModelsHTTPStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"server busy"}`))
	}))
	defer srv.Close()
	got, err := New(srv.URL, "").Models(context.Background())
	if err == nil {
		t.Errorf("Models = %#v, nil; want an error carrying the 503 status", got)
	}
}
