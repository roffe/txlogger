package dashboard

import (
	"github.com/roffe/txlogger/pkg/ecu"
)

func (db *Dashboard) createRouter() map[string]func(float64) {
	// Every route runs under db.mu (see SetValue), which guards the state the
	// closures below capture, cfg.AirDemToString included. Gauge setters are
	// goroutine-safe and go through db.gate: a gauge the layout doesn't place
	// is never fed. Text and image elements need no gating; their setters
	// post only the canvas mutation with fyne.Do.
	setRPM := db.gate("rpm", db.gauges.rpm.SetValue)
	setVehicleSpeed := db.gate("speed", db.gauges.speed.SetValue)
	if db.cfg.UseMPH {
		setSpeed := setVehicleSpeed
		setVehicleSpeed = func(value float64) {
			setSpeed(value * 0.621371)
		}
	}
	setIDC := idcSetter(db.text.idc, "Idc")
	var rpm float64 // latest, turns injection time into duty cycle

	// The dashboard is fed by signal, each ECU's profile names the symbol
	// carrying it; see ecu.Signal.
	bySignal := map[ecu.Signal]func(float64){
		ecu.RPM: func(value float64) {
			rpm = value
			setRPM(value)
		},
		ecu.Speed:        setVehicleSpeed,
		ecu.IAT:          db.gate("iat", db.gauges.iat.SetValue),
		ecu.Coolant:      db.gate("engineTemp", db.gauges.engineTemp.SetValue),
		ecu.MAP:          db.gate("pressure", db.gauges.pressure.SetValue),
		ecu.PreThrottle:  db.gate("pressure", db.gauges.pressure.SetValue2),
		ecu.Throttle:     db.gate("throttle", db.gauges.throttle.SetValue),
		ecu.BoostPWM:     db.gate("pwm", db.gauges.pwm.SetValue),
		ecu.NBLambda:     db.gate("nblambda", db.gauges.nblambda.SetValue),
		ecu.Airmass:      db.gate("airmass", db.gauges.airmass.SetValue),
		ecu.AirmassReq:   db.gate("airmass", db.gauges.airmass.SetValue2),
		ecu.Ignition:     textSetter(db.text.ign, "Ign", "", 1),
		ecu.IgnOffset:    ioffSetter(db.text.ioff, db.image.taz),
		ecu.Knock:        knkDetSetter(db.image.knockIcon),
		ecu.ActiveAirDem: db.activeAirSetter(db.text.activeAirDem),
		ecu.FuelAdapt:    textSetter(db.text.amul, "Amul", "%", 2),
		ecu.InjDuty:      setIDC,
		ecu.InjTime: func(value float64) {
			setIDC(value * rpm * rpmIDCconstant)
		},
	}

	router := map[string]func(float64){
		"CRUISE": showHider(db.text.cruise),
		"CEL":    showHider(db.image.checkEngine),
		"LIMP":   showHider(db.image.limpMode),
	}
	for _, p := range ecu.Profiles() {
		for sig, sym := range p.Signals {
			set, ok := bySignal[sig]
			if !ok {
				continue
			}
			if scale := p.Scale[sym]; scale != nil {
				set = scaled(set, scale)
			}
			router[sym] = set
		}
	}
	router[db.cfg.WidebandSymbol] = db.setWBLambda
	return router
}

func scaled(set func(float64), scale func(float64) float64) func(float64) {
	return func(value float64) { set(scale(value)) }
}

/*
func knkIoffSetter(obj *canvas.Text) func(float64) {
	return func(value float64) {
		cyl1 := int16(value) >> 48
		cyl2 := int16(value>>32) & 0xFFFF
		cyl3 := int16(value>>16) & 0xFFFF
		cyl4 := int16(value) & 0xFFFF
		obj.Text = fmt.Sprintf("Knk Ioff: %d %d %d %d", cyl1, cyl2, cyl3, cyl4)
		obj.Refresh()
	}
}
*/
