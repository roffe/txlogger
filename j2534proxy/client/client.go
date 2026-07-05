// Package client launches and talks to the j2534proxy helper: a 32-bit
// process that loads the 32-bit J2534 PassThru DLLs the 64-bit txlogger
// cannot. Start spawns the proxy, registers every DLL it reports as a
// normal gocan v2 adapter, and keeps the proxy alive with pings; the proxy
// exits by itself when txlogger dies (stdin EOF / ping timeout).
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/j2534proxy/protocol"
)

const pingInterval = 3 * time.Second

// Proxy is a running j2534proxy helper process.
type Proxy struct {
	addr  string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	conn  net.Conn
}

// Start launches the proxy executable (next to the txlogger binary),
// registers the adapters it serves — returned so GUIs can extend their
// adapter lists — and starts the keepalive. The proxy follows ctx:
// cancelling it drops the lifeline and the proxy exits.
func Start(ctx context.Context, logf func(string)) (*Proxy, []gocan.AdapterInfo, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	exe := filepath.Join(filepath.Dir(self), "j2534proxy.exe")
	if _, err := os.Stat(exe); err != nil {
		return nil, nil, fmt.Errorf("j2534proxy not found: %w", err)
	}

	cmd := exec.Command(exe)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	// First stdout line tells us where it listens.
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		stdin.Close()
		return nil, nil, fmt.Errorf("proxy did not report a pipe: %w", err)
	}
	var pipeName string
	if _, err := fmt.Sscanf(strings.TrimSpace(line), "PIPE %s", &pipeName); err != nil {
		stdin.Close()
		return nil, nil, fmt.Errorf("unexpected proxy greeting %q: %w", line, err)
	}

	p := &Proxy{
		addr:  pipeName,
		cmd:   cmd,
		stdin: stdin,
	}

	p.conn, err = dialProxy(p.addr)
	if err != nil {
		stdin.Close()
		return nil, nil, err
	}
	if err := protocol.Write(p.conn, protocol.CmdHello, nil); err != nil {
		p.close()
		return nil, nil, err
	}
	cmdB, payload, err := protocol.Read(p.conn)
	if err != nil || cmdB != protocol.CmdList {
		p.close()
		return nil, nil, fmt.Errorf("proxy adapter list failed: %v", err)
	}
	var list []protocol.AdapterInfo
	if err := json.Unmarshal(payload, &list); err != nil {
		p.close()
		return nil, nil, err
	}

	registered := make([]gocan.AdapterInfo, 0, len(list))
	for _, info := range list {
		name := info.Name
		ai := gocan.AdapterInfo{
			Name:        name,
			Description: info.Description + " (32-bit, via j2534proxy)",
			Capabilities: gocan.Capabilities{
				HSCAN: info.HSCAN,
				SWCAN: info.SWCAN,
				KLine: info.KLine,
			},
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				return &Adapter{addr: p.addr, name: name, cfg: cfg}, nil
			},
		}
		gocan.Register(ai)
		registered = append(registered, ai)
		if logf != nil {
			logf("j2534proxy: " + name)
		}
	}

	go p.keepalive(ctx)
	return p, registered, nil
}

// keepalive feeds both proxy lifelines (stdin bytes + protocol pings) until
// ctx is cancelled, then drops them so the proxy exits.
func (p *Proxy) keepalive(ctx context.Context) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	defer p.close()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := p.stdin.Write([]byte{'K'}); err != nil {
				return
			}
			if err := protocol.Write(p.conn, protocol.CmdPing, nil); err != nil {
				return
			}
			protocol.Read(p.conn) // pong; errors surface on the next ping
		}
	}
}

func (p *Proxy) close() {
	p.stdin.Close()
	p.conn.Close()
}

// Adapter runs one proxy-served J2534 DLL as a gocan v2 adapter.
type Adapter struct {
	addr string
	name string
	cfg  gocan.Config
	bus  *gocan.Bus

	mu   sync.Mutex
	conn net.Conn
}

// Name reports the proxy-side adapter name (used by Bus.AdapterName).
func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Open(ctx context.Context, bus *gocan.Bus) error {
	a.bus = bus
	conn, err := dialProxy(a.addr)
	if err != nil {
		return fmt.Errorf("j2534proxy not running: %w", err)
	}
	req, err := json.Marshal(protocol.OpenRequest{
		Name:          a.name,
		Port:          a.cfg.Port,
		PortBaudrate:  a.cfg.PortBaudrate,
		CANRate:       a.cfg.CANRate,
		CANFilter:     a.cfg.CANFilter,
		UseExtendedID: a.cfg.UseExtendedID,
		Debug:         a.cfg.Debug,
	})
	if err != nil {
		conn.Close()
		return err
	}
	if err := protocol.Write(conn, protocol.CmdOpen, req); err != nil {
		conn.Close()
		return err
	}
	cmd, payload, err := protocol.Read(conn)
	if err != nil {
		conn.Close()
		return err
	}
	switch cmd {
	case protocol.CmdOpenOK:
	case protocol.CmdOpenErr:
		conn.Close()
		return errors.New(string(payload))
	default:
		conn.Close()
		return fmt.Errorf("unexpected open reply %q", cmd)
	}
	a.conn = conn

	go a.readLoop(ctx)
	return nil
}

func (a *Adapter) Close() error {
	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

func (a *Adapter) Send(ctx context.Context, f gocan.Frame) error {
	responses := gocan.ExpectedResponses(ctx)
	buf := make([]byte, 6+f.Length)
	buf[0] = byte(f.ID)
	buf[1] = byte(f.ID >> 8)
	buf[2] = byte(f.ID >> 16)
	buf[3] = byte(f.ID >> 24)
	buf[4] = byte(responses)
	buf[5] = f.Length
	copy(buf[6:], f.Bytes())

	a.mu.Lock()
	defer a.mu.Unlock()
	return protocol.Write(a.conn, protocol.CmdFrame, buf)
}

// SetFilter replaces the PASS filters on the proxied device.
func (a *Adapter) SetFilter(ids []uint32) error {
	payload := make([]byte, 4*len(ids))
	for i, id := range ids {
		payload[4*i] = byte(id)
		payload[4*i+1] = byte(id >> 8)
		payload[4*i+2] = byte(id >> 16)
		payload[4*i+3] = byte(id >> 24)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return protocol.Write(a.conn, protocol.CmdSetFilter, payload)
}

func (a *Adapter) readLoop(ctx context.Context) {
	for {
		cmd, payload, err := protocol.Read(a.conn)
		if err != nil {
			if ctx.Err() == nil {
				a.bus.Fatal(fmt.Errorf("j2534proxy connection lost: %w", err))
			}
			return
		}
		switch cmd {
		case protocol.CmdFrameIn:
			if len(payload) < 6 {
				continue
			}
			dlc := int(payload[5])
			if dlc > 8 || len(payload) < 6+dlc {
				continue
			}
			f := gocan.Frame{
				ID:       uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24,
				Extended: payload[4]&1 == 1,
				Length:   uint8(dlc),
			}
			copy(f.Data[:], payload[6:6+dlc])
			a.bus.Deliver(f)
		case protocol.CmdEvent:
			if len(payload) < 1 {
				continue
			}
			a.bus.Emit(gocan.Event{Type: gocan.EventType(payload[0]), Details: string(payload[1:])})
		case protocol.CmdPong:
		}
	}
}
