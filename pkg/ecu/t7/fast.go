package t7

import (
	"bytes"
	"context"
	"fmt"

	"github.com/avast/retry-go/v4"
)

// fastDumpChunk is one FastUpload request: ~1 s of stream, so a failed chunk is
// cheap to repeat.
const fastDumpChunk = 0x8000

// probeFast asks the ECU for the fast transfer routines (firmware extension,
// see t7kwp.FastInfo) and returns their block size, or 0 for stock firmware.
// Probe before EOLProgrammingStart: the EOL code that runs from RAM is the
// firmware currently in flash.
func (t *Client) probeFast(ctx context.Context) int {
	block, ok, err := t.kwp.FastInfo(ctx)
	if err != nil {
		t.cfg.OnMessage(fmt.Sprintf("Fast transfer probe failed, using standard transfer: %v", err))
		return 0
	}
	if !ok {
		return 0
	}
	t.cfg.OnMessage(fmt.Sprintf("ECU supports fast transfer (%d byte blocks)", block))
	return block
}

// fastRead appends [addr+out.Len(), addr+length) to out with FastUpload. A
// chunk that keeps failing (typically an adapter that can't receive at full bus
// load) ends it with an error; out keeps what was read, so the caller can go on
// with the KWP path from there.
func (t *Client) fastRead(ctx context.Context, out *bytes.Buffer, addr, length int) error {
	for pos := addr + out.Len(); pos < addr+length; pos = addr + out.Len() {
		n := min(fastDumpChunk, addr+length-pos)
		err := retry.Do(func() error {
			b, err := t.kwp.FastUpload(ctx, uint32(pos), uint32(n))
			if err == nil {
				out.Write(b)
			}
			return err
		},
			retry.Context(ctx),
			retry.Attempts(3),
			retry.OnRetry(func(n uint, err error) {
				t.cfg.OnMessage(fmt.Sprintf("Fast read at 0x%X failed, retrying: %v", pos, err))
			}),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return err
		}
		t.cfg.OnProgress(float64(out.Len()))
	}
	return nil
}

// fastWrite programs segs with FastDownloadBlock. It returns the segments still
// to write: none, or, when a block keeps failing, the rest from that block on
// for the KWP path to finish. Resending a block is safe, see FastDownloadBlock.
func (t *Client) fastWrite(ctx context.Context, bin []byte, segs []writeSegment, block int) ([]writeSegment, error) {
	for i, seg := range segs {
		end := (seg.end + 1) &^ 1 // fast blocks are even; regions end even, so this stays in bin
		for pos := seg.start; pos < end; {
			next := min(pos+block, end)
			err := retry.Do(func() error {
				return t.kwp.FastDownloadBlock(ctx, uint32(pos), bin[pos:next])
			},
				retry.Context(ctx),
				retry.Attempts(5),
				retry.OnRetry(func(n uint, err error) {
					t.cfg.OnMessage(fmt.Sprintf("Fast write at 0x%X failed, retrying: %v", pos, err))
				}),
				retry.LastErrorOnly(true),
			)
			if err != nil {
				return append([]writeSegment{{pos, seg.end}}, segs[i+1:]...), err
			}
			pos = next
			t.cfg.OnProgress(float64(pos))
		}
	}
	return nil, nil
}
