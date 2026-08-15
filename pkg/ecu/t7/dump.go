package t7

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
)

// dumpChunkSize is the payload of one readMemoryByAddress round-trip. The KWP
// length is a single byte, so 0xF5 is close to the ceiling.
const dumpChunkSize = 0xF5

func (t *Client) DumpECU(ctx context.Context) ([]byte, error) {
	ok, err := t.KnockKnock(ctx)
	if err != nil || !ok {
		return nil, fmt.Errorf("failed to authenticate: %v", err)
	}
	defer t.StopSession(ctx)
	return t.readECU(ctx, 0, 0x80000)
}

func (t *Client) readECU(ctx context.Context, addr, length int) ([]byte, error) {
	t.cfg.OnProgress(-float64(length))
	t.cfg.OnMessage("Dumping ECU")

	start := time.Now()
	out := bytes.NewBuffer(make([]byte, 0, length))

	for readPos := addr; readPos < addr+length; {
		t.cfg.OnProgress(float64(out.Len()))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		readLength := min(dumpChunkSize, addr+length-readPos)
		err := retry.Do(func() error {
			b, err := t.kwp.ReadMemoryByAddressF0(ctx, readPos, readLength)
			if err != nil {
				return err
			}
			out.Write(b)
			return nil
		},
			retry.Context(ctx),
			retry.Attempts(3),
			retry.OnRetry(func(n uint, err error) {
				t.cfg.OnMessage(fmt.Sprintf("Failed to read memory by address, pos: 0x%X, length: 0x%X, retrying: %v", readPos, readLength, err))
			}),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to read memory by address, pos: 0x%X, length: 0x%X: %w", readPos, readLength, err)
		}
		readPos += readLength
	}

	t.cfg.OnProgress(float64(out.Len()))
	t.cfg.OnMessage(fmt.Sprintf("Done, took: %s", time.Since(start).Round(time.Second).String()))

	return out.Bytes(), nil
}
