package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ndjson serves a canned /api/chat stream and records the request body.
func ndjson(t *testing.T, got *chatRequest, chunks ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			if err := json.NewDecoder(r.Body).Decode(got); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}
		for _, c := range chunks {
			w.Write([]byte(c + "\n"))
		}
	}))
}

func TestChatAccumulatesStream(t *testing.T) {
	var req chatRequest
	srv := ndjson(t, &req,
		`{"message":{"role":"assistant","content":"Boost "},"done":false}`,
		`{"message":{"role":"assistant","content":"looks high"},"done":false}`,
		`{"message":{"role":"assistant","content":"."},"done":true}`,
	)
	defer srv.Close()

	c := New(srv.URL, "test-model")
	var streamed strings.Builder
	msg, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, func(tok string) {
		streamed.WriteString(tok)
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "Boost looks high." {
		t.Errorf("content = %q, want %q", msg.Content, "Boost looks high.")
	}
	if streamed.String() != msg.Content {
		t.Errorf("streamed %q but accumulated %q — the UI would show something different from the history", streamed.String(), msg.Content)
	}
	if req.Model != "test-model" || !req.Stream {
		t.Errorf("request = %+v, want model test-model and stream true", req)
	}
	if req.Options["num_ctx"] != float64(16384) {
		t.Errorf("num_ctx = %v, want 16384", req.Options["num_ctx"])
	}
}

func TestChatCollectsToolCalls(t *testing.T) {
	srv := ndjson(t, nil,
		`{"message":{"role":"assistant","content":"Let me look. "},"done":false}`,
		`{"message":{"role":"assistant","tool_calls":[{"function":{"name":"read_symbol","arguments":{"name":"BFuelCal.Map"}}}]},"done":false}`,
		`{"message":{"role":"assistant"},"done":true}`,
	)
	defer srv.Close()

	msg, err := New(srv.URL, "m").Chat(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(msg.ToolCalls))
	}
	call := msg.ToolCalls[0].Function
	if call.Name != "read_symbol" || call.Arguments["name"] != "BFuelCal.Map" {
		t.Errorf("call = %s%v, want read_symbol{name: BFuelCal.Map}", call.Name, call.Arguments)
	}
	if msg.Content != "Let me look. " {
		t.Errorf("content = %q, want the pre-call text preserved", msg.Content)
	}
}

func TestChatSurfacesErrors(t *testing.T) {
	// In-band error (Ollama reports a bad model with HTTP 200 + an error field).
	srv := ndjson(t, nil, `{"error":"model \"nope\" not found, try pulling it first"}`)
	defer srv.Close()
	if _, err := New(srv.URL, "nope").Chat(context.Background(), nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Errorf("in-band error = %v, want it surfaced", err)
	}

	// Server down: the message must name the URL so the user knows where to look.
	down := httptest.NewServer(nil)
	addr := down.URL
	down.Close()
	_, err := New(addr, "m").Chat(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), addr) {
		t.Errorf("unreachable error = %v, want it to name %s", err, addr)
	}
}

func TestNewToolSchema(t *testing.T) {
	tool := NewTool("read_symbol", "Read one symbol", Object(map[string]any{
		"name": map[string]any{"type": "string"},
	}, "name"))

	b, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Ollama rejects a tool without type/function/parameters in this exact shape.
	for _, want := range []string{`"type":"function"`, `"name":"read_symbol"`, `"required":["name"]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
}
