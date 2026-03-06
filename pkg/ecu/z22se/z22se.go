package z22se

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t8legion"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
)

func init() {
	ecu.Register(&ecu.EcuInfo{
		Name:    "Z22SE",
		NewFunc: New,
		CANRate: 500,
		Filter:  []uint32{0x5E8, 0x7E8},
	})
}

type Client struct {
	c              *gocan.Client
	defaultTimeout time.Duration
	legion         *t8legion.Client
	gm             *gmlan.Client
	cfg            *ecu.Config
}

// ReadDTC implements [ecu.Client].
func (t *Client) ReadDTC(context.Context) ([]dtc.DTC, error) {
	return nil, nil
}

func New(c *gocan.Client, cfg *ecu.Config) ecu.Client {
	t := &Client{
		c:              c,
		cfg:            ecu.LoadConfig(cfg),
		defaultTimeout: 150 * time.Millisecond,
		legion:         t8legion.New(c, cfg, 0x7e0, 0x7e8),
		gm:             gmlan.New(c, 0x7e0, 0x5e8, 0x7e8),
	}
	return t
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	return nil
}

func (t *Client) ResetECU(ctx context.Context) error {
	if t.legion.IsRunning() {
		err := retry.Do(func() error {
			return t.legion.Exit(ctx)
		},
			retry.Attempts(3),
			retry.Delay(400*time.Millisecond),
			retry.Context(ctx),
			retry.LastErrorOnly(true),
		)
		if err != nil {
			return fmt.Errorf("failed to exit legion: %w", err)
		}
	}
	return nil
}

func (t *Client) DumpECU(ctx context.Context) ([]byte, error) {
	if err := t.legion.Bootstrap(ctx, true); err != nil {
		return nil, err
	}

	t.cfg.OnMessage("Dumping ECU")
	start := time.Now()

	bin, err := t.legion.ReadFlash(ctx, t8legion.EcuByte_T8, 0x100000)
	if err != nil {
		return nil, err
	}

	t.cfg.OnMessage("Verifying md5..")

	ecuMD5bytes, err := t.legion.IDemand(ctx, t8legion.GetTrionic8MD5, 0x00)
	if err != nil {
		return nil, err
	}
	calculatedMD5 := md5.Sum(bin)

	t.cfg.OnMessage(fmt.Sprintf("Remote MD5 : %X", ecuMD5bytes))
	t.cfg.OnMessage(fmt.Sprintf("Local MD5  : %X", calculatedMD5))

	if !bytes.Equal(ecuMD5bytes, calculatedMD5[:]) {
		return nil, errors.New("md5 Verification failed")
	}

	t.cfg.OnMessage("Done, took: " + time.Since(start).String())

	return bin, nil
}

func (t *Client) FlashECU(ctx context.Context, bin []byte) error {
	if len(bin) != 0x100000 {
		return errors.New("err: Invalid Z22SE file size")
	}
	if err := t.legion.Bootstrap(ctx, true); err != nil {
		return err
	}

	fmask, err := t.legion.DeterminePartitionmask(ctx, bin, t8legion.EcuByte_T8, true, true, true)
	if err != nil {
		return err
	}

	if fmask == 0 {
		t.cfg.OnMessage("Noting to flash, ecu and local bin are same.. returning")
		return nil
	}

	err = t.legion.EraseFlash(ctx, t8legion.EcuByte_T8, fmask)
	if err != nil {
		return err
	}
	start := time.Now()
	err = t.legion.WriteFlash(ctx, t8legion.EcuByte_T8, 0x100000, bin, fmask)
	if err != nil {
		return err
	}
	t.cfg.OnMessage("Done, took: " + time.Since(start).String())

	status, err := t.legion.VerifyFlash(ctx, bin, t8legion.EcuByte_T8, fmask)
	if err != nil {
		return err
	}
	if !status {
		return errors.New("failed md5 verification")
	}
	t.cfg.OnMessage("Verifying md5: sucess")

	return nil
}

func (t *Client) EraseECU(ctx context.Context) error {
	return t.legion.EraseFlash(ctx, t8legion.EcuByte_T8, uint64(math.MaxUint64))
}

func (t *Client) RequestSecurityAccess(ctx context.Context) error {
	log.Println("Requesting t8 security access")
	return t.gm.RequestSecurityAccess(ctx, 0x01, 0, t8sec.CalculateAccessKey)
}
