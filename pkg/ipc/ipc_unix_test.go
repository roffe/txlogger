//go:build !windows

package ipc

import (
	"encoding/gob"
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// useSocket points the package at a socket path inside a fresh temp dir for
// the duration of the test.
func useSocket(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s")
	if len(path) > 100 { // sun_path limit is 108 bytes
		t.Skipf("temp dir too long for a unix socket: %s", path)
	}
	old := socketFile
	socketFile = path
	t.Cleanup(func() { socketFile = old })
	return path
}

func startServer(t *testing.T, r Router) *Server {
	t.Helper()
	srv, err := NewServer(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	return srv
}

// exchange sends one gob message over a fresh connection and decodes the
// reply, returning io.EOF when the server closes without answering.
func exchange(t *testing.T, msg Message) (Message, error) {
	t.Helper()
	c, err := dial()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if err := gob.NewEncoder(c).Encode(msg); err != nil {
		t.Fatal(err)
	}
	var reply Message
	err = gob.NewDecoder(c).Decode(&reply)
	return reply, err
}

// A server built with the production router answers a gob "ping" with
// "pong", and ping() reports the instance as running.
func TestServerAnswersPing(t *testing.T) {
	useSocket(t)
	startServer(t, CreateIPCRouter(nil))

	reply, err := exchange(t, Message{Type: "ping"})
	if err != nil {
		t.Fatal(err)
	}
	if reply != (Message{Type: "pong"}) {
		t.Errorf("reply = %v, want pong", reply)
	}
	if !ping() {
		t.Error("ping() = false with a server running")
	}
}

// ping() is false when nothing listens, when the server closes without a
// reply, and when the reply is not "pong".
func TestPingFailures(t *testing.T) {
	t.Run("no socket", func(t *testing.T) {
		useSocket(t)
		if ping() {
			t.Error("ping() = true with no socket")
		}
	})
	t.Run("no handler", func(t *testing.T) {
		useSocket(t)
		startServer(t, Router{})
		if ping() {
			t.Error("ping() = true when the server has no ping handler")
		}
	})
	t.Run("wrong reply", func(t *testing.T) {
		useSocket(t)
		startServer(t, Router{"ping": func(string) *Message { return &Message{Type: "nope"} }})
		if ping() {
			t.Error("ping() = true for a non-pong reply")
		}
	})
}

// A handler returning nil sends nothing back; the connection is just closed.
func TestNilHandlerReplyClosesConnection(t *testing.T) {
	useSocket(t)
	got := make(chan string, 1)
	startServer(t, Router{"open": func(d string) *Message { got <- d; return nil }})

	if _, err := exchange(t, Message{Type: "open", Data: "x.csv"}); !errors.Is(err, io.EOF) {
		t.Errorf("decode after nil reply: %v, want EOF", err)
	}
	if d := <-got; d != "x.csv" {
		t.Errorf("handler got %q, want x.csv", d)
	}
}

// A client sending garbage or hanging up early does not stop the server
// from serving the next client.
func TestServerSurvivesBadClients(t *testing.T) {
	useSocket(t)
	startServer(t, CreateIPCRouter(nil))

	c, err := dial()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Write([]byte("not a gob stream at all"))
	c.Close()

	c, err = dial()
	if err != nil {
		t.Fatal(err)
	}
	c.Close()

	if !ping() {
		t.Error("server stopped answering after bad clients")
	}
}

// NewServer fails when the socket path is already taken.
func TestNewServerAddressInUse(t *testing.T) {
	path := useSocket(t)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if srv, err := NewServer(Router{}); err == nil {
		srv.Close()
		t.Fatal("NewServer succeeded on an occupied path")
	}
}

// Close is idempotent and removes the socket, so the instance no longer
// reports as running.
func TestServerClose(t *testing.T) {
	path := useSocket(t)
	srv, err := NewServer(CreateIPCRouter(nil))
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	srv.Close()
	if fileExists(path) {
		t.Error("socket file left behind after Close")
	}
	if ping() || IsRunning() {
		t.Error("closed server still reported as running")
	}
}

// IsRunning is false when there is no socket file.
func TestIsRunningNoSocket(t *testing.T) {
	path := useSocket(t)
	if IsRunning() {
		t.Error("IsRunning() = true with no socket")
	}
	if fileExists(path) {
		t.Error("IsRunning created the socket file")
	}
}

// IsRunning removes a socket file nobody listens on (left by a crashed
// instance) and reports false, so a new instance can listen there.
func TestIsRunningRemovesStaleSocket(t *testing.T) {
	path := useSocket(t)
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	l.SetUnlinkOnClose(false)
	l.Close()
	if !fileExists(path) {
		t.Fatal("setup: stale socket file missing")
	}

	if IsRunning() {
		t.Error("IsRunning() = true for a stale socket")
	}
	if fileExists(path) {
		t.Error("stale socket file not removed")
	}
	srv, err := NewServer(Router{})
	if err != nil {
		t.Fatalf("NewServer after stale cleanup: %v", err)
	}
	srv.Close()
}

// openRouter answers ping and forwards the data of every "open" request.
func openRouter(got chan<- string) Router {
	return Router{
		"ping": func(string) *Message { return &Message{Type: "pong"} },
		"open": func(d string) *Message { got <- d; return nil },
	}
}

func waitOpen(t *testing.T, got <-chan string) string {
	t.Helper()
	select {
	case d := <-got:
		return d
	case <-time.After(5 * time.Second):
		t.Fatal("running instance never received an open request")
		return ""
	}
}

// With an instance running, IsRunning reports true and asks it to show
// itself with an "open" request carrying no file.
func TestIsRunningSendsShow(t *testing.T) {
	useSocket(t)
	got := make(chan string, 1)
	startServer(t, openRouter(got))
	saved := flag.Args()
	if err := flag.CommandLine.Parse(nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = flag.CommandLine.Parse(saved) })

	if !IsRunning() {
		t.Fatal("IsRunning() = false with a server running")
	}
	if d := waitOpen(t, got); d != "" {
		t.Errorf("open data = %q, want empty", d)
	}
}

// The first command-line argument is forwarded to the running instance as
// the file to open.
func TestIsRunningForwardsFilename(t *testing.T) {
	useSocket(t)
	got := make(chan string, 1)
	startServer(t, openRouter(got))
	saved := flag.Args()
	if err := flag.CommandLine.Parse([]string{"/logs/run.csv"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = flag.CommandLine.Parse(saved) })

	if !IsRunning() {
		t.Fatal("IsRunning() = false with a server running")
	}
	if d := waitOpen(t, got); d != "/logs/run.csv" {
		t.Errorf("open data = %q, want /logs/run.csv", d)
	}
}
