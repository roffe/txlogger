package aichat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/ollama"
	"github.com/roffe/txlogger/pkg/widgets/internal/uitest"
)

// chatRequest is the part of an /api/chat body the tests look at.
type chatRequest struct {
	Model    string           `json:"model"`
	Messages []ollama.Message `json:"messages"`
	Tools    []ollama.Tool    `json:"tools"`
}

// chatServer stands in for Ollama's /api/chat: turn returns the NDJSON lines to
// stream for the nth request (0-based), and every request body is recorded.
type chatServer struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []chatRequest
}

func newChatServer(t *testing.T, turn func(n int) []string) *chatServer {
	t.Helper()
	s := &chatServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode chat request: %v", err)
		}
		s.mu.Lock()
		n := len(s.reqs)
		s.reqs = append(s.reqs, req)
		s.mu.Unlock()
		for _, line := range turn(n) {
			io.WriteString(w, line+"\n")
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *chatServer) requests() []chatRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.reqs)
}

// client returns a Config.Client getter for this server.
func (s *chatServer) client() func() *ollama.Client {
	return func() *ollama.Client { return ollama.New(s.URL, "qwen") }
}

// chunk encodes one NDJSON stream chunk.
func chunk(t *testing.T, msg ollama.Message, done bool) string {
	t.Helper()
	b, err := json.Marshal(struct {
		Message ollama.Message `json:"message"`
		Done    bool           `json:"done"`
	}{msg, done})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// say is a chunk of assistant text.
func say(t *testing.T, content string, done bool) string {
	t.Helper()
	return chunk(t, ollama.Message{Role: "assistant", Content: content}, done)
}

// callTools is a final chunk with optional text and the given tool calls.
func callTools(t *testing.T, content string, calls ...ollama.ToolCall) string {
	t.Helper()
	return chunk(t, ollama.Message{Role: "assistant", Content: content, ToolCalls: calls}, true)
}

func toolCall(name string, args map[string]any) ollama.ToolCall {
	var c ollama.ToolCall
	c.Function.Name = name
	c.Function.Arguments = args
	return c
}

// startChat builds the widget on the UI goroutine of a uitest app, where
// fyne.Do runs, and shows it in a test window.
func startChat(t *testing.T, cfg *Config) (*Widget, func(func())) {
	t.Helper()
	onUI := uitest.Start(t)
	var w *Widget
	onUI(func() {
		w = New(cfg)
		win := test.NewWindow(w)
		win.Resize(fyne.NewSize(600, 400))
	})
	return w, onUI
}

// ask types a prompt and presses Enter, then waits for the turn to finish.
func ask(t *testing.T, onUI func(func()), w *Widget, prompt string) {
	t.Helper()
	onUI(func() { test.Type(w.entry, prompt+"\n") })
	waitIdle(t, onUI, w)
}

// waitIdle polls on the UI goroutine until the chat is no longer busy.
func waitIdle(t *testing.T, onUI func(func()), w *Widget) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		var busy bool
		onUI(func() { busy = w.busy })
		if !busy {
			return
		}
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatal("chat still busy after 10s")
		}
	}
}

// waitFor blocks until ch is closed or fails the test after 10s.
func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

type chatState struct {
	transcript, pending, status, entry, rendered string
	busy, sendDisabled, stopDisabled             bool
	msgs                                         []ollama.Message
}

func snapshot(onUI func(func()), w *Widget) chatState {
	var s chatState
	onUI(func() {
		s = chatState{
			transcript:   w.transcript.String(),
			pending:      w.pending,
			status:       w.status.Text,
			entry:        w.entry.Text,
			rendered:     w.rt.String(),
			busy:         w.busy,
			sendDisabled: w.send.Disabled(),
			stopDisabled: w.stop.Disabled(),
			msgs:         slices.Clone(w.msgs),
		}
	})
	return s
}

// checkIdle asserts the controls are back in their idle state.
func checkIdle(t *testing.T, s chatState) {
	t.Helper()
	if s.busy || s.sendDisabled || !s.stopDisabled || s.status != "" {
		t.Errorf("after the turn: busy=%v send disabled=%v stop disabled=%v status=%q; want idle, send on, stop off, no status",
			s.busy, s.sendDisabled, s.stopDisabled, s.status)
	}
}

func toolNames(tools []ollama.Tool) []string {
	var out []string
	for _, tl := range tools {
		out = append(out, tl.Function.Name)
	}
	return out
}

// A tool-calling round trip: the model asks for read_symbol, the tool result
// goes back as a "tool" message with the system prompt and full history, and
// the final streamed answer lands in the transcript after the call line.
func TestConverseToolRoundTrip(t *testing.T) {
	srv := newChatServer(t, func(n int) []string {
		if n == 0 {
			return []string{callTools(t, "", toolCall("read_symbol", map[string]any{"name": "BFuelCal.Map"}))}
		}
		return []string{say(t, "Rich ", false), say(t, "at idle.", true)}
	})
	cfg := testConfig(t, nil)
	cfg.Client = srv.client()
	w, onUI := startChat(t, cfg)

	ask(t, onUI, w, "Why lean?")

	reqs := srv.requests()
	if len(reqs) != 2 {
		t.Fatalf("got %d chat requests, want 2", len(reqs))
	}
	first := reqs[0]
	if first.Model != "qwen" {
		t.Errorf("model = %q, want qwen", first.Model)
	}
	if want := []string{"list_symbols", "read_symbol", "log_info", "log_samples", "show_map_diff"}; !slices.Equal(toolNames(first.Tools), want) {
		t.Errorf("tools = %v, want %v", toolNames(first.Tools), want)
	}
	if len(first.Messages) != 2 || first.Messages[0].Role != "system" || first.Messages[1].Role != "user" || first.Messages[1].Content != "Why lean?" {
		t.Fatalf("first request messages = %+v, want system + user", first.Messages)
	}
	wantSys := string(t7Context) + "Current context:\n- ECU: T7\n- Binary: test.bin\n- Log: none loaded\n"
	if sys := first.Messages[0].Content; sys != wantSys {
		t.Errorf("system prompt tail = %q, want the T7 reference followed by %q",
			strings.TrimPrefix(sys, string(t7Context)), strings.TrimPrefix(wantSys, string(t7Context)))
	}

	second := reqs[1].Messages
	if len(second) != 4 {
		t.Fatalf("second request has %d messages, want system, user, assistant, tool: %+v", len(second), second)
	}
	if second[2].Role != "assistant" || len(second[2].ToolCalls) != 1 || second[2].ToolCalls[0].Function.Name != "read_symbol" {
		t.Errorf("history[2] = %+v, want the assistant's read_symbol call", second[2])
	}
	wantTool := ollama.Message{Role: "tool", ToolName: "read_symbol", Content: "BFuelCal.Map [Fuel correction factor] (4 values)\n" +
		"scale = 1 (raw ECU counts, unscaled)\n" +
		"columns = BFuelCal.AirXSP [mg/c]\n" +
		"rows = BFuelCal.RpmYSP [rpm]\n" +
		"\ny\\x,[0] 20,[1] 100\n" +
		"[0] 1000,100,110\n" +
		"[1] 4000,120,130\n"}
	if second[3].Role != wantTool.Role || second[3].ToolName != wantTool.ToolName || second[3].Content != wantTool.Content {
		t.Errorf("tool result = %+v, want %+v", second[3], wantTool)
	}

	s := snapshot(onUI, w)
	wantTranscript := "**You:** Why lean?\n\n> *read_symbol(name=BFuelCal.Map)*\n\n**Assistant:** Rich at idle.\n\n"
	if s.transcript != wantTranscript {
		t.Errorf("transcript = %q, want %q", s.transcript, wantTranscript)
	}
	if s.pending != "" || s.entry != "" {
		t.Errorf("pending = %q, entry = %q; want both cleared", s.pending, s.entry)
	}
	if !strings.Contains(s.rendered, "Rich at idle.") || strings.Contains(s.rendered, "Ask about the loaded binary") {
		t.Errorf("rendered text = %q, want the conversation in place of the intro", s.rendered)
	}
	if len(s.msgs) != 4 || s.msgs[3].Role != "assistant" || s.msgs[3].Content != "Rich at idle." {
		t.Errorf("history = %+v, want user, call, tool result, answer", s.msgs)
	}
	checkIdle(t, s)
}

// Text the model sends alongside a tool call is kept in the transcript, and a
// tool result starting with "Error:" is echoed to the user while other
// results (like an empty list_symbols match) are not.
func TestConverseShowsPreCallTextAndToolErrors(t *testing.T) {
	srv := newChatServer(t, func(n int) []string {
		if n == 0 {
			return []string{callTools(t, " Let me check. ",
				toolCall("read_symbol", map[string]any{"name": "Xyz.Map"}),
				toolCall("frobnicate", nil),
				toolCall("list_symbols", map[string]any{"filter": "zzz"}),
			)}
		}
		return []string{say(t, "Done.", true)}
	})
	cfg := testConfig(t, nil)
	cfg.Client = srv.client()
	w, onUI := startChat(t, cfg)

	ask(t, onUI, w, "look")

	want := "**You:** look\n\n" +
		"**Assistant:** Let me check.\n\n" +
		"> *read_symbol(name=Xyz.Map)*\n\n" +
		"> Error: no symbol named \"Xyz.Map\". Use list_symbols to find the right name.\n\n" +
		"> *frobnicate()*\n\n" +
		"> Error: unknown tool frobnicate\n\n" +
		"> *list_symbols(filter=zzz)*\n\n" +
		"**Assistant:** Done.\n\n"
	s := snapshot(onUI, w)
	if s.transcript != want {
		t.Errorf("transcript =\n%q\nwant\n%q", s.transcript, want)
	}

	reqs := srv.requests()
	if len(reqs) != 2 {
		t.Fatalf("got %d chat requests, want 2", len(reqs))
	}
	var names []string
	for _, m := range reqs[1].Messages {
		if m.Role == "tool" {
			names = append(names, m.ToolName)
		}
	}
	if want := []string{"read_symbol", "frobnicate", "list_symbols"}; !slices.Equal(names, want) {
		t.Errorf("tool results sent back for %v, want %v", names, want)
	}
	checkIdle(t, s)
}

// failedTurn is a turn that ends in an error other than a user cancel.
type failedTurn struct {
	name    string
	client  func(t *testing.T) func() *ollama.Client
	reqs    int    // chat requests the turn sends
	wantErr string // the error the user should see
}

func failedTurns() []failedTurn {
	const noModel = "no Ollama model configured — set one under Settings > AI"
	return []failedTurn{
		{
			name:    "nil client",
			client:  func(*testing.T) func() *ollama.Client { return func() *ollama.Client { return nil } },
			wantErr: noModel,
		},
		{
			name: "no model",
			client: func(t *testing.T) func() *ollama.Client {
				srv := newChatServer(t, func(int) []string { return nil })
				return func() *ollama.Client { return ollama.New(srv.URL, "") }
			},
			wantErr: noModel,
		},
		{
			name: "http error",
			client: func(t *testing.T) func() *ollama.Client {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "model not found", http.StatusNotFound)
				}))
				t.Cleanup(srv.Close)
				return func() *ollama.Client { return ollama.New(srv.URL, "qwen") }
			},
			reqs:    1,
			wantErr: "ollama 404 Not Found: model not found",
		},
		{
			// A model that never stops calling tools is cut off after maxTurns.
			name: "max turns",
			client: func(t *testing.T) func() *ollama.Client {
				srv := newChatServer(t, func(int) []string {
					return []string{callTools(t, "", toolCall("log_info", nil))}
				})
				return srv.client()
			},
			reqs:    maxTurns,
			wantErr: fmt.Sprintf("gave up after %d tool-calling turns without an answer", maxTurns),
		},
	}
}

// runFailedTurn asks one question that fails as tc describes and returns the
// chat state, what was sent to the host log and how many requests reached a
// server.
func runFailedTurn(t *testing.T, tc failedTurn) (chatState, []string, int) {
	t.Helper()
	var reqs atomic.Int32
	client := tc.client(t)
	cfg := testConfig(t, nil)
	cfg.Client = func() *ollama.Client {
		c := client()
		if c != nil {
			c.HTTP = &http.Client{Transport: countingTransport{&reqs}}
		}
		return c
	}
	var logged []string // appended on the UI goroutine
	cfg.Logger = func(s string) { logged = append(logged, s) }
	w, onUI := startChat(t, cfg)

	ask(t, onUI, w, "hello")

	var got []string
	onUI(func() { got = slices.Clone(logged) })
	return snapshot(onUI, w), got, int(reqs.Load())
}

// countingTransport counts the requests it forwards to the default transport.
type countingTransport struct{ n *atomic.Int32 }

func (c countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.n.Add(1)
	return http.DefaultTransport.RoundTrip(r)
}

// A failed turn gives the controls back, sends only the requests it needs
// (none without a model) and keeps the failed reply out of the history.
func TestFailedTurnRestoresControls(t *testing.T) {
	for _, tc := range failedTurns() {
		t.Run(tc.name, func(t *testing.T) {
			s, _, reqs := runFailedTurn(t, tc)
			if reqs != tc.reqs {
				t.Errorf("sent %d chat requests, want %d", reqs, tc.reqs)
			}
			if len(s.msgs) == 0 || s.msgs[0].Role != "user" || s.msgs[0].Content != "hello" {
				t.Errorf("history = %+v, want it to start with the question", s.msgs)
			}
			if tc.reqs <= 1 && len(s.msgs) != 1 {
				t.Errorf("history = %+v, want only the question", s.msgs)
			}
			checkIdle(t, s)
		})
	}
}

// A failed turn shows the error in the transcript and sends it to the host
// log, prefixed "ai chat: ".
func TestFailedTurnShowsError(t *testing.T) {
	for _, tc := range failedTurns() {
		t.Run(tc.name, func(t *testing.T) {
			s, logged, _ := runFailedTurn(t, tc)
			if !strings.HasSuffix(s.transcript, "\n> **Error:** "+tc.wantErr+"\n\n") {
				t.Errorf("transcript = %q, want it to end with the error %q", s.transcript, tc.wantErr)
			}
			if !slices.Equal(logged, []string{"ai chat: " + tc.wantErr}) {
				t.Errorf("logged %q, want the error once", logged)
			}
		})
	}
}

// While a turn runs the send button is off, stop is on, the status says
// "thinking…", and further submits are ignored; tapping stop cancels the
// request without reporting the cancellation as an error.
func TestStopCancelsTurn(t *testing.T) {
	arrived, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(arrived) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	cfg := testConfig(t, nil)
	cfg.Client = func() *ollama.Client { return ollama.New(srv.URL, "qwen") }
	var logged []string
	cfg.Logger = func(s string) { logged = append(logged, s) }
	w, onUI := startChat(t, cfg)

	onUI(func() {
		w.entry.SetText("slow question")
		test.Tap(w.send)
	})
	waitFor(t, arrived, "the chat request")

	s := snapshot(onUI, w)
	if !s.busy || !s.sendDisabled || s.stopDisabled || s.status != "thinking…" || s.entry != "" {
		t.Errorf("mid-turn: busy=%v send disabled=%v stop disabled=%v status=%q entry=%q",
			s.busy, s.sendDisabled, s.stopDisabled, s.status, s.entry)
	}
	onUI(func() {
		w.entry.SetText("impatient")
		w.submit()
	})
	if s := snapshot(onUI, w); s.entry != "impatient" || len(s.msgs) != 1 {
		t.Errorf("submit while busy: entry = %q, history = %+v; want it ignored", s.entry, s.msgs)
	}

	onUI(func() { test.Tap(w.stop) })
	waitIdle(t, onUI, w)

	s = snapshot(onUI, w)
	if strings.Contains(s.transcript, "Error") {
		t.Errorf("transcript = %q, want no error for a user cancel", s.transcript)
	}
	var got []string
	onUI(func() { got = slices.Clone(logged) })
	if len(got) != 0 {
		t.Errorf("logged %q, want nothing for a user cancel", got)
	}
	checkIdle(t, s)
}

// An empty or whitespace-only prompt does nothing: no request, no history,
// and the entry keeps its text.
func TestSubmitIgnoresBlankPrompt(t *testing.T) {
	cfg := testConfig(t, nil)
	called := false
	cfg.Client = func() *ollama.Client { called = true; return nil }
	w, onUI := startChat(t, cfg)

	onUI(func() {
		w.entry.SetText("   ")
		w.submit()
	})
	s := snapshot(onUI, w)
	if s.busy || s.transcript != "" || len(s.msgs) != 0 || s.entry != "   " || called {
		t.Errorf("blank submit: busy=%v transcript=%q history=%v entry=%q client called=%v",
			s.busy, s.transcript, s.msgs, s.entry, called)
	}
}

// reset forgets the conversation: the transcript, status and history are
// cleared, and the next request carries only the new question.
func TestResetClearsConversation(t *testing.T) {
	srv := newChatServer(t, func(int) []string { return []string{say(t, "Hi.", true)} })
	cfg := testConfig(t, nil)
	cfg.Client = srv.client()
	w, onUI := startChat(t, cfg)

	ask(t, onUI, w, "first")
	onUI(w.reset)

	s := snapshot(onUI, w)
	if s.transcript != "" || s.pending != "" || s.status != "" || len(s.msgs) != 0 || s.rendered != "" {
		t.Errorf("after reset: transcript=%q pending=%q status=%q history=%v rendered=%q; want all empty",
			s.transcript, s.pending, s.status, s.msgs, s.rendered)
	}

	ask(t, onUI, w, "second")
	reqs := srv.requests()
	if len(reqs) != 2 {
		t.Fatalf("got %d chat requests, want 2", len(reqs))
	}
	if msgs := reqs[1].Messages; len(msgs) != 2 || msgs[1].Content != "second" {
		t.Errorf("request after reset = %+v, want system + the new question only", msgs)
	}
}

// resetDuringToolCall presses reset while a show_map_diff call is running on
// the chat goroutine and returns the chat state once the turn has wound down,
// with the number of chat requests sent.
func resetDuringToolCall(t *testing.T) (chatState, int) {
	t.Helper()
	srv := newChatServer(t, func(n int) []string {
		if n == 0 {
			return []string{callTools(t, "", toolCall("show_map_diff", map[string]any{"name": "BFuelCal.Map", "changes": "0,0=1"}))}
		}
		return []string{say(t, "late answer", true)}
	})
	entered, release := make(chan struct{}), make(chan struct{})
	cfg := testConfig(t, nil)
	cfg.Client = srv.client()
	cfg.ShowMapDiff = func(string, []float64) error {
		close(entered)
		<-release
		return nil
	}
	w, onUI := startChat(t, cfg)

	onUI(func() { test.Type(w.entry, "make it richer\n") })
	waitFor(t, entered, "the show_map_diff call")
	if s := snapshot(onUI, w); s.status != "running show_map_diff" ||
		!strings.HasSuffix(s.transcript, "> *show_map_diff(changes=0,0=1, name=BFuelCal.Map)*\n\n") {
		t.Errorf("mid-call: status = %q, transcript = %q", s.status, s.transcript)
	}
	onUI(w.reset)
	close(release)
	waitIdle(t, onUI, w)
	return snapshot(onUI, w), len(srv.requests())
}

// Reset during a running tool call cancels the turn: no further request is
// sent and nothing from the old turn reaches the cleared transcript.
func TestResetDuringToolCallStopsTurn(t *testing.T) {
	s, n := resetDuringToolCall(t)
	if n != 1 {
		t.Errorf("got %d chat requests, want the turn to stop after the first", n)
	}
	if s.transcript != "" {
		t.Errorf("transcript after reset = %q, want empty", s.transcript)
	}
	checkIdle(t, s)
}

// Reset during a running tool call leaves no trace of the old conversation in
// the history sent with the next question.
func TestResetDuringToolCallClearsHistory(t *testing.T) {
	s, _ := resetDuringToolCall(t)
	if len(s.msgs) != 0 {
		t.Errorf("history after reset = %+v, want empty", s.msgs)
	}
}

// The system prompt prepends the T7 reference for T7 only and always lists
// the ECU, the binary and the log (or says they are not loaded).
func TestSystemPrompt(t *testing.T) {
	lf := newFakeLog(1000, 2000, 3000)
	empty := logfile.FromRows(nil, nil)
	for _, tc := range []struct {
		name, ecu, bin string
		lf             logfile.Logfile
		want           string
	}{
		{"t7 with bin and log", "T7", "a.bin", lf, "Current context:\n- ECU: T7\n- Binary: a.bin\n- Log: test.t7l, 3 samples\n"},
		{"t5 nothing loaded", "T5", "", nil, "Current context:\n- ECU: T5\n- Binary: none loaded\n- Log: none loaded\n"},
		{"empty log", "T8", "b.bin", empty, "Current context:\n- ECU: T8\n- Binary: b.bin\n- Log: none loaded\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &Widget{cfg: &Config{
				ECU:     func() string { return tc.ecu },
				BinName: func() string { return tc.bin },
				Log:     func() logfile.Logfile { return tc.lf },
				LogName: func() string { return "test.t7l" },
			}}
			got := w.systemPrompt()
			prefix := ""
			if tc.ecu == "T7" {
				prefix = string(t7Context)
			}
			if got != prefix+tc.want {
				t.Errorf("systemPrompt() tail = %q, want %q (T7 reference included: %v)",
					strings.TrimPrefix(got, string(t7Context)), tc.want, bytes.HasPrefix([]byte(got), t7Context))
			}
		})
	}
	if len(t7Context) == 0 {
		t.Error("T7 reference prompt is empty")
	}
}

// argSummary prints a call's arguments as key=value pairs in key order.
func TestArgSummary(t *testing.T) {
	for _, tc := range []struct {
		args map[string]any
		want string
	}{
		{nil, ""},
		{map[string]any{"name": "BFuelCal.Map"}, "name=BFuelCal.Map"},
		{map[string]any{"stride": float64(2), "channels": "a,b", "start": 0}, "channels=a,b, start=0, stride=2"},
	} {
		if got := argSummary(tc.args); got != tc.want {
			t.Errorf("argSummary(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

// A new chat is idle with the intro text; render shows the committed
// transcript plus any streaming text, and an unforced render inside the
// throttle interval is skipped.
func TestRender(t *testing.T) {
	test.NewTempApp(t)
	w := New(testConfig(t, nil))

	if w.send.Disabled() || !w.stop.Disabled() || w.busy {
		t.Errorf("new chat: send disabled=%v stop disabled=%v busy=%v; want send on, stop off, idle", w.send.Disabled(), w.stop.Disabled(), w.busy)
	}
	intro := w.rt.String()
	if !strings.Contains(intro, "Ask about the loaded binary") {
		t.Errorf("intro = %q", intro)
	}

	w.transcript.WriteString("**You:** hi\n\n")
	w.pending = "streaming"
	w.lastRender = time.Now().Add(time.Hour)
	w.render(false)
	if got := w.rt.String(); got != intro {
		t.Errorf("throttled render changed the text to %q", got)
	}

	w.render(true)
	got := w.rt.String()
	if !strings.Contains(got, "You: hi") || !strings.Contains(got, "Assistant: streaming") || strings.Contains(got, "**") {
		t.Errorf("rendered = %q, want the parsed transcript and the streaming reply", got)
	}

	w.pending = ""
	w.lastRender = time.Time{}
	w.render(false)
	if got := w.rt.String(); strings.Contains(got, "streaming") {
		t.Errorf("render after the interval = %q, want the streaming text gone", got)
	}
	if time.Since(w.lastRender) > time.Minute {
		t.Errorf("lastRender = %v, want it stamped by the render", w.lastRender)
	}
}
