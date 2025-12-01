package dtcreader

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

func (d *DTCReader) readT7DTCS(ctx context.Context, cl *gocan.Client) {
	kwp := kwp2000.New(cl)
	if err := kwp.StartSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {
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

	d.dtcs = dtcs
	fyne.Do(d.Refresh)
}

func (d *DTCReader) clearT7DTCS(ctx context.Context, cl *gocan.Client) {
	kwp := kwp2000.New(cl)

	if err := kwp.StartSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {
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
