package debug

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// useHome points the home directory at a temp dir, optionally creating the
// txlogger directory the log lives in, and resets the package so the next Log
// call opens the file afresh. It returns the log file path.
func useHome(t *testing.T, createDir bool) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, "txlogger")
	if createDir {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	reset := func() {
		if fh != nil {
			fh.Close()
		}
		fh = nil
		initOnce = sync.Once{}
	}
	reset()
	t.Cleanup(reset)
	return filepath.Join(dir, "txlogger.log")
}

// captureLog redirects the standard logger for the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

var lineRE = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{3}) (.*)$`)

// splitLine separates the millisecond timestamp prefix from the rest.
func splitLine(t *testing.T, line string) (time.Time, string) {
	t.Helper()
	m := lineRE.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("line %q lacks the timestamp prefix", line)
	}
	ts, err := time.ParseInLocation("2006/01/02 15:04:05.000", m[1], time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return ts, m[2]
}

// Log appends "<timestamp> <file>:<line> <msg>" to <home>/txlogger/txlogger.log,
// naming the caller of Log.
func TestLogWritesTimestampCallerAndMessage(t *testing.T) {
	path := useHome(t, true)

	before := time.Now().Truncate(time.Millisecond)
	_, _, line, _ := runtime.Caller(0)
	Log("hello world")
	after := time.Now()

	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines: %q", len(lines), lines)
	}
	ts, rest := splitLine(t, lines[0])
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not within [%v, %v]", ts, before, after)
	}
	if want := fmt.Sprintf("debug_test.go:%d hello world", line+1); rest != want {
		t.Errorf("got %q, want %q", rest, want)
	}
}

// The log file is opened in append mode, so earlier sessions are kept.
func TestLogAppendsToExistingFile(t *testing.T) {
	path := useHome(t, true)
	if err := os.WriteFile(path, []byte("previous session\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	Log("first")
	Log("second")

	lines := readLines(t, path)
	if len(lines) != 3 || lines[0] != "previous session" {
		t.Fatalf("got %q, want the old line followed by two new ones", lines)
	}
	for i, want := range []string{"first", "second"} {
		if _, rest := splitLine(t, lines[i+1]); !strings.HasSuffix(rest, " "+want) {
			t.Errorf("line %d = %q, want message %q", i+1, rest, want)
		}
	}
}

// logVia mimics MainWindow.Log: LogDepth(2) attributes the line to whoever
// called the wrapper.
func logVia(msg string) { LogDepth(2, msg) }

// LogDepth reports the caller the given number of frames up, and drops the
// file:line part when the stack is not that deep.
func TestLogDepth(t *testing.T) {
	path := useHome(t, true)

	_, _, line, _ := runtime.Caller(0)
	logVia("via wrapper")
	LogDepth(1, "direct")
	LogDepth(1000, "too deep")

	lines := readLines(t, path)
	if len(lines) != 3 {
		t.Fatalf("got %d lines: %q", len(lines), lines)
	}
	want := []string{
		fmt.Sprintf("debug_test.go:%d via wrapper", line+1),
		fmt.Sprintf("debug_test.go:%d direct", line+2),
		"too deep",
	}
	for i := range want {
		if _, rest := splitLine(t, lines[i]); rest != want[i] {
			t.Errorf("line %d = %q, want %q", i, rest, want[i])
		}
	}
}

// LogRaw writes the message verbatim as one line.
func TestLogRaw(t *testing.T) {
	path := useHome(t, true)
	Log("open") // opens the file
	LogRaw("raw message, no prefix")

	lines := readLines(t, path)
	if len(lines) != 2 || lines[1] != "raw message, no prefix" {
		t.Fatalf("got %q", lines)
	}
}

// When the txlogger directory is missing the logger does not create it or the
// file: messages are dropped with a notice on the standard logger, and Close
// stays safe.
func TestLogWithoutLogDirectory(t *testing.T) {
	path := useHome(t, false)
	out := captureLog(t)

	Log("lost")
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("log directory was created: %v", err)
	}
	if !strings.Contains(out.String(), "error opening file") || !strings.Contains(out.String(), "debug file not open") {
		t.Fatalf("standard log output = %q, want open error and drop notice", out.String())
	}
	Close() // main defers Close even when the file never opened
}

// Close flushes and closes the file; later messages no longer reach it.
func TestCloseStopsWriting(t *testing.T) {
	path := useHome(t, true)
	Log("before close")
	Close()
	LogRaw("after close")

	lines := readLines(t, path)
	if len(lines) != 1 || !strings.HasSuffix(lines[0], " before close") {
		t.Fatalf("got %q, want only the line logged before Close", lines)
	}
}

// Concurrent Log calls open the file once and never interleave lines.
func TestLogConcurrent(t *testing.T) {
	path := useHome(t, true)
	const workers, per = 8, 25

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range per {
				Log(fmt.Sprintf("worker %d message %d", w, i))
			}
		}()
	}
	wg.Wait()

	lines := readLines(t, path)
	if len(lines) != workers*per {
		t.Fatalf("got %d lines, want %d", len(lines), workers*per)
	}
	msgRE := regexp.MustCompile(`^debug_test\.go:\d+ worker \d message \d+$`)
	seen := make(map[string]bool)
	for _, l := range lines {
		_, rest := splitLine(t, l)
		if !msgRE.MatchString(rest) {
			t.Fatalf("mangled line %q", l)
		}
		seen[rest[strings.Index(rest, " ")+1:]] = true
	}
	if len(seen) != workers*per {
		t.Fatalf("got %d distinct messages, want %d", len(seen), workers*per)
	}
}

// When the home directory cannot be determined the logger reports it on the
// standard logger and writes txlogger.log in the working directory instead.
func TestLogFallsBackToWorkingDir(t *testing.T) {
	switch runtime.GOOS {
	case "windows", "plan9", "android", "ios":
		t.Skipf("os.UserHomeDir does not depend only on $HOME on %s", runtime.GOOS)
	}
	useHome(t, false)
	t.Setenv("HOME", "")
	wd := t.TempDir()
	t.Chdir(wd)
	out := captureLog(t)

	Log("in cwd")
	if got := out.String(); !strings.Contains(got, "error getting user home dir") || !strings.Contains(got, "$HOME is not defined") {
		t.Fatalf("standard log output = %q, want the home dir error", got)
	}
	lines := readLines(t, filepath.Join(wd, "txlogger.log"))
	if len(lines) != 1 || !strings.HasSuffix(lines[0], " in cwd") {
		t.Fatalf("got %q, want the message in ./txlogger.log", lines)
	}
}

// When the home directory cannot be determined the error itself is logged.
func TestLogReportsHomeDirError(t *testing.T) {
	switch runtime.GOOS {
	case "windows", "plan9", "android", "ios":
		t.Skipf("os.UserHomeDir does not depend only on $HOME on %s", runtime.GOOS)
	}
	useHome(t, false)
	t.Setenv("HOME", "")
	t.Chdir(t.TempDir()) // the logger falls back to the working directory
	out := captureLog(t)

	Log("x")
	got := out.String()
	if strings.Contains(got, "%w") || !strings.Contains(got, "error getting user home dir: $HOME is not defined") {
		t.Fatalf("standard log output = %q", got)
	}
}
