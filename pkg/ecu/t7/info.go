package t7

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/roffe/txlogger/pkg/kwp2000"
	"github.com/roffe/txlogger/pkg/model"
)

var T7Headers = []model.Header{
	{Desc: "Engine type", ID: 0x97},
	{Desc: "Software version:", ID: 0x95},
	{Desc: "Software P/N", ID: 0x94},
	{Desc: "Immobiilizer code", ID: 0x92},
	{Desc: "Chassis ID/VIN", ID: 0x90},
	{Desc: "Box Hardware P/N", ID: 0x91},
	{Desc: "Tester info", ID: 0x98},
	{Desc: "Software date", ID: 0x99},
}

// Print out some Trionic7 info
func (t *Client) Info(ctx context.Context) ([]model.HeaderResult, error) {
	if err := t.DataInitialization(ctx); err != nil {
		return nil, err
	}

	defer t.StopSession(ctx)

	var out []model.HeaderResult
	for _, d := range T7Headers {
		h, err := t.GetHeader(ctx, byte(d.ID))
		if err != nil {
			return nil, fmt.Errorf("ECU info failed: %v", err)
		}

		var res model.HeaderResult
		if d.ID == 0xF6 {
			res.Value = fmt.Sprintf("%02X %02X %02X %02X", h[0], h[1], h[2], h[3])
		} else {
			res.Value = strings.Trim(h, "\x00")
		}
		res.Desc = d.Desc
		res.ID = d.ID
		out = append(out, res)
	}

	out = append(out, t.l3SeedXOR(ctx)...)
	return out, nil
}

const xorOffset = 0x4B

// l3SeedXOR requests the level-3 SecurityAccess seed and presents the algorithm
// XOR derived from it. The seed's byte order on the wire is unconfirmed, so both
// the big-endian and byte-swapped interpretations are shown; on a locked
// ECU one of them is the working XOR (with SUB 2356).
func (t *Client) l3SeedXOR(ctx context.Context) []model.HeaderResult {
	hi, lo, err := t.requestL3Seed(ctx)
	if err != nil {
		return []model.HeaderResult{{Header: model.Header{Desc: "L3 seed"}, Value: "n/a (" + err.Error() + ")"}}
	}
	seedBE := uint16(hi)<<8 | uint16(lo)
	seedSwap := uint16(lo)<<8 | uint16(hi)
	return []model.HeaderResult{
		{Header: model.Header{Desc: "L3 seed (hi lo)"}, Value: fmt.Sprintf("%02X %02X", hi, lo)},
		{Header: model.Header{Desc: "XOR (BE seed)"}, Value: fmt.Sprintf("XOR %04X SUB 2356", seedBE+xorOffset)},
		{Header: model.Header{Desc: "XOR (byteswapped seed)"}, Value: fmt.Sprintf("XOR %04X SUB 2356", seedSwap+xorOffset)},
	}
}

// requestL3Seed asks the ECU for its level-3 SecurityAccess seed (KWP2000
// service 0x27, sub-function 0x03) and returns the two seed bytes, without
// completing the handshake — no key is sent, so the ECU stays locked.
func (t *Client) requestL3Seed(ctx context.Context) (hi, lo byte, err error) {
	seed, err := t.kwp.RequestSeed(ctx, kwp2000.HIGH_PRIORITY)
	if err != nil {
		return 0, 0, fmt.Errorf("request L3 seed: %w", err)
	}
	return byte(seed >> 8), byte(seed), nil
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	res, err := t.Info(ctx)
	if err != nil {
		return err
	}
	log.Println("----- ECU info ---------------")
	for _, r := range res {
		log.Println(r.Desc, r.Value)
	}
	log.Println("------------------------------")
	return nil
}
