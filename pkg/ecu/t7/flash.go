package t7

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/avast/retry-go/v4"
)

func (t *Client) LoadBinFile(filename string) (int64, []byte, error) {
	var temp byte
	readBytes := 0
	data, err := os.ReadFile(filename)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to read bin file: %v", err)
	}
	readBytes = len(data)

	if readBytes == 256*1024 {
		return 0, nil, errors.New("error: is this a Trionic 5 ECU binary?")
	}
	if readBytes == 512*1024 || readBytes == 0x70100 {
		// Convert Motorola byte-order to Intel byte-order (just in RAM)
		if data[0] == 0xFF && data[1] == 0xFF && data[2] == 0xFC && data[3] == 0xEF {
			log.Println("note: Motorola byte-order detected.")
			for i := 0; i < readBytes; i += 2 {
				temp = data[i]
				data[i] = data[i+1]
				data[i+1] = temp
			}
		}
	}

	if readBytes == 512*1024 || readBytes == 0x70100 {
		return int64(readBytes), data, nil
	}

	return int64(readBytes), nil, errors.New("invalid bin size")
}

// Writable flash regions. The gap 0x7B000-0x7FE00 is never written: the ECU's
// hardware-specific flash algorithms live at 0x7C000 and must survive reflash.
// The PI area sits in 0x7FE00-0x80000 (matches TrionicCANLib's 512-byte window).
var t7WriteRegions = []struct{ start, end int }{
	{0x000000, 0x07B000},
	{0x07FE00, 0x080000},
}

const (
	flashChunkSize = 128   // bytes per transferData (matches TrionicCANLib)
	ffSkipMinRun   = 0x200 // skip contiguous 0xFF runs at least this long: erased flash is already 0xFF, so writing it is a no-op
)

type writeSegment struct{ start, end int }

// computeWriteSegments splits the writable regions into runs of real data,
// skipping large 0xFF stretches. Small 0xFF gaps (< ffSkipMinRun) stay in a
// segment rather than paying a requestDownload round-trip to skip them.
func computeWriteSegments(bin []byte) []writeSegment {
	var segs []writeSegment
	for _, r := range t7WriteRegions {
		i := r.start
		for i < r.end {
			for i < r.end && bin[i] == 0xFF { // skip to next data byte
				i++
			}
			if i >= r.end {
				break
			}
			segStart := i
			lastData := i
			for i < r.end {
				if bin[i] != 0xFF {
					lastData = i
					i++
					continue
				}
				j := i // measure the 0xFF run
				for j < r.end && bin[j] == 0xFF {
					j++
				}
				if j-i >= ffSkipMinRun {
					break // long run ends this segment
				}
				i = j // short gap: absorb into the segment
			}
			segs = append(segs, writeSegment{segStart &^ 1, lastData + 1}) // even-align start, trim trailing 0xFF
		}
	}
	return segs
}

// NormalizeBin validates a 512 KB Trionic 7 image and returns it in Intel byte
// order, without mutating the caller's slice.
func NormalizeBin(bin []byte) ([]byte, error) {
	if len(bin) != 0x80000 {
		return nil, fmt.Errorf("error: expected a 512KB (0x80000) Trionic 7 binary, got %d bytes", len(bin))
	}
	// Auto-correct Motorola byte-order dumps (FF FF FC EF) to Intel order.
	if bin[0] == 0xFF && bin[1] == 0xFF && bin[2] == 0xFC && bin[3] == 0xEF {
		log.Println("note: Motorola byte-order detected, swapping to Intel")
		swapped := make([]byte, len(bin))
		for i := 0; i < len(bin); i += 2 {
			swapped[i], swapped[i+1] = bin[i+1], bin[i]
		}
		bin = swapped
	}
	if bin[0] != 0xFF || bin[1] != 0xFF || bin[2] != 0xEF || bin[3] != 0xFC {
		return nil, fmt.Errorf("error: bin doesn't appear to be for a Trionic 7 ECU! (%02X%02X%02X%02X)",
			bin[0], bin[1], bin[2], bin[3])
	}
	return bin, nil
}

// Flash the ECU
func (t *Client) FlashECU(ctx context.Context, bin []byte) error {
	bin, err := NormalizeBin(bin)
	if err != nil {
		return err
	}

	if err := t.DataInitialization(ctx); err != nil {
		return err
	}

	ok, err := t.KnockKnock(ctx)
	if err != nil || !ok {
		return fmt.Errorf("failed to authenticate: %v", err)
	}

	if err := t.EraseECU(ctx); err != nil {
		return err
	}

	return t.writeBin(ctx, bin)
}

// writeBin runs the download phase: requestDownload/transferData over every data
// segment, then requestTransferExit. The chip is already erased, so segments may
// be written in any order and a failed one resumes from its failure point.
func (t *Client) writeBin(ctx context.Context, bin []byte) error {
	t.cfg.OnProgress(-float64(0x80000))
	t.cfg.OnMessage("Flashing ECU")

	start := time.Now()
	for _, seg := range computeWriteSegments(bin) {
		binPos := seg.start // persists across attempts; only advances on successful writes
		err := retry.Do(func() error {
			// re-anchor the ECU write pointer at the last good position so a retry
			// resumes where it failed instead of re-flashing the whole segment
			if err := t.kwp.RequestDownload(ctx, uint32(binPos), uint32(seg.end-binPos)); err != nil {
				return err
			}
			for binPos < seg.end {
				writeBytes := min(seg.end-binPos, flashChunkSize)
				if err := t.kwp.TransferDataBlock(ctx, bin[binPos:binPos+writeBytes]); err != nil {
					return fmt.Errorf("writing 0x%X-0x%X: %w", binPos, binPos+writeBytes, err)
				}
				binPos += writeBytes
				t.cfg.OnProgress(float64(binPos))
			}
			return nil
		},
			retry.Context(ctx),
			retry.Attempts(30),
			retry.OnRetry(func(n uint, err error) {
				t.cfg.OnMessage(fmt.Sprintf("retrying block write: %v", err))
			}),
			retry.Delay(150*time.Millisecond),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return err
		}
	}

	if err := t.kwp.RequestTransferExit(ctx); err != nil {
		return fmt.Errorf("exit download mode failed: %w", err)
	}

	t.cfg.OnMessage(fmt.Sprintf("Done, took: %s", time.Since(start).Round(time.Second)))
	return nil
}
