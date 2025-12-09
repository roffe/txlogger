package dashboard

import (
	"fmt"

	"fyne.io/fyne/v2/canvas"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
)

func (db *Dashboard) createRouter() map[string]func(float64) {
	var rpm float64
	t5rpmSetter := func(value float64) {
		rpm = value
		db.gauges.rpm.SetValue(value)
	}

	idcSetterT5 := func(obj *canvas.Text, text string) func(float64) {
		idcc := idcSetter(obj, text)
		return func(value float64) {
			idcc((value * rpm) * rpmIDCconstant)
		}
	}

	ioff := ioffSetter(db.text.ioff, db.image.taz)

	// activeAirDem := db.activeAirDemSetter(db.text.activeAirDem)

	setVehicleSpeed := db.gauges.speed.SetValue
	if db.cfg.UseMPH {
		setVehicleSpeed = func(value float64) {
			db.gauges.speed.SetValue(value * 0.621371)
		}
	}

	t5throttle := func(value float64) {
		// value should be 0-100% input is 0 - 192
		valuePercent := min(192, value) / 192 * 100
		db.gauges.throttle.SetValue(valuePercent)
	}

	t5setnbl := func(value float64) {
		if value < 128 {
			// Interpolate in the range 0 to 128, mapping to -25 to 0.
			db.gauges.nblambda.SetValue(interpol(0, -25, 128, 0, value))
			return
		}
		// Interpolate in the range 128 to 255, mapping to 0 to 25.
		db.gauges.nblambda.SetValue(interpol(128, 0, 255, 25, value))
	}

	router := map[string]func(float64){
		"In.v_Vehicle": setVehicleSpeed, // t7 & t8
		"Bil_hast":     setVehicleSpeed, // t5

		"ActualIn.n_Engine": db.gauges.rpm.SetValue,
		"Rpm":               t5rpmSetter, // t5

		"ActualIn.T_AirInlet": db.gauges.iat.SetValue,
		"Lufttemp":            db.gauges.iat.SetValue, // t5

		"ActualIn.T_Engine": db.gauges.engineTemp.SetValue,
		"Kyl_temp":          db.gauges.engineTemp.SetValue, // t5

		"P_medel":             db.gauges.pressure.SetValue, // t5
		"In.p_AirInlet":       db.gauges.pressure.SetValue,
		"ActualIn.p_AirInlet": db.gauges.pressure.SetValue,

		"Max_tryck":                 db.gauges.pressure.SetValue2, // t5
		"In.p_AirBefThrottle":       db.gauges.pressure.SetValue2,
		"ActualIn.p_AirBefThrottle": db.gauges.pressure.SetValue2,

		"Medeltrot":      t5throttle,                  // t5
		"Out.X_AccPedal": db.gauges.throttle.SetValue, // t7
		"Out.X_AccPos":   db.gauges.throttle.SetValue, // t8

		"Out.PWM_BoostCntrl": db.gauges.pwm.SetValue, // t7 & t8
		"PWM_ut10":           db.gauges.pwm.SetValue, // t5

		//"AdpFuelProt.MulFuelAdapt": amulSetter(db.text.amul, "Amul"), // t7
		"AdpFuelProt.MulFuelAdapt": textSetter(db.text.amul, "Amul", "%", 2), // t7

		// Wideband lambda
		//"AD_EGR": db.gauges.wblambda.SetValue, // t5
		//"DisplProt.LambdaScanner": db.wblambda.SetValue, // t7 & t8
		//"Lambda.External":     db.wblambda.SetValue,
		db.cfg.WidebandSymbol: db.gauges.wblambda.SetValue, // Wideband lambda

		// NB lambda
		"Lambda.LambdaInt": db.gauges.nblambda.SetValue, // t7 & t8
		"Lambdaint":        t5setnbl,                    // t5

		"MAF.m_AirInlet":        db.gauges.airmass.SetValue,  // t7 & t8
		"m_Request":             db.gauges.airmass.SetValue2, // t7
		"AirMassMast.m_Request": db.gauges.airmass.SetValue2, // t8

		"Out.fi_Ignition": textSetter(db.text.ign, "Ign", "", 1),
		"Ign_angle":       textSetter(db.text.ign, "Ign", "", 1),

		"ECMStat.ST_ActiveAirDem": db.activeAirSetter(db.text.activeAirDem), // t7 & t8

		"IgnProt.fi_Offset":     ioff, // t7
		"IgnMastProt.fi_Offset": ioff, // t8

		"CRUISE": showHider(db.text.cruise),
		"CEL":    showHider(db.image.checkEngine),
		"LIMP":   showHider(db.image.limpMode),

		"Knock_offset1234": knkDetSetter(db.image.knockIcon),
		"KnkDet.KnockCyl":  knkDetSetter(db.image.knockIcon),

		"Myrtilos.InjectorDutyCycle": idcSetter(db.text.idc, "Idc"),   // t7
		"Insptid_ms10":               idcSetterT5(db.text.idc, "Idc"), // t5

		ebus.TOPIC_ECU: func(value float64) {
			switch symbol.ECUType(int(value)) {
			case symbol.ECU_T5: //T5
				db.cfg.AirDemToString = func(f float64) string {
					return fmt.Sprintf("%.0f", f)
				}
			case symbol.ECU_T7: //T7
				db.cfg.AirDemToString = datalogger.AirDemToStringT7
			case symbol.ECU_T8: //T8
				db.cfg.AirDemToString = datalogger.AirDemToStringT8
			}
		},
	}

	return router
}
