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
	raw, err := t.kwp.ReadDTCByStatus(ctx, readDTCStatus)
	if err != nil {
		return nil, err
	}
	out := make([]dtc.DTC, 0, len(raw))
	for _, d := range raw {
		out = append(out, dtc.DTC{ECU: dtc.ECU_T7, Code: d.Code, Status: d.Status})
	}
	return out, nil
}
