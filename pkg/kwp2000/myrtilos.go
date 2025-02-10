package kwp2000

import (
	"context"
	"fmt"

	"github.com/roffe/gocan"
)

func (t *Client) SendEU0DRegistrationKey(ctx context.Context, key []byte) error {
	payload := append([]byte{0x40, 0xA1, 0x05, EU0D_SET_REGISTRATION_KEY}, key...)
	frame := gocan.NewFrame(REQ_MSG_ID, payload, gocan.ResponseRequired)
	//	log.Println("Send>", frame.String())
	resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
	if err != nil {
		return fmt.Errorf("SetEU0DRegistrationKey: %w", err)
	}
	//	log.Println("Recv>", resp.String())
	return checkErr(resp)
}

func (t *Client) SendEU0DRegistrationKeyLong(ctx context.Context, key []byte) error {
	return t.sendLong(ctx, append([]byte{0x06, EU0D_SET_REGISTRATION_KEY}, key...))
}
