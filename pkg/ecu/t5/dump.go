package t5

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
)

func (t *Client) DumpECU(ctx context.Context) ([]byte, error) {
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx); err != nil {
			return nil, err
		}
	}

	ecutype, err := t.DetermineECU(ctx)
	if err != nil {
		return nil, err
	}

	start := getstartAddress(ecutype)
	length := 0x80000 - start

	t.cfg.OnProgress(-float64(length))
	t.cfg.OnMessage("Dumping ECU")

	buffer := make([]byte, length)
	startTime := time.Now()
	progress := 0

	address := start + 5
	for i := 0; i < int(length/6); i++ {
		err := retry.Do(
			func() error {
				b, err := t.ReadMemoryByAddress(ctx, address)
				if err != nil {
					return err
				}
				copy(buffer[i*6:], b)
				progress += len(b)
				return nil
			},
			retry.OnRetry(func(n uint, err error) {
				t.cfg.OnError(fmt.Errorf("retrying to read memory by address: %w", err))
			}),
			retry.Attempts(3),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return nil, err
		}
		address += 6
		t.cfg.OnProgress(float64(progress))
	}

	// Get the leftover bytes
	if (length % 6) > 0 {
		err := retry.Do(
			func() error {
				b, err := t.ReadMemoryByAddress(ctx, start+length-1)
				if err != nil {
					return err
				}
				for j := (6 - (length % 6)); j < 6; j++ {
					buffer[length-6+j] = b[j]
					progress++
				}
				return nil
			},
			retry.OnRetry(func(n uint, err error) {
				t.cfg.OnError(fmt.Errorf("t5 retrying to read memory by address: %w", err))
			}),
			retry.Attempts(3),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return nil, err
		}
		t.cfg.OnProgress(float64(progress))
	}

	t.cfg.OnProgress(float64(length))
	t.cfg.OnMessage(fmt.Sprintf("Done, took: %s", time.Since(startTime).Round(time.Millisecond).String()))

	// Validate the dump but still return it: a broken checksum can also mean
	// the ECU itself holds a broken bin, which is still worth saving.
	if checksum, err := t.GetECUChecksum(ctx); err != nil {
		t.cfg.OnError(fmt.Errorf("dump warning: %w", err))
	} else if calculated, err := t.CalculateBinChecksum(buffer); err != nil {
		t.cfg.OnError(fmt.Errorf("dump warning: %w", err))
	} else if !bytes.Equal(checksum, calculated) {
		t.cfg.OnError(fmt.Errorf("dump warning: ECU checksum %X does not match dumped data %X, dump may be corrupt", checksum, calculated))
	}

	return buffer, nil
}

func getstartAddress(ecutype ECUType) uint32 {
	switch ecutype {
	case T52ECU, T55AST52:
		return 0x60000
	default:
		return 0x40000
	}
}
