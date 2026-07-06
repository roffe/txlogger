//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Microsoft/go-winio"
	gocan "github.com/roffe/gocan/v2"
	_ "github.com/roffe/gocan/v2/adapters/j2534" // registers with -tags j2534
	"github.com/roffe/txlogger/j2534proxy/protocol"
)

// lifelineTimeout is how long the proxy survives without a sign of life
// (stdin byte or protocol ping) from txlogger.
const lifelineTimeout = 15 * time.Second

var lastAlive atomic.Int64

func alive() { lastAlive.Store(time.Now().UnixNano()) }

func run() error {
	alive()
	go stdinLifeline()
	go watchdog()

	pipeName := fmt.Sprintf(`\\.\pipe\txlogger-j2534proxy-%d`, os.Getpid())
	ln, err := winio.ListenPipe(pipeName, nil)
	if err != nil {
		return err
	}
	// txlogger reads this line from our stdout to find the pipe.
	fmt.Printf("PIPE %s\n", pipeName)

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handleConn(conn)
	}
}

// stdinLifeline treats bytes on stdin as keepalives and EOF as the parent
// having died.
func stdinLifeline() {
	buf := make([]byte, 1)
	for {
		if _, err := os.Stdin.Read(buf); err != nil {
			os.Exit(0)
		}
		alive()
	}
}

func watchdog() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		if time.Since(time.Unix(0, lastAlive.Load())) > lifelineTimeout {
			os.Exit(0)
		}
	}
}

// connWriter serializes protocol writes from the frame, event and pong paths.
type connWriter struct {
	mu   sync.Mutex
	conn net.Conn
}

func (w *connWriter) send(cmd byte, payload []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return protocol.Write(w.conn, cmd, payload)
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	w := &connWriter{conn: conn}

	cmd, payload, err := protocol.Read(conn)
	if err != nil {
		return
	}
	alive()

	switch cmd {
	case protocol.CmdHello:
		if err := sendAdapterList(w); err != nil {
			return
		}
		// The hello connection doubles as the keepalive channel.
		for {
			cmd, _, err := protocol.Read(conn)
			if err != nil {
				return
			}
			alive()
			if cmd == protocol.CmdPing {
				w.send(protocol.CmdPong, nil)
			}
		}
	case protocol.CmdOpen:
		serveAdapter(w, conn, payload)
	}
}

func sendAdapterList(w *connWriter) error {
	var list []protocol.AdapterInfo
	for _, info := range gocan.Adapters() {
		list = append(list, protocol.AdapterInfo{
			Name:        info.Name,
			Description: info.Description,
			HSCAN:       info.Capabilities.HSCAN,
			SWCAN:       info.Capabilities.SWCAN,
			KLine:       info.Capabilities.KLine,
		})
	}
	payload, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return w.send(protocol.CmdList, payload)
}

// serveAdapter opens the requested adapter on an in-proxy bus and relays
// frames and events for the lifetime of the connection.
func serveAdapter(w *connWriter, conn net.Conn, payload []byte) {
	var req protocol.OpenRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		w.send(protocol.CmdOpenErr, []byte(err.Error()))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus, err := gocan.Open(ctx, req.Name, gocan.Config{
		Port:          req.Port,
		PortBaudrate:  req.PortBaudrate,
		CANRate:       req.CANRate,
		CANFilter:     req.CANFilter,
		UseExtendedID: req.UseExtendedID,
		Debug:         req.Debug,
	}, gocan.WithEventFunc(func(e gocan.Event) {
		w.send(protocol.CmdEvent, append([]byte{byte(e.Type)}, e.String()...))
	}))
	if err != nil {
		w.send(protocol.CmdOpenErr, []byte(err.Error()))
		return
	}
	defer bus.Close()

	if err := w.send(protocol.CmdOpenOK, nil); err != nil {
		return
	}

	// all bus traffic -> client
	go func() {
		for f := range bus.Frames(ctx) {
			buf := make([]byte, 6+f.Length)
			buf[0] = byte(f.ID)
			buf[1] = byte(f.ID >> 8)
			buf[2] = byte(f.ID >> 16)
			buf[3] = byte(f.ID >> 24)
			if f.Extended {
				buf[4] = 1
			}
			buf[5] = f.Length
			copy(buf[6:], f.Bytes())
			if w.send(protocol.CmdFrameIn, buf) != nil {
				cancel()
				return
			}
		}
	}()

	// client -> bus
	for {
		cmd, payload, err := protocol.Read(conn)
		if err != nil {
			return
		}
		alive()
		switch cmd {
		case protocol.CmdPing:
			w.send(protocol.CmdPong, nil)
		case protocol.CmdFrame:
			if len(payload) < 6 {
				continue
			}
			id := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			responses := int(payload[4])
			dlc := int(payload[5])
			if dlc > 8 || len(payload) < 6+dlc {
				continue
			}
			f := gocan.Frame{ID: id, Length: uint8(dlc)}
			copy(f.Data[:], payload[6:6+dlc])
			sctx := ctx
			if responses > 0 {
				sctx = gocan.WithExpectedResponses(ctx, responses)
			}
			if err := bus.Send(sctx, f); err != nil {
				w.send(protocol.CmdEvent, append([]byte{byte(gocan.EventTypeError)}, "send: "+err.Error()...))
			}
		case protocol.CmdSetFilter:
			var ids []uint32
			for i := 0; i+4 <= len(payload); i += 4 {
				ids = append(ids, uint32(payload[i])|uint32(payload[i+1])<<8|uint32(payload[i+2])<<16|uint32(payload[i+3])<<24)
			}
			if sf, ok := bus.Adapter().(interface{ SetFilter([]uint32) error }); ok {
				if err := sf.SetFilter(ids); err != nil {
					w.send(protocol.CmdEvent, append([]byte{byte(gocan.EventTypeError)}, "setfilter: "+err.Error()...))
				}
			}
		}
	}
}
