package ipc

import (
	"os"
	"path/filepath"
	"testing"
)

// isLogfile accepts only .csv names, case-insensitively; other log formats
// and binaries are not opened as logs over IPC.
func TestIsLogfile(t *testing.T) {
	cases := map[string]bool{
		"log.csv":         true,
		"/a/b/LOG.CSV":    true,
		"run.Csv":         true,
		"log.t7l":         false,
		"firmware.bin":    false,
		"csv":             false,
		"dir.csv/log.txt": false,
		"":                false,
	}
	for name, want := range cases {
		if got := isLogfile(name); got != want {
			t.Errorf("isLogfile(%q) = %v, want %v", name, got, want)
		}
	}
}

// Message.String renders "type: data".
func TestMessageString(t *testing.T) {
	if got := (Message{Type: "open", Data: "/tmp/x.bin"}).String(); got != "open: /tmp/x.bin" {
		t.Errorf("got %q", got)
	}
	if got := (Message{Type: "ping"}).String(); got != "ping: " {
		t.Errorf("got %q", got)
	}
}

// fileExists reports whether a path exists.
func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !fileExists(f) || !fileExists(dir) {
		t.Error("existing file/dir reported missing")
	}
	if fileExists(filepath.Join(dir, "missing")) {
		t.Error("missing file reported existing")
	}
}

// The production router answers "ping" with an empty "pong" without
// touching the main window, and also routes "open".
func TestRouterPing(t *testing.T) {
	r := CreateIPCRouter(nil)
	got := r["ping"]("anything")
	if got == nil || *got != (Message{Type: "pong"}) {
		t.Errorf("ping handler = %v, want pong", got)
	}
	if _, ok := r["open"]; !ok {
		t.Error("router has no open handler")
	}
}
