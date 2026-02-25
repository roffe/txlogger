package t8z22se

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t8legion"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
	"github.com/roffe/txlogger/pkg/ecu/t8util"
	"github.com/roffe/txlogger/pkg/model"
)

func init() {
	ecu.Register(&ecu.EcuInfo{
		Name:    "Trionic 8 z22se",
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

// Info implements [ecu.Client].
func (t *Client) Info(context.Context) ([]model.HeaderResult, error) {
	return nil, nil
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
	if err := t.legion.Bootstrap(ctx, true); err != nil {
		return err
	}
	t.cfg.OnMessage("Comparing MD5's for erase")
	t.cfg.OnProgress(-9)
	t.cfg.OnProgress(0)
	for i := 1; i <= 9; i++ {
		lmd5 := t8util.GetPartitionMD5(bin, 6, i)
		md5, err := t.legion.GetMD5(ctx, t8legion.GetTrionic8MD5, uint16(i))
		if err != nil {
			return err
		}
		t.cfg.OnMessage(fmt.Sprintf("local partition   %d> %X", i, lmd5))
		t.cfg.OnMessage(fmt.Sprintf("remote partition  %d> %X", i, md5))
		t.cfg.OnProgress(float64(i))
	}

	return nil
}

func (t *Client) EraseECU(ctx context.Context) error {
	return nil
}

func (t *Client) RequestSecurityAccess(ctx context.Context) error {
	log.Println("Requesting t8 security access")
	return t.gm.RequestSecurityAccess(ctx, 0x01, 0, t8sec.CalculateAccessKey)
}
