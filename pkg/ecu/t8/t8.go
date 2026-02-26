package t8

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t8legion"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
)

func init() {
	ecu.Register(&ecu.EcuInfo{
		Name:    "Trionic 8",
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

func (t *Client) EraseECU(ctx context.Context) error {
	return nil
}

func (t *Client) RequestSecurityAccess(ctx context.Context) error {
	log.Println("Requesting t8 security access")
	return t.gm.RequestSecurityAccess(ctx, 0x01, 0, t8sec.CalculateAccessKey)
}

func (t *Client) GetOilQuality(ctx context.Context) (float64, error) {
	resp, err := t.RequestECUInfoAsUint64(ctx, pidOilQuality)
	if err != nil {
		return 0, err
	}
	quality := float64(resp) / 256
	return quality, nil
}

func (t *Client) SetOilQuality(ctx context.Context, quality float64) error {
	return t.gm.WriteDataByIdentifierUint32(ctx, pidOilQuality, uint32(quality*256))
}

func (t *Client) GetTopSpeed(ctx context.Context) (uint16, error) {
	resp, err := t.gm.ReadDataByIdentifierUint16(ctx, pidTopSpeed)
	if err != nil {
		return 0, err
	}
	speed := resp / 10
	return speed, nil
}

func (t *Client) SetTopSpeed(ctx context.Context, speed uint16) error {
	speed *= 10
	return t.gm.WriteDataByIdentifierUint16(ctx, pidTopSpeed, speed)
}

func (t *Client) GetRPMLimiter(ctx context.Context) (uint16, error) {
	return t.gm.ReadDataByIdentifierUint16(ctx, pidRPMLimiter)

}

func (t *Client) SetRPMLimit(ctx context.Context, limit uint16) error {
	return t.gm.WriteDataByIdentifierUint16(ctx, pidRPMLimiter, limit)
}

func (t *Client) GetVehicleVIN(ctx context.Context) (string, error) {
	return t.gm.ReadDataByIdentifierString(ctx, pidVIN)
}

func (t *Client) SetVehicleVIN(ctx context.Context, vin string) error {
	if len(vin) != 17 {
		return errors.New("invalid vin length")
	}
	return t.gm.WriteDataByIdentifier(ctx, pidVIN, []byte(vin))
}

func (t *Client) GetE85Percent(ctx context.Context) (float64, error) {
	resp, err := t.gm.ReadDataByPacketIdentifier(ctx, 0x01, dpidE85Percent)
	if err != nil {
		return 0, err
	}
	val := binary.LittleEndian.Uint32(resp[2:6])
	return float64(val), nil
}

func (t *Client) SetE85Percent(ctx context.Context, percent float64) error {
	val := uint32(percent)
	data := []byte{0x00, byte(val)}

	if err := t.gm.DeviceControlWithCode(ctx, 0x18, data); err != nil {
		return err
	}
	return nil
}

const (
	pidRPMLimiter  = 0x29
	pidOilQuality  = 0x25
	pidTopSpeed    = 0x02
	pidVIN         = 0x90
	dpidE85Percent = 0x7A
)
