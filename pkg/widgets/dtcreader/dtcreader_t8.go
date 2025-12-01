package dtcreader

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/txlogger/pkg/dtc"
)

func (d *DTCReader) readT8DTCS(ctx context.Context, cl *gocan.Client) {
	gm := gmlan.New(cl, 0x7e0, 0x7e8)

	if err := gm.InitiateDiagnosticOperation(ctx, gmlan.LEV_DADTC); err != nil {
		d.err(err)
		return
	}

	defer func() {
		_ = gm.ReturnToNormalMode(ctx)
		time.Sleep(75 * time.Millisecond)
	}()

	dtcs, err := gm.ReadDiagnosticInformation(ctx, 0x81, 0x12)
	if err != nil {
		d.err(err)
		return
	}

	var ddtcs []dtc.DTC
	for _, d := range dtcs {
		ddtcs = append(ddtcs, dtc.DTC{
			ECU:    dtc.ECU_T8,
			Code:   d.Code,
			Status: d.Status,
		})
	}

	d.dtcs = ddtcs
	fyne.Do(d.Refresh)
}

func (d *DTCReader) clearT8DTCS(ctx context.Context, cl *gocan.Client) {
	gm := gmlan.New(cl, 0x7e0, 0x7e8)

	if err := gm.InitiateDiagnosticOperation(ctx, gmlan.LEV_DADTC); err != nil {
		d.err(err)
		return
	}
	defer func() {
		_ = gm.ReturnToNormalMode(ctx)
		time.Sleep(75 * time.Millisecond)
	}()
	//if err := gm.ClearDiagnosticInformation(ctx, 0x7e0); err != nil {
	if err := gm.ClearDiagnosticInformation(ctx, 0x7DF); err != nil {
		d.err(err)
		return
	}
	/*
		dtcs, err := gm.ReadDiagnosticInformation(ctx, 0x81, 0x12)
		if err != nil {
			d.err(err)
			return
		}

		var ddtcs []dtc.DTC
		for _, d := range dtcs {
			newDtc := dtc.DTC{
				Code:   d.Code,
				Status: d.Status,
			}
			ddtcs = append(ddtcs, newDtc)
			log.Println(newDtc.String())
		}
		d.dtcs = ddtcs
	*/
	d.dtcs = []dtc.DTC{}
	fyne.Do(d.Refresh)
}
