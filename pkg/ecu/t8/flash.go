package t8

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/txlogger/pkg/ecu/t8legion"
	"github.com/roffe/txlogger/pkg/ecu/t8util"
)

func (t *Client) DeterminePartitionmask(ctx context.Context, file []byte, device byte, boot, nvme, z22se bool) (uint64, error) {
	t.cfg.OnMessage(fmt.Sprintf("Determine Partition Mask, format boot: %t, nvme: %t", boot, nvme))
	start := uint32(0)
	formatmask := uint64(0)

	if z22se || boot {
		start = 1
	} else if device == 5 {
		start = 2
	}

	for i := start; i <= 9; i++ {
		lmd5 := t8util.GetPartitionMD5(file, 6, int(i))
		md5, err := t.legion.GetMD5(ctx, t8legion.GetTrionic8MD5, uint16(i))
		if err != nil {
			return 0, err
		}
		if !bytes.Equal(lmd5, md5) {
			formatmask |= uint64(1 << (i - 1))
		}
	}

	if !z22se {
		if !boot {
			formatmask &= 0x1FE
		}
		if !nvme {
			formatmask &= 0x1F9
		}
	}

	if device == 5 {
		formatmask &= uint64(0x1BF)
		formatmask |= uint64((formatmask & 1) << 8)
		if !z22se {
			formatmask &= uint64(0x1BF)
		}
	}

	return formatmask, nil
}

func (t *Client) FlashECU(ctx context.Context, bin []byte) error {
	if len(bin) != 0x100000 {
		return errors.New("err: Invalid T8 file size")
	}

	if err := t.legion.Bootstrap(ctx, false); err != nil {
		return err
	}

	fmask, err := t.DeterminePartitionmask(ctx, bin, t8legion.EcuByte_T8, false, false, false)
	if err != nil {
		return err
	}

	err = t.legion.EraseFlash(ctx, t8legion.EcuByte_T8, fmask)
	if err != nil {
		return err
	}
	start := time.Now()
	err = t.legion.WriteFlash(ctx, t8legion.EcuByte_T8, 0x100000, bin, false)
	if err != nil {
		return err
	}

	t.cfg.OnMessage("Verifying md5..")

	ecuMD5bytes, err := t.legion.IDemand(ctx, t8legion.GetTrionic8MD5, 0x00)
	if err != nil {
		return err
	}
	calculatedMD5 := md5.Sum(bin)

	t.cfg.OnMessage(fmt.Sprintf("Remote MD5 : %X", ecuMD5bytes))
	t.cfg.OnMessage(fmt.Sprintf("Local MD5  : %X", calculatedMD5))

	if !bytes.Equal(ecuMD5bytes, calculatedMD5[:]) {
		return errors.New("md5 Verification failed")
	}

	t.cfg.OnMessage("Done, took: " + time.Since(start).String())

	return nil
}
