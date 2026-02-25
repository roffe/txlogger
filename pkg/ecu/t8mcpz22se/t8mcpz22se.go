package t8mcpz22se

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t8legion"
	"github.com/roffe/txlogger/pkg/model"
)

func init() {
	ecu.Register(&ecu.EcuInfo{
		Name:    "Trionic 8 MCP z22se",
		NewFunc: New,
		CANRate: 500,
		Filter:  []uint32{0x7E8},
	})
}

type Client struct {
	c              *gocan.Client
	cfg            *ecu.Config
	defaultTimeout time.Duration
	legion         *t8legion.Client
}

func New(c *gocan.Client, cfg *ecu.Config) ecu.Client {
	t := &Client{
		c:              c,
		cfg:            ecu.LoadConfig(cfg),
		defaultTimeout: 150 * time.Millisecond,
		legion:         t8legion.New(c, cfg, 0x7e0, 0x7e8),
	}
	return t
}

func (t *Client) ReadDTC(ctx context.Context) ([]dtc.DTC, error) {
	return nil, errors.New("MCP cannot do this")
}

func (t *Client) Info(ctx context.Context) ([]model.HeaderResult, error) {
	return nil, nil
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	return nil
}

func (t *Client) FlashECU(ctx context.Context, bin []byte) error {
	return nil
}

func (t *Client) DumpECU(ctx context.Context) ([]byte, error) {
	if err := t.legion.Bootstrap(ctx, true); err != nil {
		return nil, err
	}

	_, err := t.legion.IDemand(ctx, t8legion.StartSecondaryBootloader, 0)
	if err != nil {
		return nil, errors.New("failed to start secondary bootloader")
	}
	t.cfg.OnMessage("Dumping MCP")

	start := time.Now()

	bin, err := t.legion.ReadFlash(ctx, t8legion.EcuByte_MCP, 0x40100)
	if err != nil {
		return nil, err
	}

	ecumd5bytes, err := t.legion.IDemand(ctx, t8legion.GetTrionic8MCPMD5, 0x00)
	if err != nil {
		return nil, err
	}
	calculatedMD5 := md5.Sum(bin)

	t.cfg.OnMessage(fmt.Sprintf("Remote md5 : %X", ecumd5bytes))
	t.cfg.OnMessage(fmt.Sprintf("Local md5  : %X", calculatedMD5))

	if !bytes.Equal(ecumd5bytes, calculatedMD5[:]) {
		return nil, errors.New("md5 Verification failed")
	}

	t.cfg.OnMessage("Done, took: " + time.Since(start).String())

	return bin, nil
}

func (t *Client) EraseECU(ctx context.Context) error {
	return nil
}

func (t *Client) ResetECU(ctx context.Context) error {
	if t.legion.IsRunning() {
		if err := t.legion.Exit(ctx); err != nil {
			return err
		}
	}
	return nil
}
