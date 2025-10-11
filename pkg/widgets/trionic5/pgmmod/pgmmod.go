package pgmmod

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type Widget struct {
	widget.BaseWidget
	b        boxes
	data     []byte
	content  *fyne.Container
	SaveFunc func([]byte) error
	LoadFunc func() ([]byte, error)
}

type boxes struct {

	// byte 0
	enrichAfterStart  *widget.Check // Enrichment after start (0x01)
	wotEnrich         *widget.Check // WOT enrichment (0x02)
	ipolWait          *widget.Check // IPOL wait (0x04) **
	tempCorr          *widget.Check // Temperature correction (0x08)
	lambdaCtrl        *widget.Check // Lambda control (0x10)
	adaptivity        *widget.Check // Adaptivity	(0x20)
	idleCtrl          *widget.Check // Idle control (0x40)
	enrichDuringStart *widget.Check // Enrichment during start (0x80)

	// byte 1
	constantInjection      *widget.Check // Constant injection (E51) (0x01)
	lambdaCtrlOnTransients *widget.Check // Lambda control on transients (0x02)
	fuelcutInEngineBrake   *widget.Check // Fuel cut in engine brake (0x04)
	constantInjectionTime  *widget.Check // Constant injection time	(0x08)
	accelEnrich            *widget.Check // Acceleration enrichment (0x10)
	decelEnrich            *widget.Check // Deceleration enrichment (0x20)
	bil104                 *widget.Check // Bil 104 (0x40)
	idleFuelAdapt          *widget.Check // Idle fuel adaptation (0x80)

	// byte 2
	lambdaPuff               *widget.Check // Lambda puff (0x01) **
	useIdleInjectionMap      *widget.Check // Use idle injection map (0x02)
	correctionForEngagingAC  *widget.Check // Correction for engaging A/C (0x04)
	amosOn                   *widget.Check // AMOS on (0x08)
	fuelAdjustmentDuringIdle *widget.Check // Fuel adjustment during idle (0x10)
	purgeCtrl                *widget.Check // Purge control (0x20)
	adaptionOfIdleCtrl       *widget.Check // Adaption of idle control (0x40)
	lambdaCtrlDuringIdle     *widget.Check // Lambda control during idle (0x80)

	// byte 3
	heatplates            *widget.Check // Heatplates (0x01)
	automaticTransmission *widget.Check // Automatic Transmission (0x02)
	loadControl           *widget.Check // Load control (0x04)
	etsTcs                *widget.Check // ETS/TCS (0x08)
	boostCtrl             *widget.Check // Boost control (0x10)
	higherIdleDuringStart *widget.Check // Higher idle during start (0x20)
	globalAdaptation      *widget.Check // Global adaptation (0x40)
	tempCorrClosedLoop    *widget.Check // Temperature correction closed loop (0x80)

	// byte 4
	loadBufferingDuringIdle *widget.Check // Load buffering during idle (0x01)
	fixedIdleign            *widget.Check // Fixed idle ignition gear 1 & 2 (0x02)
	noFuelFutInR12          *widget.Check // No fuel fut in R12 (0x04)
	airpumpCtrl             *widget.Check // Airpump control (0x08)
	naengine                *widget.Check // Naturally aspirated engine (0x10)
	knkdetOff               *widget.Check // Knock detection OFF (0x20)
	constantAngle           *widget.Check // Constant angle (0x40)
	purgeValveMY94          *widget.Check // Purge valve MY94 (0x80)

	// byte 5
	tankPressureDiagnostics *widget.Check // Tank pressure diagnostics (0x10)
	vssEnabled              *widget.Check // VSS enabled (0x80)

}

func New() *Widget {
	w := &Widget{
		b: boxes{
			enrichAfterStart:  widget.NewCheck("Enrichment after start", nil),
			wotEnrich:         widget.NewCheck("WOT enrichment", nil),
			ipolWait:          widget.NewCheck("IPOL wait", nil),
			tempCorr:          widget.NewCheck("Temperature correction", nil),
			lambdaCtrl:        widget.NewCheck("Lambda control", nil),
			adaptivity:        widget.NewCheck("Adaptivity", nil),
			idleCtrl:          widget.NewCheck("Idle control", nil),
			enrichDuringStart: widget.NewCheck("Enrichment during start", nil),

			constantInjection:      widget.NewCheck("Constant injection (E51)", nil),
			lambdaCtrlOnTransients: widget.NewCheck("Lambda control on transients", nil),
			fuelcutInEngineBrake:   widget.NewCheck("Fuel cut in engine brake", nil),
			constantInjectionTime:  widget.NewCheck("Constant injection time", nil),
			accelEnrich:            widget.NewCheck("Acceleration enrichment", nil),
			decelEnrich:            widget.NewCheck("Deceleration enrichment", nil),
			bil104:                 widget.NewCheck("Bil 104", nil),
			idleFuelAdapt:          widget.NewCheck("Idle fuel adaptation", nil),

			lambdaPuff:               widget.NewCheck("Correction for TPS opening", nil),
			useIdleInjectionMap:      widget.NewCheck("Use idle injection map", nil),
			correctionForEngagingAC:  widget.NewCheck("Correction for engaging A/C", nil),
			amosOn:                   widget.NewCheck("AMOS on", nil),
			fuelAdjustmentDuringIdle: widget.NewCheck("Fuel adjustment during idle", nil),
			purgeCtrl:                widget.NewCheck("Purge control", nil),
			adaptionOfIdleCtrl:       widget.NewCheck("Adaption of idle control", nil),
			lambdaCtrlDuringIdle:     widget.NewCheck("Lambda control during idle", nil),

			heatplates:            widget.NewCheck("Heatplates", nil),
			automaticTransmission: widget.NewCheck("Automatic Transmission", nil),
			loadControl:           widget.NewCheck("Load control", nil),
			etsTcs:                widget.NewCheck("ETS/TCS", nil),
			boostCtrl:             widget.NewCheck("Boost control", nil),
			higherIdleDuringStart: widget.NewCheck("Higher idle during start", nil),
			globalAdaptation:      widget.NewCheck("Global adaptation", nil),
			tempCorrClosedLoop:    widget.NewCheck("Temp corr in closed loop", nil),

			loadBufferingDuringIdle: widget.NewCheck("Load buffering during idle", nil),
			fixedIdleign:            widget.NewCheck("Fixed idle ignition gear 1 & 2", nil),
			noFuelFutInR12:          widget.NewCheck("No fuel fut in R12", nil),
			airpumpCtrl:             widget.NewCheck("Airpump control", nil),
			naengine:                widget.NewCheck("Naturally aspirated engine", nil),
			knkdetOff:               widget.NewCheck("Knock detection OFF", nil),
			constantAngle:           widget.NewCheck("Constant angle", nil),
			purgeValveMY94:          widget.NewCheck("Purge valve MY94", nil),

			tankPressureDiagnostics: widget.NewCheck("Tank pressure diagnostics", nil),
			vssEnabled:              widget.NewCheck("VSS enabled", nil),
		},
		SaveFunc: func([]byte) error { return nil },
		LoadFunc: func() ([]byte, error) { return nil, nil },
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	w.content = container.NewBorder(
		nil,
		container.NewGridWithColumns(2,
			widget.NewButton("Load", func() {
				b, err := w.LoadFunc()
				if err != nil {
					dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])

				}
				log.Printf("Load: %X", b)
				w.Set(b)
			}),
			widget.NewButton("Save", func() {
				log.Printf("Save: %X", w.Get())
				err := w.SaveFunc(w.Get())
				if err != nil {
					dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
				}
			}),
		),
		nil,
		nil,
		container.NewGridWithColumns(4,
			w.b.enrichAfterStart, w.b.wotEnrich, w.b.ipolWait, w.b.tempCorr,
			w.b.lambdaCtrl, w.b.adaptivity, w.b.idleCtrl, w.b.enrichDuringStart,

			w.b.constantInjection, w.b.lambdaCtrlOnTransients, w.b.fuelcutInEngineBrake, w.b.constantInjectionTime,
			w.b.accelEnrich, w.b.decelEnrich, w.b.bil104, w.b.idleFuelAdapt,

			w.b.lambdaPuff, w.b.useIdleInjectionMap, w.b.correctionForEngagingAC, w.b.amosOn,
			w.b.fuelAdjustmentDuringIdle, w.b.purgeCtrl, w.b.adaptionOfIdleCtrl, w.b.lambdaCtrlDuringIdle,

			w.b.heatplates, w.b.automaticTransmission, w.b.loadControl, w.b.etsTcs,
			w.b.boostCtrl, w.b.higherIdleDuringStart, w.b.globalAdaptation, w.b.tempCorrClosedLoop,

			w.b.loadBufferingDuringIdle, w.b.fixedIdleign, w.b.noFuelFutInR12, w.b.airpumpCtrl,
			w.b.naengine, w.b.knkdetOff, w.b.constantAngle, w.b.purgeValveMY94,

			w.b.tankPressureDiagnostics, w.b.vssEnabled,
		),
	/*
		container.NewGridWithColumns(3,
			w.b.automaticTransmission, w.b.etsTcs, w.b.heatplates,
			w.b.naengine, w.b.airpumpCtrl, w.b.boostCtrl,
			w.b.knkdetOff, w.b.fixedIdleign, w.b.purgeCtrl,
			w.b.purgeValveMY94, w.b.vssEnabled, w.b.accelEnrich,
			w.b.decelEnrich, w.b.wotEnrich, w.b.enrichDuringStart,
			w.b.enrichAfterStart, w.b.adaptionOfIdleCtrl, w.b.adaptivity,
			w.b.adaptivityWithClosedThrtle, w.b.tankPressureDiagnostics,
			w.b.globalAdaptation, w.b.constantInjection, w.b.fuelcutInEngineBrake,
			w.b.loadControl, w.b.noFuelFutInR12, w.b.constantInjectionTime,
			w.b.higherIdleDuringStart, w.b.idleCtrl, w.b.loadBufferingDuringIdle,
			w.b.useIdleInjectionMap, w.b.correctionForEngagingAC, w.b.correctionForTpsOpening,
			w.b.lambdaCtrl, w.b.lambdaCtrlDuringIdle, w.b.lambdaCtrlOnTransients,
			w.b.enableSecondLambda, w.b.fuelAdjustmentDuringIdle, w.b.tempCorr,
			w.b.tempCorrClosedLoop,
		),
	*/
	)

	return &renderer{w: w}
}

func (w *Widget) clearChecks() {
	w.b.enrichAfterStart.SetChecked(false)
	w.b.wotEnrich.SetChecked(false)
	w.b.ipolWait.SetChecked(false)
	w.b.tempCorr.SetChecked(false)
	w.b.lambdaCtrl.SetChecked(false)
	w.b.adaptivity.SetChecked(false)
	w.b.idleCtrl.SetChecked(false)
	w.b.enrichDuringStart.SetChecked(false)

	w.b.constantInjection.SetChecked(false)
	w.b.lambdaCtrlOnTransients.SetChecked(false)
	w.b.fuelcutInEngineBrake.SetChecked(false)
	w.b.constantInjectionTime.SetChecked(false)
	w.b.accelEnrich.SetChecked(false)
	w.b.decelEnrich.SetChecked(false)
	w.b.bil104.SetChecked(false)
	w.b.idleFuelAdapt.SetChecked(false)

	w.b.lambdaPuff.SetChecked(false)
	w.b.useIdleInjectionMap.SetChecked(false)
	w.b.correctionForEngagingAC.SetChecked(false)
	w.b.amosOn.SetChecked(false)
	w.b.fuelAdjustmentDuringIdle.SetChecked(false)
	w.b.purgeCtrl.SetChecked(false)
	w.b.adaptionOfIdleCtrl.SetChecked(false)
	w.b.lambdaCtrlDuringIdle.SetChecked(false)

	w.b.heatplates.SetChecked(false)
	w.b.automaticTransmission.SetChecked(false)
	w.b.loadControl.SetChecked(false)
	w.b.etsTcs.SetChecked(false)
	w.b.boostCtrl.SetChecked(false)
	w.b.higherIdleDuringStart.SetChecked(false)
	w.b.globalAdaptation.SetChecked(false)
	w.b.tempCorrClosedLoop.SetChecked(false)

	w.b.loadBufferingDuringIdle.SetChecked(false)
	w.b.fixedIdleign.SetChecked(false)
	w.b.noFuelFutInR12.SetChecked(false)
	w.b.airpumpCtrl.SetChecked(false)
	w.b.naengine.SetChecked(false)
	w.b.knkdetOff.SetChecked(false)
	w.b.constantAngle.SetChecked(false)
	w.b.purgeValveMY94.SetChecked(false)

	w.b.tankPressureDiagnostics.SetChecked(false)
	w.b.vssEnabled.SetChecked(false)
}

func (w *Widget) Set(data []byte) {
	w.data = data
	if len(data) < 6 {
		w.disableVSSOptions()
	}
	if len(data) < 5 {
		w.disableAdvancedControls()
	}

	w.clearChecks()

	if data[0]&0x01 > 0 {
		w.b.enrichAfterStart.SetChecked(true)
	}
	if data[0]&0x02 > 0 {
		w.b.wotEnrich.SetChecked(true)
	}
	if data[0]&0x04 > 0 {
		w.b.ipolWait.SetChecked(true)
	}
	if data[0]&0x08 > 0 {
		w.b.tempCorr.SetChecked(true)
	}
	if data[0]&0x10 > 0 {
		w.b.lambdaCtrl.SetChecked(true)
	}
	if data[0]&0x20 > 0 {
		w.b.adaptivity.SetChecked(true)
	}
	if data[0]&0x40 > 0 {
		w.b.idleCtrl.SetChecked(true)
	}
	if data[0]&0x80 > 0 {
		w.b.enrichDuringStart.SetChecked(true)
	}

	if data[1]&0x01 > 0 {
		w.b.constantInjectionTime.SetChecked(true)
	}
	if data[1]&0x02 > 0 {
		w.b.lambdaCtrlOnTransients.SetChecked(true)
	}
	if data[1]&0x04 > 0 {
		w.b.fuelcutInEngineBrake.SetChecked(true)
	}
	if data[1]&0x08 > 0 {
		w.b.constantInjectionTime.SetChecked(true)
	}
	if data[1]&0x10 > 0 {
		w.b.accelEnrich.SetChecked(true)
	}
	if data[1]&0x20 > 0 {
		w.b.decelEnrich.SetChecked(true)
	}
	if data[1]&0x40 > 0 {
		w.b.bil104.SetChecked(true)
	}
	if data[1]&0x80 > 0 {
		w.b.idleFuelAdapt.SetChecked(true)
	}

	if data[2]&0x01 > 0 {
		w.b.lambdaPuff.SetChecked(true)
	}
	if data[2]&0x02 > 0 {
		w.b.useIdleInjectionMap.SetChecked(true)
	}
	if data[2]&0x04 > 0 {
		w.b.correctionForEngagingAC.SetChecked(true)
	}
	if data[2]&0x08 > 0 {
		w.b.amosOn.SetChecked(true)
	}
	if data[2]&0x10 > 0 {
		w.b.fuelAdjustmentDuringIdle.SetChecked(true)
	}
	if data[2]&0x20 > 0 {
		w.b.purgeCtrl.SetChecked(true)
	}
	if data[2]&0x40 > 0 {
		w.b.adaptionOfIdleCtrl.SetChecked(true)
	}
	if data[2]&0x80 > 0 {
		w.b.lambdaCtrlDuringIdle.SetChecked(true)
	}

	if data[3]&0x01 > 0 {
		w.b.heatplates.SetChecked(true)
	}
	if data[3]&0x02 > 0 {
		w.b.automaticTransmission.SetChecked(true)
	}
	if data[3]&0x04 > 0 {
		w.b.loadControl.SetChecked(true)
	}
	if data[3]&0x08 > 0 {
		w.b.etsTcs.SetChecked(true)
	}
	if data[3]&0x10 > 0 {
		w.b.boostCtrl.SetChecked(true)
	}
	if data[3]&0x20 > 0 {
		w.b.higherIdleDuringStart.SetChecked(true)
	}
	if data[3]&0x40 > 0 {
		w.b.globalAdaptation.SetChecked(true)
	}
	if data[3]&0x80 > 0 {
		w.b.tempCorrClosedLoop.SetChecked(true) //fuelAdjustmentDuringIdle
	}

	if len(data) > 4 {
		if data[4]&0x01 > 0 {
			w.b.loadBufferingDuringIdle.SetChecked(true)
		}
		if data[4]&0x02 > 0 {
			w.b.fixedIdleign.SetChecked(true)
		}
		if data[4]&0x04 > 0 {
			w.b.noFuelFutInR12.SetChecked(true)
		}
		if data[4]&0x08 > 0 {
			w.b.airpumpCtrl.SetChecked(true)
		}
		if data[4]&0x10 > 0 {
			w.b.naengine.SetChecked(true)
		}
		if data[4]&0x20 > 0 {
			w.b.knkdetOff.SetChecked(true)
		}
		if data[4]&0x40 > 0 {
			w.b.constantAngle.SetChecked(true)
		}
		if data[4]&0x80 > 0 {
			w.b.purgeValveMY94.SetChecked(true)
		}
	}

	if len(data) > 5 {
		if data[5]&0x10 > 0 {
			w.b.tankPressureDiagnostics.SetChecked(true)
		}
		if data[5]&0x80 > 0 {
			w.b.vssEnabled.SetChecked(true)
		}
	}
}

func (w *Widget) Get() []byte {
	w.data = make([]byte, len(w.data))

	if w.b.enrichAfterStart.Checked {
		w.data[0] |= 0x01
	}
	if w.b.wotEnrich.Checked {
		w.data[0] |= 0x02
	}
	if w.b.ipolWait.Checked {
		w.data[0] |= 0x04
	}
	if w.b.tempCorr.Checked {
		w.data[0] |= 0x08
	}
	if w.b.lambdaCtrl.Checked {
		w.data[0] |= 0x10
	}
	if w.b.adaptivity.Checked {
		w.data[0] |= 0x20
	}
	if w.b.idleCtrl.Checked {
		w.data[0] |= 0x40
	}
	if w.b.enrichDuringStart.Checked {
		w.data[0] |= 0x80
	}

	if w.b.constantInjection.Checked {
		w.data[1] |= 0x01
	}
	if w.b.lambdaCtrlOnTransients.Checked {
		w.data[1] |= 0x02
	}
	if w.b.fuelcutInEngineBrake.Checked {
		w.data[1] |= 0x04
	}
	if w.b.constantInjectionTime.Checked {
		w.data[1] |= 0x08
	}
	if w.b.accelEnrich.Checked {
		w.data[1] |= 0x10
	}
	if w.b.decelEnrich.Checked {
		w.data[1] |= 0x20
	}
	if w.b.bil104.Checked {
		w.data[1] |= 0x40
	}
	if w.b.idleFuelAdapt.Checked {
		w.data[1] |= 0x80
	}

	if w.b.lambdaPuff.Checked {
		w.data[2] |= 0x01
	}
	if w.b.useIdleInjectionMap.Checked {
		w.data[2] |= 0x02
	}
	if w.b.correctionForEngagingAC.Checked {
		w.data[2] |= 0x04
	}
	if w.b.amosOn.Checked {
		w.data[2] |= 0x08
	}
	if w.b.fuelAdjustmentDuringIdle.Checked {
		w.data[2] |= 0x10
	}
	if w.b.purgeCtrl.Checked {
		w.data[2] |= 0x20
	}
	if w.b.adaptionOfIdleCtrl.Checked {
		w.data[2] |= 0x40
	}
	if w.b.lambdaCtrlDuringIdle.Checked {
		w.data[2] |= 0x80
	}

	if w.b.heatplates.Checked {
		w.data[3] |= 0x01
	}
	if w.b.automaticTransmission.Checked {
		w.data[3] |= 0x02
	}
	if w.b.loadControl.Checked {
		w.data[3] |= 0x04
	}
	if w.b.etsTcs.Checked {
		w.data[3] |= 0x08
	}
	if w.b.boostCtrl.Checked {
		w.data[3] |= 0x10
	}
	if w.b.higherIdleDuringStart.Checked {
		w.data[3] |= 0x20
	}
	if w.b.globalAdaptation.Checked {
		w.data[3] |= 0x40
	}
	if w.b.tempCorrClosedLoop.Checked {
		w.data[3] |= 0x80
	}

	if len(w.data) > 4 {
		if w.b.loadBufferingDuringIdle.Checked {
			w.data[4] |= 0x01
		}
		if w.b.fixedIdleign.Checked {
			w.data[4] |= 0x02
		}
		if w.b.noFuelFutInR12.Checked {
			w.data[4] |= 0x04
		}
		if w.b.airpumpCtrl.Checked {
			w.data[4] |= 0x08
		}
		if w.b.naengine.Checked {
			w.data[4] |= 0x10
		}
		if w.b.knkdetOff.Checked {
			w.data[4] |= 0x20
		}
		if w.b.constantAngle.Checked {
			w.data[4] |= 0x40
		}
		if w.b.purgeValveMY94.Checked {
			w.data[4] |= 0x80
		}
	}

	if len(w.data) > 5 {
		if w.b.tankPressureDiagnostics.Checked {
			w.data[5] |= 0x10
		}
		if w.b.vssEnabled.Checked {
			w.data[5] |= 0x80
		}
	}

	out := make([]byte, len(w.data))
	copy(out, w.data)

	return out
}

func (w *Widget) disableVSSOptions() {
	w.b.tankPressureDiagnostics.Disable()
	w.b.vssEnabled.Disable()
}

func (w *Widget) disableAdvancedControls() {
	w.b.loadBufferingDuringIdle.Disable()
	w.b.fixedIdleign.Disable()
	w.b.noFuelFutInR12.Disable()
	w.b.airpumpCtrl.Disable()
	w.b.naengine.Disable()
	w.b.knkdetOff.Disable()
	w.b.constantAngle.Disable()
	w.b.purgeValveMY94.Disable()
}

type renderer struct {
	w *Widget
}

func (r *renderer) Layout(size fyne.Size) {
	r.w.content.Resize(size)
}

func (r *renderer) MinSize() fyne.Size {
	return r.w.content.MinSize()
}

func (r *renderer) Refresh() {
	r.w.content.Refresh()
}

func (r *renderer) Destroy() {
}

func (r *renderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.w.content}
}
