package txbridge

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

// ResolveAddress keeps the host from a tcp:// port string, forces port 1337
// and falls back to DefaultAddress for anything else.
func TestResolveAddress(t *testing.T) {
	cases := map[string]string{
		"":                       DefaultAddress,
		"COM3":                   DefaultAddress, // serial port, not tcp
		"tcp://":                 DefaultAddress,
		"tcp://10.0.0.5":         "10.0.0.5:1337", // port appended
		"tcp://10.0.0.5:8080":    "10.0.0.5:1337", // wrong port overwritten
		"tcp://192.168.4.1:1337": "192.168.4.1:1337",
	}
	for in, want := range cases {
		if got := ResolveAddress(in); got != want {
			t.Errorf("ResolveAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

// pipeClient returns a Client wired to one end of an in-memory pipe and the
// other end, which the test drives as the txbridge.
func pipeClient(t *testing.T) (*Client, net.Conn) {
	t.Helper()
	cl, dev := net.Pipe()
	t.Cleanup(func() {
		cl.Close()
		dev.Close()
	})
	return &Client{conn: cl}, dev
}

// readN reads exactly n bytes from the device end of the pipe.
func readN(t *testing.T, dev net.Conn, n int) []byte {
	t.Helper()
	dev.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, n)
	if _, err := io.ReadFull(dev, buf); err != nil {
		t.Fatalf("device read: %v", err)
	}
	return buf
}

// SendCommand writes a bare command byte when data is nil and a framed
// command (cmd, len, data..., 8-bit sum of data) otherwise, including for
// empty non-nil data.
func TestSendCommandWireBytes(t *testing.T) {
	cases := []struct {
		name string
		cmd  byte
		data []byte
		want []byte
	}{
		{"bare restart", 'q', nil, []byte{'q'}},
		{"get option", 'G', []byte{0x01}, []byte{'G', 0x01, 0x01, 0x01}},
		{"set option", 'C', []byte{0x01, 'a', 'b'}, []byte{'C', 0x03, 0x01, 'a', 'b', 0xC4}},
		{"empty data is framed", 'C', []byte{}, []byte{'C', 0x00, 0x00}},
		{"checksum wraps", 'C', []byte{0xFF, 0x02}, []byte{'C', 0x02, 0xFF, 0x02, 0x01}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, dev := pipeClient(t)
			errc := make(chan error, 1)
			go func() { errc <- c.SendCommand(tc.cmd, tc.data) }()
			if got := readN(t, dev, len(tc.want)); !bytes.Equal(got, tc.want) {
				t.Errorf("wire = % X, want % X", got, tc.want)
			}
			if err := <-errc; err != nil {
				t.Errorf("SendCommand: %v", err)
			}
		})
	}
}

// SendCommand rejects payloads longer than the one-byte length field can
// describe and writes nothing.
func TestSendCommandTooLong(t *testing.T) {
	c, dev := pipeClient(t)
	got := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(dev) // everything written until the client hangs up
		got <- b
	}()
	if err := c.SendCommand('C', make([]byte, 256)); err == nil {
		t.Fatal("SendCommand with 256 data bytes: want error, got nil")
	}
	c.Disconnect()
	if b := <-got; len(b) != 0 {
		t.Errorf("device received % X, want nothing", b)
	}
}

// SendCommand reports an error once the device end has gone away.
func TestSendCommandAfterPeerClosed(t *testing.T) {
	c, dev := pipeClient(t)
	dev.Close()
	if err := c.SendCommand('G', []byte{0x01}); err == nil {
		t.Error("framed SendCommand to closed peer: want error, got nil")
	}
	if err := c.SendCommand('q', nil); err == nil {
		t.Error("bare SendCommand to closed peer: want error, got nil")
	}
}

// ReadCommand parses one framed reply into its command byte and data.
func TestReadCommand(t *testing.T) {
	c, dev := pipeClient(t)
	go dev.Write([]byte{'G', 0x03, 0x01, 'a', 'b', 0xC4})
	cmd, err := c.ReadCommand(5 * time.Second)
	if err != nil {
		t.Fatalf("ReadCommand: %v", err)
	}
	if cmd.Command != 'G' || !bytes.Equal(cmd.Data, []byte{0x01, 'a', 'b'}) {
		t.Errorf("got command %q data % X, want 'G' data 01 61 62", cmd.Command, cmd.Data)
	}
}

// ReadCommand rejects replies whose checksum or length byte does not match
// the bytes received. A length byte promising more bytes than arrive ends in
// the read timeout.
func TestReadCommandRejectsBadFrames(t *testing.T) {
	cases := map[string][]byte{
		"bad checksum":     {'G', 0x02, 0x01, 0x02, 0x04},
		"length too long":  {'G', 0x05, 0x01, 0x02, 0x03},
		"length too short": {'G', 0x01, 0x01, 0x02, 0x03},
	}
	for name, frame := range cases {
		t.Run(name, func(t *testing.T) {
			c, dev := pipeClient(t)
			go dev.Write(frame)
			if cmd, err := c.ReadCommand(500 * time.Millisecond); err == nil {
				t.Errorf("ReadCommand(% X) = %v, want error", frame, cmd)
			}
		})
	}
}

// ReadCommand returns ErrNoData for a zero-length read.
func TestReadCommandNoData(t *testing.T) {
	c, dev := pipeClient(t)
	go dev.Write([]byte{}) // net.Pipe delivers this as a 0-byte read
	if _, err := c.ReadCommand(5 * time.Second); !errors.Is(err, ErrNoData) {
		t.Errorf("err = %v, want ErrNoData", err)
	}
}

// ReadCommand gives up with a deadline error when the device stays silent,
// and reports EOF once the device has hung up.
func TestReadCommandTimeoutAndEOF(t *testing.T) {
	c, dev := pipeClient(t)
	if _, err := c.ReadCommand(10 * time.Millisecond); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("silent device: err = %v, want deadline exceeded", err)
	}
	dev.Close()
	if _, err := c.ReadCommand(5 * time.Second); !errors.Is(err, io.EOF) {
		t.Errorf("closed device: err = %v, want io.EOF", err)
	}
}

// ReadCommand returns a whole frame even when it arrives in more than one
// TCP segment, as a stream socket is free to deliver it.
func TestReadCommandSplitFrame(t *testing.T) {
	c, dev := pipeClient(t)
	go func() {
		dev.Write([]byte{'G', 0x03})
		dev.Write([]byte{0x01, 'a', 'b', 0xC4})
	}()
	cmd, err := c.ReadCommand(5 * time.Second)
	if err != nil {
		t.Fatalf("ReadCommand: %v", err)
	}
	if cmd.Command != 'G' || !bytes.Equal(cmd.Data, []byte{0x01, 'a', 'b'}) {
		t.Errorf("got command %q data % X, want 'G' data 01 61 62", cmd.Command, cmd.Data)
	}
}

// Every I/O method returns ErrNotConnected before Connect, and Disconnect
// is a no-op.
func TestNotConnected(t *testing.T) {
	c := NewClient("127.0.0.1:1")
	if err := c.SendCommand('q', nil); !errors.Is(err, ErrNotConnected) {
		t.Errorf("SendCommand: err = %v, want ErrNotConnected", err)
	}
	if _, err := c.ReadCommand(time.Millisecond); !errors.Is(err, ErrNotConnected) {
		t.Errorf("ReadCommand: err = %v, want ErrNotConnected", err)
	}
	if err := c.Disconnect(); err != nil {
		t.Errorf("Disconnect: %v", err)
	}
}

// listen starts a loopback listener and returns it with a channel that
// yields each accepted connection.
func listen(t *testing.T) (net.Listener, <-chan net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	conns := make(chan net.Conn, 4)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { conn.Close() })
			conns <- conn
		}
	}()
	return ln, conns
}

func accepted(t *testing.T, conns <-chan net.Conn) net.Conn {
	t.Helper()
	select {
	case conn := <-conns:
		return conn
	case <-time.After(5 * time.Second):
		t.Fatal("no connection accepted")
		return nil
	}
}

// Connect dials the address the client was built with and is a no-op while
// already connected; Disconnect closes the socket and drops back to the
// not-connected state, and is safe to repeat.
func TestConnectDisconnect(t *testing.T) {
	ln, conns := listen(t)
	c := NewClient(ln.Addr().String())
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	dev := accepted(t, conns)
	first := c.conn
	if err := c.Connect(); err != nil {
		t.Fatalf("second Connect: %v", err)
	}
	if c.conn != first {
		t.Error("second Connect replaced the open connection")
	}

	if err := c.SendCommand('q', nil); err != nil {
		t.Fatalf("SendCommand: %v", err)
	}
	if got := readN(t, dev, 1); got[0] != 'q' {
		t.Errorf("device got %q, want 'q'", got[0])
	}

	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	dev.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := dev.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("device read after Disconnect: err = %v, want io.EOF", err)
	}
	if err := c.SendCommand('q', nil); !errors.Is(err, ErrNotConnected) {
		t.Errorf("SendCommand after Disconnect: err = %v, want ErrNotConnected", err)
	}
	if err := c.Disconnect(); err != nil {
		t.Errorf("second Disconnect: %v", err)
	}
}

// SetAddress changes where the next Connect dials.
func TestSetAddress(t *testing.T) {
	ln, conns := listen(t)
	c := NewClient("")
	c.SetAddress(ln.Addr().String())
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Disconnect()
	accepted(t, conns)
}

// A failed dial returns the error and leaves the client disconnected.
func TestConnectRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens here any more

	c := NewClient(addr)
	if err := c.Connect(); err == nil {
		c.Disconnect()
		t.Fatal("Connect to closed port: want error, got nil")
	}
	if err := c.SendCommand('q', nil); !errors.Is(err, ErrNotConnected) {
		t.Errorf("SendCommand after failed Connect: err = %v, want ErrNotConnected", err)
	}
}
