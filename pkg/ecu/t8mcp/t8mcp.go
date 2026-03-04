package t8mcp

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t8legion"
	"github.com/roffe/txlogger/pkg/model"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

func init() {
	ecu.Register(&ecu.EcuInfo{
		Name:    "Trionic 8 MCP",
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
	if err := t.legion.Bootstrap(ctx, false); err != nil {
		return nil, err
	}

	_, err := t.legion.IDemand(ctx, t8legion.StartSecondaryBootloader, 0)
	if err != nil {
		return nil, errors.New("failed to start secondary bootloader")
	}

	ver, err := t.legion.GetMCPVersion(ctx)
	if err != nil {
		return nil, err
	}

	t.cfg.OnMessage("MCP Firmware information: " + ver)

	return nil, nil
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	return nil
}

func (t *Client) FlashECU(ctx context.Context, bin []byte) error {
	if len(bin) != 0x40100 {
		return errors.New("err: Invalid T8 MCP file size")
	}
	if err := t.legion.Bootstrap(ctx, false); err != nil {
		return err
	}
	if err := t.legion.StartSecondaryBootloader(ctx); err != nil {
		return err
	}
	boot := fyne.CurrentApp().Preferences().BoolWithFallback(settings.PrefsBoot, false)
	fmask, err := t.legion.DeterminePartitionmask(ctx, bin, t8legion.EcuByte_MCP, boot, true, false)
	if err != nil {
		return err
	}
	if fmask == 0 {
		t.cfg.OnMessage("Noting to flash, ecu and local bin are same.. returning")
		return nil
	}
	err = t.legion.EraseFlash(ctx, t8legion.EcuByte_MCP, fmask)
	if err != nil {
		return err
	}
	start := time.Now()
	err = t.legion.WriteFlash(ctx, t8legion.EcuByte_MCP, 0x40100, bin, fmask)
	if err != nil {
		return err
	}
	t.cfg.OnMessage("Done, took: " + time.Since(start).String())

	status, err := t.legion.VerifyFlash(ctx, bin, t8legion.EcuByte_MCP, fmask)
	if err != nil {
		return err
	}
	if !status {
		return errors.New("failed md5 verification")
	}
	t.cfg.OnMessage("Verifying md5: sucess")

	err = t.legion.MarryMCP(ctx)

	return err
}

func (t *Client) DumpECU(ctx context.Context) ([]byte, error) {
	if err := t.legion.Bootstrap(ctx, false); err != nil {
		return nil, err
	}
	if err := t.legion.StartSecondaryBootloader(ctx); err != nil {
		return nil, err
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
