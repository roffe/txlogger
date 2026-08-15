// Package aichat is a chat window backed by a local Ollama model that can query
// the loaded binary and log file through tool calls.
//
// Nothing leaves the machine: the model runs in Ollama on localhost (or wherever
// the user points it) and reads the bin and log through the same accessors the
// map viewer and log player use.
package aichat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/ollama"
)

// maxTurns bounds the tool-calling loop. A model that keeps calling tools
// without answering would otherwise run forever.
const maxTurns = 12

// renderInterval throttles markdown re-parsing while tokens stream in.
//
// ponytail: re-parses the whole transcript each tick, so a very long
// conversation eventually gets sluggish — switch to appending segments if that
// ever shows up in practice.
const renderInterval = 100 * time.Millisecond

// Config carries the accessors the chat needs from its host. Any of the
// getters may return a nil/zero value; the tools report that to the model as a
// plain "not loaded" message.
type Config struct {
	FW      func() symbol.SymbolCollection
	ECU     func() string
	Log     func() logfile.Logfile
	BinName func() string
	LogName func() string
	Client  func() *ollama.Client
	Logger  func(string)

	// ShowMapDiff opens a Before/After/Diff view of a map with the model's
	// proposed values. Called from the chat's worker goroutine, so the host
	// marshals to the UI thread itself. Nil disables the tool.
	ShowMapDiff func(name string, proposed []float64) error
}

type Widget struct {
	widget.BaseWidget
	cfg *Config

	rt     *widget.RichText
	scroll *container.Scroll
	entry  *widget.Entry
	send   *widget.Button
	stop   *widget.Button
	status *widget.Label

	content fyne.CanvasObject

	msgs       []ollama.Message
	transcript strings.Builder
	pending    string
	lastRender time.Time
	cancel     context.CancelFunc
	busy       bool
}

func New(cfg *Config) *Widget {
	w := &Widget{cfg: cfg}

	w.rt = widget.NewRichTextFromMarkdown("Ask about the loaded binary or log. The model can list and read symbols, and pull samples out of the log.")
	w.rt.Wrapping = fyne.TextWrapWord
	w.scroll = container.NewVScroll(w.rt)

	w.entry = widget.NewEntry()
	w.entry.SetPlaceHolder("Why is it lean at high load?")
	w.entry.OnSubmitted = func(string) { w.submit() }

	w.send = widget.NewButtonWithIcon("", theme.MailSendIcon(), w.submit)
	w.stop = widget.NewButtonWithIcon("", theme.CancelIcon(), w.abort)
	w.stop.Disable()

	w.status = widget.NewLabel("")
	w.status.Truncation = fyne.TextTruncateEllipsis

	reset := widget.NewButtonWithIcon("", theme.ContentClearIcon(), w.reset)

	input := container.NewBorder(nil, nil, reset, container.NewHBox(w.send, w.stop), w.entry)
	w.content = container.NewBorder(nil, container.NewVBox(input, w.status), nil, nil, w.scroll)

	w.ExtendBaseWidget(w)
	return w
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.content)
}

func (w *Widget) reset() {
	w.abort()
	w.msgs = nil
	w.transcript.Reset()
	w.pending = ""
	w.status.SetText("")
	w.render(true)
}

func (w *Widget) abort() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *Widget) submit() {
	prompt := strings.TrimSpace(w.entry.Text)
	if prompt == "" || w.busy {
		return
	}
	w.entry.SetText("")
	w.busy = true
	w.send.Disable()
	w.stop.Enable()

	w.msgs = append(w.msgs, ollama.Message{Role: "user", Content: prompt})
	w.appendf("**You:** %s\n\n", prompt)

	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go func() {
		defer cancel()
		err := w.converse(ctx)
		fyne.Do(func() {
			w.busy = false
			w.send.Enable()
			w.stop.Disable()
			w.status.SetText("")
			if err != nil && ctx.Err() == nil {
				w.appendf("\n> **Error:** %s\n\n", err)
				if w.cfg.Logger != nil {
					w.cfg.Logger("ai chat: " + err.Error())
				}
			}
			w.render(true)
		})
	}()
}

// converse runs the tool-calling loop until the model answers without asking
// for another tool. Runs on a goroutine — every UI touch goes through fyne.Do.
func (w *Widget) converse(ctx context.Context) error {
	client := w.cfg.Client()
	if client == nil || client.Model == "" {
		return fmt.Errorf("no Ollama model configured — set one under Settings > AI")
	}
	tools := w.tools()

	for turn := 0; turn < maxTurns; turn++ {
		fyne.Do(func() { w.status.SetText("thinking…") })

		msgs := append([]ollama.Message{{Role: "system", Content: w.systemPrompt()}}, w.msgs...)
		reply, err := client.Chat(ctx, msgs, tools, func(tok string) {
			fyne.Do(func() {
				w.pending += tok
				w.render(false)
			})
		})
		if err != nil {
			return err
		}
		w.msgs = append(w.msgs, reply)

		if len(reply.ToolCalls) == 0 {
			fyne.Do(func() {
				w.appendf("**Assistant:** %s\n\n", strings.TrimSpace(reply.Content))
			})
			return nil
		}

		// Some models emit a sentence before the call; keep it so the user can
		// follow the reasoning.
		if txt := strings.TrimSpace(reply.Content); txt != "" {
			fyne.Do(func() { w.appendf("**Assistant:** %s\n\n", txt) })
		} else {
			fyne.Do(func() { w.pending = "" })
		}

		for _, call := range reply.ToolCalls {
			name := call.Function.Name
			fyne.Do(func() {
				w.status.SetText("running " + name)
				// The arrow stays outside the italics: italic text renders with
				// NotoSans-Italic, which has no U+2192 and draws a tofu box.
				w.appendf("> *%s(%s)*\n\n", name, argSummary(call.Function.Arguments))
			})
			out := w.runTool(name, call.Function.Arguments)
			// Tool results are for the model, but a failed call otherwise
			// leaves the user staring at a call that did nothing.
			if err, _, _ := strings.Cut(out, "\n"); strings.HasPrefix(err, "Error:") {
				fyne.Do(func() { w.appendf("> %s\n\n", err) })
			}
			w.msgs = append(w.msgs, ollama.Message{Role: "tool", ToolName: name, Content: out})
		}
	}
	return fmt.Errorf("gave up after %d tool-calling turns without an answer", maxTurns)
}

func (w *Widget) systemPrompt() string {
	var b strings.Builder
	switch w.cfg.ECU() {
	case "T7":
		b.Write(t7Context)
	}

	b.WriteString("Current context:\n")

	fmt.Fprintf(&b, "- ECU: %s\n", w.cfg.ECU())
	if name := w.cfg.BinName(); name != "" {
		fmt.Fprintf(&b, "- Binary: %s\n", name)
	} else {
		b.WriteString("- Binary: none loaded\n")
	}
	if lf := w.cfg.Log(); lf != nil && lf.Len() > 0 {
		fmt.Fprintf(&b, "- Log: %s, %d samples\n", w.cfg.LogName(), lf.Len())
	} else {
		b.WriteString("- Log: none loaded\n")
	}

	return b.String()
}

// appendf commits text to the transcript and clears the streaming buffer.
func (w *Widget) appendf(format string, args ...any) {
	fmt.Fprintf(&w.transcript, format, args...)
	w.pending = ""
	w.render(true)
}

func (w *Widget) render(force bool) {
	if !force && time.Since(w.lastRender) < renderInterval {
		return
	}
	w.lastRender = time.Now()
	text := w.transcript.String()
	if w.pending != "" {
		text += "**Assistant:** " + w.pending
	}
	w.rt.ParseMarkdown(text)
	w.scroll.ScrollToBottom()
}

func argSummary(args map[string]any) string {
	parts := make([]string, 0, len(args))
	for _, k := range sortedKeys(args) {
		parts = append(parts, fmt.Sprintf("%s=%v", k, args[k]))
	}
	return strings.Join(parts, ", ")
}
