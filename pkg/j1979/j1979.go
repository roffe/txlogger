// Package j1979 implements an SAE J1979 (OBD-II) diagnostics client for the
// SAAB Trionic 7 ECU over gocan v2.
//
// The T7 serves J1979 modes $01-$09 on its KWP2000 CAN transport: any service
// ID <= 0x0F received on 0x240 is routed to the firmware's SaeJ1979Comm
// handler (Diagnos/Source/Scantool.cpp, dispatched in Vios.77/Source/YDiaga.c
// KW2000ComHandler). Responses use the regular T7 chunked framing on the KWP
// response channel, so this client shares session semantics with pkg/kwp2000:
// either call StartSession here, or reuse an already started KWP2000 session
// on the same bus (the default response ID 0x258 is correct for T7).
package j1979

import (
	"context"
	"fmt"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

// T7 CAN identifiers.
var (
	INIT_MSG_ID        uint32 = 0x220 // startCommunication request
	INIT_RESP_ID       uint32 = 0x238 // startCommunication response
	REQ_MSG_ID         uint32 = 0x240 // request channel
	RESP_MSG_ID        uint32 = 0x258 // response channel (default, ECU may reassign)
	RESP_CHUNK_CONF_ID uint32 = 0x266 // tester ack of a response chunk
)

// J1979 modes.
const (
	MODE_CURRENT_DATA     = 0x01
	MODE_FREEZE_FRAME     = 0x02
	MODE_STORED_DTCS      = 0x03
	MODE_CLEAR_DTCS       = 0x04
	MODE_O2_TEST_RESULTS  = 0x05
	MODE_TEST_RESULTS     = 0x06
	MODE_PENDING_DTCS     = 0x07
	MODE_CONTROL          = 0x08
	MODE_VEHICLE_INFO     = 0x09
	positiveResponseFlag  = 0x40
	negativeResponse      = 0x7F
	startCommunicationReq = 0x81
	stopCommunicationReq  = 0x82
	testerPresentReq      = 0x3E
)

var DefaultTimeout = 250 * time.Millisecond

type Client struct {
	c          *gocan.Bus
	responseID uint32
}

func New(bus *gocan.Bus) *Client {
	return &Client{c: bus, responseID: RESP_MSG_ID}
}

// SetResponseID overrides the KWP response channel, e.g. when a session was
// started elsewhere and the ECU assigned a non-default ID.
func (c *Client) SetResponseID(id uint32) { c.responseID = id }

func (c *Client) request(ctx context.Context, id uint32, payload []byte, replyID uint32) (gocan.Frame, error) {
	rctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	return c.c.Request(rctx, gocan.NewFrame(id, payload), replyID)
}

// StartSession opens the T7 diagnostic session (startCommunication on 0x220)
// and learns the ECU-assigned response ID. Not needed if a pkg/kwp2000
// session is already active on the same bus.
func (c *Client) StartSession(ctx context.Context) error {
	resp, err := c.request(ctx, INIT_MSG_ID, []byte{0x3F, startCommunicationReq, 0x00, 0x11, byte(REQ_MSG_ID >> 8), byte(REQ_MSG_ID)}, INIT_RESP_ID)
	if err != nil {
		return fmt.Errorf("StartSession: %w", err)
	}
	if resp.Data[3] != startCommunicationReq|positiveResponseFlag {
		return fmt.Errorf("StartSession: %w", kwp2000.TranslateErrorCode(resp.Data[5]))
	}
	c.responseID = uint32(resp.Data[6])<<8 | uint32(resp.Data[7])
	return nil
}

// StopSession ends the diagnostic session.
func (c *Client) StopSession(ctx context.Context) error {
	return c.c.Send(gocan.WithExpectedResponses(ctx, 1), gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x01, stopCommunicationReq}))
}

// TesterPresent keeps the session alive; the T7 drops it after ~6 s of
// silence. Any J1979 request also refreshes the timeout, so this is only
// needed during idle gaps.
func (c *Client) TesterPresent(ctx context.Context) error {
	resp, err := c.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x01, testerPresentReq}, c.responseID)
	if err != nil {
		return fmt.Errorf("TesterPresent: %w", err)
	}
	if resp.Data[3] == negativeResponse {
		return fmt.Errorf("TesterPresent: %w", kwp2000.TranslateErrorCode(resp.Data[5]))
	}
	return nil
}

// requestJ1979 sends one J1979 request (mode + up to 2 parameter bytes, always
// a single frame) and reassembles the complete KWP message from the chunked
// response. The returned slice starts at the response SID (mode|0x40).
func (c *Client) requestJ1979(ctx context.Context, msg ...byte) ([]byte, error) {
	payload := append([]byte{0x40, 0xA1, byte(len(msg))}, msg...)
	resp, err := c.request(ctx, REQ_MSG_ID, payload, c.responseID)
	if err != nil {
		return nil, fmt.Errorf("mode $%02X: %w", msg[0], err)
	}
	if resp.Data[3] == negativeResponse {
		return nil, fmt.Errorf("mode $%02X: %w", msg[0], kwp2000.TranslateErrorCode(resp.Data[5]))
	}

	// First frame: [seq, 0xA1, kwpLen, SID, data...] — up to 5 message bytes.
	msgLen := int(resp.Data[2])
	out := make([]byte, 0, msgLen)
	n := min(msgLen, 5)
	out = append(out, resp.Data[3:3+n]...)
	remaining := msgLen - n

	// Continuation frames arrive after acking the previous one on 0x266;
	// low 6 bits of byte 0 count the frames still to come.
	for remaining > 0 {
		if resp.Data[0]&0x3F == 0 {
			return nil, fmt.Errorf("mode $%02X: truncated response, %d bytes missing", msg[0], remaining)
		}
		resp, err = c.request(ctx, RESP_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, resp.Data[0] &^ 0x40}, c.responseID)
		if err != nil {
			return nil, fmt.Errorf("mode $%02X chunk: %w", msg[0], err)
		}
		n := min(remaining, 6)
		out = append(out, resp.Data[2:2+n]...)
		remaining -= n
	}
	return out, nil
}

// mode sends a request and returns the response payload after the echoed
// response SID. An unsupported PID/TID yields an empty payload on T7, which
// the mode-specific callers turn into a typed error.
func (c *Client) mode(ctx context.Context, req ...byte) ([]byte, error) {
	msg, err := c.requestJ1979(ctx, req...)
	if err != nil {
		return nil, err
	}
	if len(msg) == 0 || msg[0] != req[0]|positiveResponseFlag {
		return nil, fmt.Errorf("mode $%02X: unexpected response % X", req[0], msg)
	}
	return msg[1:], nil
}
