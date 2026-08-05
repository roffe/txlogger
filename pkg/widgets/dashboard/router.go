package dashboard

import (
	"fmt"

	"fyne.io/fyne/v2/canvas"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
)

func (db *Dashboard) createRouter() map[string]func(float64) {
	// Gauges build their canvas objects in CreateRenderer, so every gauge
	// setter goes through db.gate: a gauge the layout doesn't place is never
	// fed. Text and image elements need no gating.
	setRPM := db.gate("rpm", db.gauges.rpm.SetValue)
	setIAT := db.gate("iat", db.gauges.iat.SetValue)
	setEngineTemp := db.gate("engineTemp", db.gauges.engineTemp.SetValue)
	setAirmass := db.gate("airmass", db.gauges.airmass.SetValue)
	setAirmassReq := db.gate("airmass", db.gauges.airmass.SetValue2)
	setPressure := db.gate("pressure", db.gauges.pressure.SetValue)
	setPressureBefThrottle := db.gate("pressure", db.gauges.pressure.SetValue2)
	setThrottle := db.gate("throttle", db.gauges.throttle.SetValue)
	setPWM := db.gate("pwm", db.gauges.pwm.SetValue)
	setNBLambda := db.gate("nblambda", db.gauges.nblambda.SetValue)

	var rpm float64
	t5rpmSetter := func(value float64) {
		rpm = value
		setRPM(value)
	}

	idcSetterT5 := func(obj *canvas.Text, text string) func(float64) {
		idcSetter := idcSetter(obj, text)
		return func(value float64) {
			idcSetter((value * rpm) * rpmIDCconstant)
		}
	}

	ioff := ioffSetter(db.text.ioff, db.image.taz)

	// activeAirDem := db.activeAirDemSetter(db.text.activeAirDem)

	setVehicleSpeed := db.gate("speed", db.gauges.speed.SetValue)
	if db.cfg.UseMPH {
		setSpeed := setVehicleSpeed
		setVehicleSpeed = func(value float64) {
			setSpeed(value * 0.621371)
		}
	}

	t5throttle := func(value float64) {
		// value should be 0-100% input is 0 - 192
		valuePercent := min(192, value) / 192 * 100
		setThrottle(valuePercent)
	}

	t5setnbl := func(value float64) {
		if value < 128 {
			// Interpolate in the range 0 to 128, mapping to -25 to 0.
			setNBLambda(interpol(0, -25, 128, 0, value))
			return
		}
		// Interpolate in the range 128 to 255, mapping to 0 to 25.
		setNBLambda(interpol(128, 0, 255, 25, value))
	}

	router := map[string]func(float64){
		"In.v_Vehicle": setVehicleSpeed, // t7 & t8
		"Bil_hast":     setVehicleSpeed, // t5

		"ActualIn.n_Engine": setRPM,
		"Rpm":               t5rpmSetter, // t5

		"ActualIn.T_AirInlet": setIAT,
		"Lufttemp":            setIAT, // t5

		"ActualIn.T_Engine": setEngineTemp,
		"Kyl_temp":          setEngineTemp, // t5

		"P_medel":             setPressure, // t5
		"In.p_AirInlet":       setPressure,
		"ActualIn.p_AirInlet": setPressure,

		"Max_tryck":                 setPressureBefThrottle, // t5
		"In.p_AirBefThrottle":       setPressureBefThrottle,
		"ActualIn.p_AirBefThrottle": setPressureBefThrottle,

		"Medeltrot":      t5throttle,  // t5
		"Out.X_AccPedal": setThrottle, // t7
		"Out.X_AccPos":   setThrottle, // t8

		"Out.PWM_BoostCntrl": setPWM, // t7 & t8
		"PWM_ut10":           setPWM, // t5

		//"AdpFuelProt.MulFuelAdapt": amulSetter(db.text.amul, "Amul"), // t7
		"AdpFuelProt.MulFuelAdapt": textSetter(db.text.amul, "Amul", "%", 2), // t7

		// Wideband lambda
		//"AD_EGR": db.gauges.wblambda.SetValue, // t5
		//"DisplProt.LambdaScanner": db.wblambda.SetValue, // t7 & t8
		//"Lambda.External":     db.wblambda.SetValue,
		db.cfg.WidebandSymbol: db.setWBLambda, // Wideband lambda

		// NB lambda
		"Lambda.LambdaInt": setNBLambda, // t7 & t8
		"Lambdaint":        t5setnbl,    // t5

		"MAF.m_AirInlet":        setAirmass,    // t7 & t8
		"m_Request":             setAirmassReq, // t7
		"AirMassMast.m_Request": setAirmassReq, // t8

		"Out.fi_Ignition": textSetter(db.text.ign, "Ign", "", 1),
		"Ign_angle":       textSetter(db.text.ign, "Ign", "", 1),

		"ECMStat.ST_ActiveAirDem": db.activeAirSetter(db.text.activeAirDem), // t7 & t8

		"IgnProt.fi_Offset":     ioff, // t7
		"IgnMastProt.fi_Offset": ioff, // t8

		"CRUISE": showHider(db.text.cruise),
		"CEL":    showHider(db.image.checkEngine),
		"LIMP":   showHider(db.image.limpMode),

		"Knock_offset1234": knkDetSetter(db.image.knockIcon),
		"KnkDet.KnockCyl":  knkDetSetter(db.image.knockIcon), // t7 & t8

		//"IgnKnk.fi_Offset": knkIoffSetter(, db.image.knockIcon), // t7

		"Myrtilos.InjectorDutyCycle": idcSetter(db.text.idc, "Idc"),   // t7
		"Insptid_ms10":               idcSetterT5(db.text.idc, "Idc"), // t5

		ebus.TOPIC_ECU: func(value float64) {
			switch symbol.ECUType(int(value)) {
			case symbol.ECU_T5: // T5
				db.cfg.AirDemToString = func(f float64) string {
					return fmt.Sprintf("%.0f", f)
				}
			case symbol.ECU_T7: // T7
				db.cfg.AirDemToString = datalogger.AirDemToStringT7
			case symbol.ECU_T8: // T8
				db.cfg.AirDemToString = datalogger.AirDemToStringT8
			}
		},
	}

	return router
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
