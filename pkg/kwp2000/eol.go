package kwp2000

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/roffe/gocan"
)

// Start EOL programming session
func (t *Client) StartEOL(ctx context.Context) error {
	log.Println("Start EOL")
	count := 0
	payload := []byte{0x40, 0xA1, 0x02, START_ROUTINE_BY_IDENTIFIER, RLI_EOL_START, 0x00, 0x00, 0x00}
	for data := make([]byte, 8); data[3] != 0x71 && count < 30; {
		f, err := t.c.SendAndWait(ctx, gocan.NewFrame(REQ_MSG_ID, payload, gocan.ResponseRequired), t.defaultTimeout, t.responseID)
		if err != nil {
			return err
		}
		t.Ack(f.Data[0], gocan.Outgoing)
		count++
		if count > 10 {
			return errors.New("failed to start EOL session")
		}
		time.Sleep(250 * time.Millisecond)
		fmt.Print(".")
	}
	return nil
}

func (t *Client) EndEOL(ctx context.Context) error {
	log.Println("End EOL")
	payload := []byte{0x40, 0xA1, 0x02, START_ROUTINE_BY_IDENTIFIER, RLI_END_EOL, 0x00, 0x00, 0x00}
	f, err := t.c.SendAndWait(ctx, gocan.NewFrame(REQ_MSG_ID, payload, gocan.ResponseRequired), t.defaultTimeout, t.responseID)
	if err != nil {
		return err
	}
	log.Println(f)
	if f.Data[3] == 0x7F {
		return fmt.Errorf("EndEOL: %w", TranslateErrorCode(f.Data[5]))
	}

	return nil
}
