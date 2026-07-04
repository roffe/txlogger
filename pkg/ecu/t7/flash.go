package t7

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan"
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

// Flash the ECU
func (t *Client) FlashECU(ctx context.Context, bin []byte) error {
	if len(bin) != 0x80000 {
		return fmt.Errorf("error: expected a 512KB (0x80000) Trionic 7 binary, got %d bytes", len(bin))
	}
	// Auto-correct Motorola byte-order dumps (FF FF FC EF) to Intel order on a copy, without mutating the caller's slice.
	if bin[0] == 0xFF && bin[1] == 0xFF && bin[2] == 0xFC && bin[3] == 0xEF {
		log.Println("note: Motorola byte-order detected, swapping to Intel")
		swapped := make([]byte, len(bin))
		for i := 0; i < len(bin); i += 2 {
			swapped[i], swapped[i+1] = bin[i+1], bin[i]
		}
		bin = swapped
	}
	if bin[0] != 0xFF || bin[1] != 0xFF || bin[2] != 0xEF || bin[3] != 0xFC {
		return fmt.Errorf("error: bin doesn't appear to be for a Trionic 7 ECU! (%02X%02X%02X%02X)",
			bin[0], bin[1], bin[2], bin[3])
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

	//if err := t.c.Adapter().SetFilter([]uint32{0x258}); err != nil {
	//	t.cfg.OnError(err)
	//}

	t.cfg.OnProgress(-float64(0x80000))
	t.cfg.OnMessage("Flashing ECU")

	start := time.Now()
	for _, seg := range computeWriteSegments(bin) {
		binPos := seg.start // persists across attempts; only advances on successful writes
		err := retry.Do(func() error {
			// re-anchor the ECU write pointer at the last good position so a retry
			// resumes where it failed instead of re-flashing the whole segment
			if err := t.writeJump(ctx, binPos, seg.end-binPos); err != nil {
				return err
			}
			for binPos < seg.end {
				left := seg.end - binPos
				writeBytes := min(left, flashChunkSize)
				if err := t.writeRange(ctx, binPos, binPos+writeBytes, bin); err != nil {
					return err
				}
				binPos += writeBytes
				t.cfg.OnProgress(float64(binPos))
			}
			return nil
		},
			retry.Context(ctx),
			retry.Attempts(30),
			retry.OnRetry(func(n uint, err error) {
				t.cfg.OnMessage(fmt.Sprintf("retrying writeRange: %v", err))
			}),
			retry.Delay(150*time.Millisecond),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return err
		}

	}
	end, err := t.c.SendAndWait(ctx, gocan.NewFrame(0x240, []byte{0x40, 0xA1, 0x01, 0x37, 0x00, 0x00, 0x00, 0x00}, gocan.ResponseRequired), t.defaultTimeout, 0x258)
	if err != nil {
		return fmt.Errorf("error waiting for data transfer exit reply: %v", err)
	}
	// Send acknowledgement
	t.Ack(end.Data[0], gocan.Outgoing)

	if end.Data[3] != 0x77 {
		return errors.New("exit download mode failed")
	}

	t.cfg.OnMessage(fmt.Sprintf("Done, took: %s", time.Since(start).Round(time.Second)))
	// defer t.StopSession(ctx)
	return nil
}

// send request "Download - tool to module" to Trionic"
func (t *Client) writeJump(ctx context.Context, offset, length int) error {
	jumpMsg := []byte{0x41, 0xA1, 0x08, 0x34, byte(offset >> 16), byte(offset >> 8), byte(offset), 0x00}
	jumpMsg2 := []byte{0x00, 0xA1, byte(length >> 16), byte(length >> 8), byte(length) /*, 0x00, 0x00, 0x00*/}

	log.Printf("writeJump: offset=%d, length=%d", offset, length)
	log.Printf("writeJump: jumpMsg=%X", jumpMsg)
	if err := t.c.Send(0x240, jumpMsg, gocan.Outgoing); err != nil {
		return fmt.Errorf("failed to enable request download #1: %w", err)
	}
	log.Printf("writeJump: jumpMsg2=%X", jumpMsg2)

	f, err := t.c.SendAndWait(ctx, gocan.NewFrame(0x240, jumpMsg2, gocan.ResponseRequired), t.defaultTimeout, 0x258)
	if err != nil {
		return fmt.Errorf("failed to enable request download #2: %w", err)
	}

	t.Ack(f.Data[0], gocan.Outgoing)

	if f.Data[3] != 0x74 {
		log.Println(f.String())
		return fmt.Errorf("invalid response enabling download mode")
	}

	return nil
}

func (t *Client) writeRange(ctx context.Context, start, end int, bin []byte) error {
	length := end - start
	binPos := start
	rows := (length + 3) / 6
	first := true
	data := make([]byte, 8)
	var resp *gocan.CANFrame
	for i := rows; i >= 0; i-- {
		data[1] = 0xA1
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		data[0] = byte(i)
		if first {
			data[0] |= 0x40
			data[2] = byte(length + 1) // length
			data[3] = 0x36             // Data Transfer
			for fq := 4; fq < 8; fq++ {
				data[fq] = bin[binPos]
				binPos++
			}
			first = false
		} else if i == 0 {
			left := end - binPos
			if left > 6 {
				return fmt.Errorf("sequence is fucked, tell roffe") // this should never happend
			}
			for i := 0; i < left; i++ {
				data[2+i] = bin[binPos]
				binPos++
			}
			if left <= 6 {
				fill := 8 - left
				for i := left; i < fill; i++ {
					data[left+2] = 0x00
				}
			}
		} else {
			for k := 2; k < 8; k++ {
				data[k] = bin[binPos]
				binPos++
			}
		}
		if i > 0 {
			// block until the adapter has written this frame, pacing the burst to
			// the ECU (degrades to async send on adapters that don't confirm writes)
			if err := t.c.SendSync(ctx, gocan.NewFrame(0x240, data, gocan.Outgoing), t.defaultTimeout); err != nil {
				return fmt.Errorf("error writing 0x%X - 0x%X was at pos 0x%X: %w", start, end, binPos, err)
			}
			continue
		}
		// last frame: SendAndWait registers the 0x258 subscription before sending,
		// so a fast ECU reply can't slip through between the send and the receive
		r, err := t.c.SendAndWait(ctx, gocan.NewFrame(0x240, data, gocan.ResponseRequired), t.defaultTimeout, 0x258)
		if err != nil {
			return fmt.Errorf("error writing 0x%X - 0x%X was at pos 0x%X: %w", start, end, binPos, err)
		}
		resp = r
	}

	// Send acknowledgement
	t.Ack(resp.Data[0], gocan.Outgoing)

	if resp.Data[3] != 0x76 {
		if resp.Data[3] == 0x7F {
			return fmt.Errorf("ECU rejected write 0x%X-0x%X: %w", start, end, TranslateErrorCode(resp.Data[5]))
		}
		return fmt.Errorf("ECU did not confirm write 0x%X-0x%X: %s", start, end, resp.String())
	}
	return nil
}
