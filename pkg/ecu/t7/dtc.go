package t7

import (
	"context"

	"github.com/roffe/txlogger/pkg/dtc"
)

// readDTCStatus selects which stored DTCs to report. Bits 5-6 are the storage
// state filter; 0x02 leaves them clear, which the firmware reads as "all DTCs".
const readDTCStatus = 0x02

func (t *Client) ReadDTC(ctx context.Context) ([]dtc.DTC, error) {
	if err := t.DataInitialization(ctx); err != nil {
		return nil, err
	}
	defer t.StopSession(ctx)
	return t.kwp.ReadDTCByStatus(ctx, readDTCStatus)
}
