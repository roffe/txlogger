package pgmstatus

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/widgets/ledicon"
)

type Widget struct {
	widget.BaseWidget

	leds leds

	content *fyne.Container
}

type leds struct {
	ignition   *ledicon.Widget // Ignition
	applSyncOk *ledicon.Widget // Appl. sync ok
	fuelNotOff *ledicon.Widget // Fuel not off (during sync. off ign)

	afterStart2ok      *ledicon.Widget // Afterstart 2 ok
	fuelKnockMap       *ledicon.Widget // Fuel knock map
	decEnleanCompleted *ledicon.Widget // Dec.enleanment completed throttledec

	engineStopped      *ledicon.Widget // Engine stopped
	throttleClosed     *ledicon.Widget // Throttle closed
	accEnrichCompleted *ledicon.Widget // Acc. enrichment completed throttleinc

	engineStarted              *ledicon.Widget // Engine started
	roomTempStart              *ledicon.Widget // Room temp. start
	decreaseRetardEnrchAllowed *ledicon.Widget // Decrease of retard enrichment allowed

	engineWarm                    *ledicon.Widget // Engine is warm
	ignitionSynchronized          *ledicon.Widget // Ignition synchronized
	startRetardEnrichmentProgress *ledicon.Widget // Start of retard enrichment in progress

	fuelCut               *ledicon.Widget // Fuel cut
	sondHeatingSecondSond *ledicon.Widget // Sond heating, second sond
	adaptionAllowed       *ledicon.Widget // Adaption allowed

	rpmLimiter           *ledicon.Widget // RPM limiter
	sondHeatingFirstSond *ledicon.Widget // Sond heating, first sond
	knockMap             *ledicon.Widget // Knock map (Not used )

	restart  *ledicon.Widget // Restart
	etsError *ledicon.Widget // ETS error
	limpHome *ledicon.Widget // Limp-home home

	purgeCtrlActive         *ledicon.Widget // Purge control active
	ordinaryIdleCtrlDisable *ledicon.Widget // Ordinary idle control disable
	alwaysActTmpComp        *ledicon.Widget // Always active temp.compensation

	idleFuelMap           *ledicon.Widget // Idle fuel map
	fuelCutAllowedDashpot *ledicon.Widget // Fuel cut allowd (Dashpot)
	afterStartEnrichCompl *ledicon.Widget // After start enrichmentcompleted

	fuelCutCyl1          *ledicon.Widget // Fuel cut cyl 1
	enrichAfterFuelcut   *ledicon.Widget // Enrich after fuel cut
	initDuringStartCompl *ledicon.Widget // Init during start completed

	fuelCutCyl2           *ledicon.Widget // Fuel cut cyl 2
	fullLoadEnrich        *ledicon.Widget // Full load enrichment
	cooligWaterEnrichFnsh *ledicon.Widget // Cooling water enrichment finished

	fuelCutCyl3      *ledicon.Widget // Fuel cut cyl 3
	fuelSynch        *ledicon.Widget // Fuel syncronized
	activeLambdaCtrl *ledicon.Widget // Active lambda control

	fuelCutCyl4 *ledicon.Widget // Fuel cut cyl 4
	tempCmp     *ledicon.Widget // Temp. compensation
}

func New() *Widget {
	w := &Widget{
		leds: leds{
			ignition:   ledicon.New("Ignition"),
			applSyncOk: ledicon.New("Appl. sync ok"),
			fuelNotOff: ledicon.New("Fuel not off (during sync. off ign)"),

			afterStart2ok:      ledicon.New("Afterstart 2 ok"),
			fuelKnockMap:       ledicon.New("Fuel knock map"),
			decEnleanCompleted: ledicon.New("Dec.enleanment completed throttledec"),

			engineStopped:      ledicon.New("Engine stopped"),
			throttleClosed:     ledicon.New("Throttle closed"),
			accEnrichCompleted: ledicon.New("Acc. enrichment completed throttleinc"),

			engineStarted:              ledicon.New("Engine started"),
			roomTempStart:              ledicon.New("Room temp. start"),
			decreaseRetardEnrchAllowed: ledicon.New("Decrease of retard enrichment allowed"),

			engineWarm:                    ledicon.New("Engine is warm"),
			ignitionSynchronized:          ledicon.New("Ignition synchronized"),
			startRetardEnrichmentProgress: ledicon.New("Start of retard enrichment in progress"),

			fuelCut:               ledicon.New("Fuel cut"),
			sondHeatingSecondSond: ledicon.New("Sond heating, second sond"),
			adaptionAllowed:       ledicon.New("Adaption allowed"),

			rpmLimiter:           ledicon.New("RPM limiter"),
			sondHeatingFirstSond: ledicon.New("Sond heating, first sond"),
			knockMap:             ledicon.New("Knock map"),

			restart:  ledicon.New("Restart"),
			etsError: ledicon.New("ETS error"),
			limpHome: ledicon.New("Limp-home home"),

			purgeCtrlActive:         ledicon.New("Purge control active"),
			ordinaryIdleCtrlDisable: ledicon.New("Ordinary idle control disable"),
			alwaysActTmpComp:        ledicon.New("Always active temp.compensation"),

			idleFuelMap:           ledicon.New("Idle fuel map"),
			fuelCutAllowedDashpot: ledicon.New("Fuel cut allowd (Dashpot)"),
			afterStartEnrichCompl: ledicon.New("After start enrichmentcompleted"),

			fuelCutCyl1:          ledicon.New("Fuel cut cyl 1"),
			enrichAfterFuelcut:   ledicon.New("Enrich after fuel cut"),
			initDuringStartCompl: ledicon.New("Init during start completed"),

			fuelCutCyl2:           ledicon.New("Fuel cut cyl 2"),
			fullLoadEnrich:        ledicon.New("Full load enrichment"),
			cooligWaterEnrichFnsh: ledicon.New("Cooling water enrichment finished"),

			fuelCutCyl3:      ledicon.New("Fuel cut cyl 3"),
			fuelSynch:        ledicon.New("Fuel syncronized"),
			activeLambdaCtrl: ledicon.New("Active lambda control"),

			fuelCutCyl4: ledicon.New("Fuel cut cyl 4"),
			tempCmp:     ledicon.New("Temp. compensation"),
		},
	}
	w.ExtendBaseWidget(w)

	w.content = container.NewAdaptiveGrid(3,
		w.leds.ignition, w.leds.applSyncOk, w.leds.fuelNotOff,
		w.leds.afterStart2ok, w.leds.fuelKnockMap, w.leds.decEnleanCompleted,
		w.leds.engineStopped, w.leds.throttleClosed, w.leds.accEnrichCompleted,
		w.leds.engineStarted, w.leds.roomTempStart, w.leds.decreaseRetardEnrchAllowed,
		w.leds.engineWarm, w.leds.ignitionSynchronized, w.leds.startRetardEnrichmentProgress,
		w.leds.fuelCut, w.leds.sondHeatingSecondSond, w.leds.adaptionAllowed,
		w.leds.rpmLimiter, w.leds.sondHeatingFirstSond, w.leds.knockMap,
		w.leds.restart, w.leds.etsError, w.leds.limpHome,
		w.leds.purgeCtrlActive, w.leds.ordinaryIdleCtrlDisable, w.leds.alwaysActTmpComp,
		w.leds.idleFuelMap, w.leds.fuelCutAllowedDashpot, w.leds.afterStartEnrichCompl,
		w.leds.fuelCutCyl1, w.leds.enrichAfterFuelcut, w.leds.initDuringStartCompl,
		w.leds.fuelCutCyl2, w.leds.fullLoadEnrich, w.leds.cooligWaterEnrichFnsh,
		w.leds.fuelCutCyl3, w.leds.fuelSynch, w.leds.activeLambdaCtrl,
		w.leds.fuelCutCyl4, w.leds.tempCmp,
	)

	return w
}

func (w *Widget) clearAll() {
	w.leds.ignition.Off()
	w.leds.applSyncOk.Off()
	w.leds.fuelNotOff.Off()
	w.leds.afterStart2ok.Off()
	w.leds.fuelKnockMap.Off()
	w.leds.decEnleanCompleted.Off()
	w.leds.engineStopped.Off()
	w.leds.throttleClosed.Off()
	w.leds.accEnrichCompleted.Off()
	w.leds.engineStarted.Off()
	w.leds.roomTempStart.Off()
	w.leds.decreaseRetardEnrchAllowed.Off()
	w.leds.engineWarm.Off()
	w.leds.ignitionSynchronized.Off()
	w.leds.startRetardEnrichmentProgress.Off()
	w.leds.fuelCut.Off()
	w.leds.sondHeatingSecondSond.Off()
	w.leds.adaptionAllowed.Off()
	w.leds.rpmLimiter.Off()
	w.leds.sondHeatingFirstSond.Off()
	w.leds.knockMap.Off()
	w.leds.restart.Off()
	w.leds.etsError.Off()
	w.leds.limpHome.Off()
	w.leds.purgeCtrlActive.Off()
	w.leds.ordinaryIdleCtrlDisable.Off()
	w.leds.alwaysActTmpComp.Off()
	w.leds.idleFuelMap.Off()
	w.leds.fuelCutAllowedDashpot.Off()
	w.leds.afterStartEnrichCompl.Off()
	w.leds.fuelCutCyl1.Off()
	w.leds.enrichAfterFuelcut.Off()
	w.leds.initDuringStartCompl.Off()
	w.leds.fuelCutCyl2.Off()
	w.leds.fullLoadEnrich.Off()
	w.leds.cooligWaterEnrichFnsh.Off()
	w.leds.fuelCutCyl3.Off()
	w.leds.fuelSynch.Off()
	w.leds.activeLambdaCtrl.Off()
	w.leds.fuelCutCyl4.Off()
	w.leds.tempCmp.Off()
}

func (w *Widget) Set(data float64) {
	value := uint64(data)
	w.clearAll()
	if value&0x01 > 0 {
		w.leds.ignition.On()
	} else {
		w.leds.ignition.Off()
	}
	if value&0x02 > 0 {
		w.leds.afterStart2ok.On()
	}
	if value&0x04 > 0 {
		w.leds.engineStopped.On()
	}
	if value&0x08 > 0 {
		w.leds.engineStarted.On()
	}
	if value&0x10 > 0 {
		w.leds.engineWarm.On()
	}
	if value&0x20 > 0 {
		w.leds.fuelCut.On()
	}
	if value&0x40 > 0 {
		w.leds.tempCmp.On()
	}
	if value&0x80 > 0 {
		w.leds.rpmLimiter.On()
	}
	if value&0x100 > 0 {
		w.leds.applSyncOk.On()
	}
	if value&0x200 > 0 {
		w.leds.fuelKnockMap.On()
	}
	if value&0x400 > 0 {
		w.leds.throttleClosed.On()
	}
	if value&0x800 > 0 {
		w.leds.roomTempStart.On()
	}
	if value&0x1000 > 0 {
		w.leds.fuelCutCyl4.On()
	}
	if value&0x2000 > 0 {
		w.leds.fuelCutCyl3.On()
	}
	if value&0x4000 > 0 {
		w.leds.fuelCutCyl2.On()
	}
	if value&0x8000 > 0 {
		w.leds.fuelCutCyl1.On()
	}
	if value&0x10000 > 0 {
		w.leds.fuelNotOff.On()
	}
	if value&0x20000 > 0 {
		w.leds.decEnleanCompleted.On()
	}
	if value&0x40000 > 0 {
		w.leds.accEnrichCompleted.On()
	}
	if value&0x80000 > 0 {
		w.leds.decreaseRetardEnrchAllowed.On()
	}
	if value&0x100000 > 0 {
		w.leds.startRetardEnrichmentProgress.On()
	}
	if value&0x200000 > 0 {
		w.leds.adaptionAllowed.On()
	}
	if value&0x400000 > 0 {
		w.leds.limpHome.On()
	}
	if value&0x800000 > 0 {
		w.leds.alwaysActTmpComp.On()
	}
	if value&0x1000000 > 0 {
		w.leds.restart.On()
	}
	if value&0x2000000 > 0 {
		w.leds.activeLambdaCtrl.On()
	}
	if value&0x4000000 > 0 {
		w.leds.afterStartEnrichCompl.On()
	}
	if value&0x8000000 > 0 {
		w.leds.initDuringStartCompl.On()
	}
	if value&0x10000000 > 0 {
		w.leds.cooligWaterEnrichFnsh.On()
	}
	if value&0x20000000 > 0 {
		w.leds.purgeCtrlActive.On()
	}
	if value&0x40000000 > 0 {
		w.leds.idleFuelMap.On()
	}
	if value&0x80000000 > 0 {
		w.leds.ignitionSynchronized.On()
	}
	if value&0x100000000 > 0 {
		w.leds.sondHeatingSecondSond.On()
	}
	if value&0x200000000 > 0 {
		w.leds.sondHeatingFirstSond.On()
	}
	if value&0x400000000 > 0 {
		w.leds.etsError.On()
	}
	if value&0x800000000 > 0 {
		w.leds.ordinaryIdleCtrlDisable.On()
	}
	if value&0x1000000000 > 0 {
		w.leds.fuelCutAllowedDashpot.On()
	}
	if value&0x2000000000 > 0 {
		w.leds.enrichAfterFuelcut.On()
	}
	if value&0x4000000000 > 0 {
		w.leds.fullLoadEnrich.On()
	}
	if value&0x8000000000 > 0 {
		w.leds.fuelSynch.On()
	}

}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.content)
}
