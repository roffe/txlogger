package dtcreader

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/dtc"
)

func (d *DTCReader) readT7DTCS(ctx context.Context, cl *gocan.Bus) {
	kwp := t7kwp.New(cl)
	if err := kwp.StartSession(ctx, t7kwp.INIT_MSG_ID, t7kwp.INIT_RESP_ID); err != nil {
		d.err(err)
		return
	}

	defer func() {
		if err := kwp.StopSession(ctx); err != nil {
			d.err(fmt.Errorf("Error stopping session: %w", err))
		}
		time.Sleep(75 * time.Millisecond)
	}()

	dtcs, err := kwp.ReadDTCByStatus(ctx, 0x02)
	if err != nil {
		d.err(err)
		return
	}

	d.dtcs = make([]dtc.DTC, 0, len(dtcs))
	for _, t := range dtcs {
		d.dtcs = append(d.dtcs, dtc.DTC{
			ECU:    dtc.ECU_T7,
			Code:   t.Code,
			Status: t.Status,
		})
	}
	fyne.Do(d.Refresh)
}

func (d *DTCReader) clearT7DTCS(ctx context.Context, cl *gocan.Bus) {
	kwp := t7kwp.New(cl)

	if err := kwp.StartSession(ctx, t7kwp.INIT_MSG_ID, t7kwp.INIT_RESP_ID); err != nil {
		d.err(err)
		return
	}

	defer func() {
		if err := kwp.StopSession(ctx); err != nil {
			d.err(err)
			return
		}
		time.Sleep(75 * time.Millisecond)
	}()

	if err := kwp.ClearDTCS(ctx); err != nil {
		d.err(err)
		return
	}

	d.dtcs = []dtc.DTC{}
	fyne.Do(d.Refresh)
}
