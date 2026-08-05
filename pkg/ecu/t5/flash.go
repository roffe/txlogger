package t5

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
)

const blockSize = 0x80

// flashPlan validates the bin against the connected box and decides where and
// how to write it. The box type is determined from the FLASH chip id alone so
// this also works when the footer has already been erased (recovery).
//
// A half-size (T5.2) bin on a 256 kB box is written twice back to back so the
// chip is filled and the CPU finds its reset vectors at chip offset 0.
func flashPlan(flashSizeKB uint16, binLength int) (start uint32, copies int, err error) {
	switch {
	case flashSizeKB == 128 && binLength == 0x20000:
		return 0x60000, 1, nil
	case flashSizeKB == 128:
		return 0, 0, fmt.Errorf("bin is %d bytes, a T5.2 box needs a 0x20000 byte bin", binLength)
	case flashSizeKB == 256 && binLength == 0x40000:
		return 0x40000, 1, nil
	case flashSizeKB == 256 && binLength == 0x20000:
		return 0x40000, 2, nil
	case flashSizeKB == 256:
		return 0, 0, fmt.Errorf("bin is %d bytes, a T5.5 box needs a 0x20000 or 0x40000 byte bin", binLength)
	}
	return 0, 0, errors.New("unknown FLASH chip type, cannot flash this ECU")
}

func (t *Client) FlashECU(ctx context.Context, bin []byte) error {
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx); err != nil {
			return err
		}
	}

	chip, err := t.GetChipTypes(ctx)
	if err != nil {
		return err
	}

	start, copies, err := flashPlan(flashSizeFromChip(chip[5]), len(bin))
	if err != nil {
		// Nothing has been touched yet: restart the ECU so it keeps
		// running its current firmware.
		_ = t.ResetECU(ctx)
		return err
	}

	// Refuse to flash a bin the ECU itself would report as broken after
	// flashing. This happens before erase, so the ECU is still intact.
	if err := t.ValidateBinChecksum(bin); err != nil {
		_ = t.ResetECU(ctx)
		return fmt.Errorf("refusing to flash: %w", err)
	}

	if copies == 2 {
		t.cfg.OnMessage("T5.2 bin on a T5.5 box, flashing two copies back to back")
	}

	if err := t.EraseECU(ctx); err != nil {
		return dirtyFlash(err)
	}

	image := bytes.Repeat(bin, copies)

	t.cfg.OnProgress(-float64(len(image)))
	t.cfg.OnMessage("Flashing ECU")

	startTime := time.Now()
	for offset := 0; offset < len(image); offset += blockSize {
		block := image[offset : offset+blockSize]
		if err := t.writeBlock(ctx, start+uint32(offset), block); err != nil {
			return dirtyFlash(fmt.Errorf("flashing failed after 0x%X bytes: %w", offset, err))
		}
		t.cfg.OnProgress(float64(offset + blockSize))
	}

	t.cfg.OnMessage(fmt.Sprintf("Done, took: %s", time.Since(startTime).Round(time.Millisecond).String()))

	if err := t.verifyFlash(ctx, start, image, bin); err != nil {
		return dirtyFlash(err)
	}
	return nil
}

// dirtyFlash marks errors that happen once erasing has started: the ECU has
// no valid firmware anymore and the SRAM bootloader is the only way back.
func dirtyFlash(err error) error {
	return fmt.Errorf("%w\nFLASH is NOT intact: do NOT turn off the ignition, retry flashing now! (power loss means BDM recovery)", err)
}

// writeBlock programs one 0x80 byte block. All-0xFF blocks are skipped since
// that is the erased state. Frames are offset-addressed and idempotent so
// transport errors are retried per frame; a programming error reported by the
// bootloader (bad chip) aborts immediately as MyBooty already retried 25
// times internally.
func (t *Client) writeBlock(ctx context.Context, address uint32, block []byte) error {
	ff := true
	for _, b := range block {
		if b != 0xFF {
			ff = false
			break
		}
	}
	if ff {
		return nil
	}

	err := retry.Do(
		func() error { return t.sendBootloaderAddressCommand(ctx, address, blockSize) },
		retry.Context(ctx),
		retry.Attempts(3),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return err
	}

	payload := make([]byte, 8)
	for i := 0; i < blockSize; i++ {
		if i%7 == 0 {
			payload[0] = byte(i)
		}
		payload[(i%7)+1] = block[i]
		if i%7 == 6 || i == blockSize-1 {
			err := retry.Do(
				func() error { return t.sendBootloaderDataCommand(ctx, payload) },
				retry.Context(ctx),
				retry.Attempts(3),
				retry.LastErrorOnly(true),
				retry.RetryIf(func(err error) bool {
					var pe *flashProgramError
					return !errors.As(err, &pe)
				}),
				retry.OnRetry(func(n uint, err error) {
					t.cfg.OnError(fmt.Errorf("retrying data frame at 0x%06X: %w", address, err))
				}),
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// verifyFlash asks the bootloader to checksum the flash (C8). MyBooty
// verifies the calculated checksum against the one stored in flash itself;
// on top of that the value is compared against the checksum of the bin we
// just sent.
//
// C8 only covers [ROM_Offset..Code_End] of the upper image, so the blocks it
// misses are read back and compared as well: the reset vectors at the start
// of flash (what the CPU actually boots from, unverified on a mirrored
// T5.2-on-T5.5 flash) and the footer block at the end.
func (t *Client) verifyFlash(ctx context.Context, start uint32, image, bin []byte) error {
	t.cfg.OnMessage("Verifying FLASH checksum...")
	ecuSum, err := t.GetECUChecksum(ctx)
	if err != nil {
		return fmt.Errorf("post-flash verification failed: %w", err)
	}
	binSum, err := t.CalculateBinChecksum(bin)
	if err != nil {
		return fmt.Errorf("post-flash verification failed: %w", err)
	}
	if !bytes.Equal(ecuSum, binSum) {
		return fmt.Errorf("post-flash verification failed: ECU checksum %X, bin checksum %X", ecuSum, binSum)
	}

	for _, check := range []struct {
		address uint32
		want    []byte
	}{
		{start, image[:blockSize]},
		{0x80000 - blockSize, image[len(image)-blockSize:]},
	} {
		got, err := t.readFlash(ctx, check.address, blockSize)
		if err != nil {
			return fmt.Errorf("post-flash read-back at 0x%06X failed: %w", check.address, err)
		}
		if !bytes.Equal(got, check.want) {
			return fmt.Errorf("post-flash read-back mismatch at 0x%06X", check.address)
		}
	}

	t.cfg.OnMessage(fmt.Sprintf("FLASH checksum verified: %X", ecuSum))
	return nil
}
