package mbt

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// The help text is embedded and must actually render: a markdown file that
// parses to nothing would give a blank Help window and nobody would notice.
func TestHelpRenders(t *testing.T) {
	if !strings.HasPrefix(helpMD, "# T7 MBT ignition analyser") {
		t.Fatalf("MBT.md not embedded, got %.40q", helpMD)
	}
	rt := widget.NewRichTextFromMarkdown(helpMD)
	if len(rt.Segments) < 50 {
		t.Errorf("markdown rendered to %d segments, expected the whole document", len(rt.Segments))
	}
	// The doc leans on tables (descriptor drift, what the binary supplies,
	// the view list). Fyne renders markdown tables only because this fork
	// enables goldmark's Table extension — if that ever goes away they
	// would silently degrade to runs of pipe characters.
	var tables int
	for _, s := range rt.Segments {
		if _, ok := s.(*widget.TableSegment); ok {
			tables++
		}
	}
	if tables < 3 {
		t.Errorf("rendered %d of MBT.md's 3 tables as tables", tables)
	}
}

// The Help button must go through the host's window opener when one is
// supplied, or the help silently opens a stray top-level window outside the
// app's window manager.
func TestHelpUsesHostWindow(t *testing.T) {
	var gotTitle string
	var gotContent fyne.CanvasObject
	w := &Widget{cfg: &Config{
		OpenWindow: func(title string, content fyne.CanvasObject) {
			gotTitle, gotContent = title, content
		},
	}}
	w.openHelp()

	if gotTitle != helpTitle {
		t.Errorf("opened window titled %q, want %q", gotTitle, helpTitle)
	}
	if gotContent == nil {
		t.Error("opened window with no content")
	}
}
