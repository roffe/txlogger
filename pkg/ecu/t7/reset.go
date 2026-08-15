package t7

import (
	"context"
)

func (t *Client) ResetECU(ctx context.Context) error {
	if err := t.DataInitialization(ctx); err != nil {
		return err
	}
	return t.kwp.ResetECU(ctx)
}
