package t7

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/ecu"
)

func init() {
	ecu.Register(&ecu.EcuInfo{
		Name:    "Trionic 7",
		NewFunc: New,
		CANRate: 500,
		Filter:  []uint32{0x238, 0x258, 0x266, 0x270},
	})
}

// Client drives a Trionic 7. All wire traffic goes through pkg/kwp2000; this
// type owns the T7 specifics: session lifecycle, seed/key selection, bin
// layout and progress reporting.
type Client struct {
	kwp *t7kwp.Client
	cfg *ecu.Config

	// lastInit is when the diagnostic session was last opened. The ECU ignores
	// startCommunication while a session is live and only drops the session
	// after ~6 s of silence, so a recent init is reused instead of re-sent.
	lastInit time.Time
}

func New(c *gocan.Bus, cfg *ecu.Config) ecu.Client {
	kwp := t7kwp.New(c)
	kwp.SetResponseID(0x258) // T7 always answers here; StartSession confirms it
	return &Client{
		kwp: kwp,
		cfg: ecu.LoadConfig(cfg),
	}
}

func (t *Client) MarryECU(context.Context, string) error {
	return errors.New("not supported")
}

func (t *Client) RecoverECU(context.Context, []byte) error {
	return errors.New("not supported")
}

// DataInitialization opens the diagnostic session (startCommunication on 0x220).
func (t *Client) DataInitialization(ctx context.Context) error {
	if !t.lastInit.IsZero() && time.Since(t.lastInit) < 8*time.Second {
		return nil
	}

	err := retry.Do(
		func() error { return t.kwp.StartSession(ctx, t7kwp.INIT_MSG_ID, t7kwp.INIT_RESP_ID) },
		retry.Context(ctx),
		retry.Attempts(6),
		retry.OnRetry(func(n uint, err error) {
			if n == 0 {
				t.cfg.OnError(err)
				return
			}
			t.cfg.OnMessage(fmt.Sprintf("Retry #%d, %v", n, err))
		}),
		retry.LastErrorOnly(true),
		retry.Delay(250*time.Millisecond),
	)
	if err != nil {
		return fmt.Errorf("/!\\ Data initialization failed: %v", err)
	}
	t.lastInit = time.Now()
	return nil
}

// StopSession ends the diagnostic session. The init cache goes with it, so the
// next operation opens a fresh session instead of talking into a dead one.
func (t *Client) StopSession(ctx context.Context) error {
	t.lastInit = time.Time{}
	return t.kwp.StopSession(ctx)
}

// GetHeader reads an ECU identification field (KWP2000 service 0x1A).
func (t *Client) GetHeader(ctx context.Context, id byte) (string, error) {
	b, err := t.kwp.ReadECUIdentification(ctx, id)
	if err != nil {
		return "", fmt.Errorf("failed getting header: %w", err)
	}
	return string(b), nil
}

func (t *Client) KnockKnock(ctx context.Context) (bool, error) {
	if err := t.DataInitialization(ctx); err != nil {
		return false, err
	}
	keys := t7kwp.KnownSeedKeys
	if sk := t.cfg.SeedKey; sk != nil {
		// user pair first, stock pairs kept as fallback
		keys = append([]t7kwp.SeedKey{*sk}, keys...)
		t.cfg.OnMessage(fmt.Sprintf("Using custom seed/key XOR %04X SUB %04X", sk.XOR, sk.Sub))
	}
	for _, k := range keys {
		ok, err := t.kwp.SecurityAccess(ctx, t7kwp.DEVELOPMENT_PRIORITY, k)
		if err != nil {
			t.cfg.OnError(fmt.Errorf("/!\\ Failed to obtain security access: %v", err))
			time.Sleep(3 * time.Second)
			continue
		}
		if ok {
			t.cfg.OnMessage(fmt.Sprintf("Security access obtained (XOR %04X SUB %04X)", k.XOR, k.Sub))
			return true, nil
		}
	}
	return false, errors.New("/!\\ Failed to obtain security access")
}
