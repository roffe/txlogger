// Package protocol is the wire protocol between txlogger and the 32-bit
// j2534proxy helper process: framed messages [cmd u8][len u16 LE][payload]
// over a Windows named pipe. The proxy exists because most J2534
// PassThru DLLs are 32-bit and cannot be loaded by the 64-bit txlogger.
package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// client -> proxy
	CmdHello     = 'H' // request the adapter list; the connection becomes the keepalive channel
	CmdOpen      = 'O' // open an adapter: JSON OpenRequest; the connection becomes the frame channel
	CmdFrame     = 't' // transmit frame: id u32 LE, responses u8, dlc u8, data
	CmdSetFilter = 'F' // replace filters: n * u32 LE
	CmdPing      = 'K'

	// proxy -> client
	CmdList    = 'L' // JSON []AdapterInfo
	CmdFrameIn = 'T' // received frame: id u32 LE, flags u8 (bit0 extended), dlc u8, data
	CmdEvent   = 'e' // event: level u8, message
	CmdOpenErr = 'E' // open failed: error string
	CmdOpenOK  = 'o'
	CmdPong    = 'k'
)

// AdapterInfo describes one proxy-served adapter.
type AdapterInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	HSCAN       bool   `json:"hscan"`
	SWCAN       bool   `json:"swcan"`
	KLine       bool   `json:"kline"`
}

// OpenRequest carries the adapter config for CmdOpen.
type OpenRequest struct {
	Name          string   `json:"name"`
	Port          string   `json:"port"`
	PortBaudrate  int      `json:"port_baudrate"`
	CANRate       float64  `json:"can_rate"`
	CANFilter     []uint32 `json:"can_filter"`
	UseExtendedID bool     `json:"use_extended_id"`
	Debug         bool     `json:"debug"`
}

// Write sends one framed message.
func Write(w io.Writer, cmd byte, payload []byte) error {
	if len(payload) > 0xFFFF {
		return fmt.Errorf("payload too large: %d", len(payload))
	}
	buf := make([]byte, 3+len(payload))
	buf[0] = cmd
	binary.LittleEndian.PutUint16(buf[1:3], uint16(len(payload)))
	copy(buf[3:], payload)
	_, err := w.Write(buf)
	return err
}

// Read reads one framed message.
func Read(r io.Reader) (cmd byte, payload []byte, err error) {
	var hdr [3]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.LittleEndian.Uint16(hdr[1:3])
	payload = make([]byte, n)
	if _, err = io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return hdr[0], payload, nil
}
