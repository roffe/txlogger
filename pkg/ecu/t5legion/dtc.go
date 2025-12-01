package t5legion

import (
	"context"
	"errors"

	"github.com/roffe/txlogger/pkg/dtc"
)

func (t *Client) ReadDTC(ctx context.Context) ([]dtc.DTC, error) {
	return nil, errors.New("not implemented yet")
}
