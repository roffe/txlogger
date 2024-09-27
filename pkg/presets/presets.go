package presets

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	symbol "github.com/roffe/ecusymbol"
)

var Map = map[string]string{
	// "LJR":          `[{"name":"ActualIn.T_Engine","method":2,"value":3469,"type":33,"length":2,"correctionfactor":1,"group":"Temperature"},{"name":"ActualIn.T_AirInlet","method":2,"value":3470,"type":33,"length":2,"correctionfactor":1,"group":"Temperature"},{"name":"IgnProt.fi_Offset","method":2,"value":3045,"type":33,"length":2,"correctionfactor":0.1,"group":"Ignition"},{"name":"Out.fi_Ignition","method":2,"value":3686,"type":33,"length":2,"correctionfactor":0.1,"group":"Ignition"},{"name":"Out.PWM_BoostCntrl","method":2,"value":3645,"type":33,"length":2,"correctionfactor":0.1,"group":"%"},{"name":"ActualIn.p_AirInlet","method":2,"value":3472,"type":33,"length":2,"correctionfactor":0.001,"group":"Boost"},{"name":"In.p_AirBefThrottle","method":2,"value":3395,"type":33,"length":2,"correctionfactor":0.001,"group":"Boost"},{"name":"MAF.m_AirInlet","method":2,"value":452,"type":32,"length":2,"correctionfactor":1,"group":"Air mg/c"},{"name":"m_Request","method":2,"value":59,"type":0,"length":2,"correctionfactor":1,"group":"Air mg/c"},{"name":"ECMStat.ST_ActiveAirDem","method":2,"value":3754,"type":36,"length":1,"correctionfactor":1,"group":"Limiters"}]`,
	// "JSH":          `[{"name":"ActualIn.T_Engine","method":2,"value":3469,"type":33,"length":2,"correctionfactor":1,"group":"Temperature"},{"name":"ActualIn.T_AirInlet","method":2,"value":3470,"type":33,"length":2,"correctionfactor":1,"group":"Temperature"},{"name":"IgnProt.fi_Offset","method":2,"value":3045,"type":33,"length":2,"correctionfactor":0.1,"group":"Ignition"},{"name":"Out.fi_Ignition","method":2,"value":3686,"type":33,"length":2,"correctionfactor":0.1,"group":"Ignition"},{"name":"Out.PWM_BoostCntrl","method":2,"value":3645,"type":33,"length":2,"correctionfactor":0.1,"group":"%"},{"name":"ActualIn.p_AirInlet","method":2,"value":3472,"type":33,"length":2,"correctionfactor":0.001,"group":"Bar"},{"name":"In.p_AirBefThrottle","method":2,"value":3395,"type":33,"length":2,"correctionfactor":0.001,"group":"Bar"},{"name":"ECMStat.p_Diff","method":2,"value":3759,"type":33,"length":2,"correctionfactor":0.001,"group":"Bar"},{"name":"MAF.m_AirInlet","method":2,"value":452,"type":32,"length":2,"correctionfactor":1,"group":"Air mg/c"},{"name":"m_Request","method":2,"value":59,"type":0,"length":2,"correctionfactor":1,"group":"Air mg/c"},{"name":"ECMStat.ST_ActiveAirDem","method":2,"value":3754,"type":36,"length":1,"correctionfactor":1,"group":"Limiters"}]`,
	//"T7 Dash": `[{"Name":"ActualIn.T_Engine","Number":3469,"SramOffset":0,"Address":15788916,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":1},{"Name":"ActualIn.T_AirInlet","Number":3470,"SramOffset":0,"Address":15788918,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":1},{"Name":"IgnProt.fi_Offset","Number":3045,"SramOffset":0,"Address":15787464,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"Degrees"},{"Name":"Out.fi_Ignition","Number":3686,"SramOffset":0,"Address":15789366,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"° BTDC"},{"Name":"Out.PWM_BoostCntrl","Number":3645,"SramOffset":0,"Address":15789300,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"%"},{"Name":"ActualIn.p_AirInlet","Number":3472,"SramOffset":0,"Address":15788922,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001},{"Name":"In.p_AirBefThrottle","Number":3395,"SramOffset":0,"Address":15788788,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001,"Unit":"Bar"},{"Name":"ECMStat.p_Diff","Number":3759,"SramOffset":0,"Address":15789466,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001,"Unit":"Bar"},{"Name":"MAF.m_AirInlet","Number":452,"SramOffset":0,"Address":15775882,"Length":2,"Mask":0,"Type":32,"ExtendedType":0,"Correctionfactor":1,"Unit":"Mg/c"},{"Name":"m_Request","Number":59,"SramOffset":0,"Address":15775190,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1,"Unit":"Mg/c"},{"Name":"ECMStat.ST_ActiveAirDem","Number":3754,"SramOffset":0,"Address":15789448,"Length":1,"Mask":0,"Type":36,"ExtendedType":0,"Correctionfactor":1},{"Name":"DisplProt.LambdaScanner","Number":3316,"SramOffset":0,"Address":15788686,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.01},{"Name":"Lambda.LambdaInt","Number":2606,"SramOffset":0,"Address":15787098,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.01}]`,
}

func Names() []string {
	var names []string
	for name := range Map {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func Set(name string, symbols []*symbol.Symbol) error {
	if strings.EqualFold(name, "T7 Dash") || strings.EqualFold(name, "T8 Dash") || strings.EqualFold(name, "T5 Dash") {
		return fmt.Errorf("cannot replace system presets")
	}

	data, err := json.Marshal(symbols)
	if err != nil {
		return err
	}

	Map[name] = string(data)
	return nil
}

func Delete(name string) error {
	if strings.EqualFold(name, "T7 Dash") || strings.EqualFold(name, "T8 Dash") || strings.EqualFold(name, "T5 Dash") {
		return fmt.Errorf("cannot delete system presets")
	}
	delete(Map, name)
	return nil
}

func Get(name string) ([]*symbol.Symbol, error) {
	data, ok := Map[name]
	if !ok {
		return nil, fmt.Errorf("preset not found")
	}

	var symbols []*symbol.Symbol
	err := json.Unmarshal([]byte(data), &symbols)
	if err != nil {
		return nil, err
	}

	return symbols, nil
}

func Load(app fyne.App) error {
	presets := app.Preferences().String("presets")
	if presets == "" {
		setDefaults()
		return nil
	}
	if err := json.Unmarshal([]byte(presets), &Map); err != nil {
		return err
	}
	setDefaults()
	return nil
}

func setDefaults() {
	Map["T5 Dash"] = `[{"Name":"Rpm","Number":86,"SramOffset":4194,"Address":0,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Medeltrot","Number":80,"SramOffset":4150,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Ign_angle","Number":168,"SramOffset":4228,"Address":0,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Lufttemp","Number":75,"SramOffset":4145,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"P_medel","Number":320,"SramOffset":10751,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Max_tryck","Number":312,"SramOffset":10747,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Regl_tryck","Number":315,"SramOffset":10748,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":0.01},{"Name":"PWM_ut10","Number":318,"SramOffset":10754,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"P_fak","Number":313,"SramOffset":11046,"Address":0,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"I_fak","Number":314,"SramOffset":11044,"Address":0,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"D_fak","Number":311,"SramOffset":11042,"Address":0,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"AD_EGR","Number":9,"SramOffset":4118,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Kyl_temp","Number":72,"SramOffset":4141,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Bil_hast","Number":60,"SramOffset":4123,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Knock_offset1234","Number":131,"SramOffset":4236,"Address":0,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Batt_volt","Number":61,"SramOffset":4122,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Insptid_ms10","Number":64,"SramOffset":4190,"Address":0,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1},{"Name":"Lambdaint","Number":73,"SramOffset":4143,"Address":0,"Length":1,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1}]`
	Map["T7 Dash"] = `[{"Name":"ActualIn.n_Engine","Number":3462,"SramOffset":15727628,"Address":15788896,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":1},{"Name":"Out.X_AccPedal","Number":3672,"SramOffset":15727628,"Address":15789332,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"%"},{"Name":"In.v_Vehicle","Number":3409,"SramOffset":15727628,"Address":15788812,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"Km/h"},{"Name":"ActualIn.T_Engine","Number":3469,"SramOffset":15727628,"Address":15788912,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":1},{"Name":"ActualIn.T_AirInlet","Number":3470,"SramOffset":15727628,"Address":15788914,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":1},{"Name":"IgnProt.fi_Offset","Number":3045,"SramOffset":15727628,"Address":15787460,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"Degrees"},{"Name":"Out.fi_Ignition","Number":3686,"SramOffset":15727628,"Address":15789362,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"° BTDC"},{"Name":"Out.PWM_BoostCntrl","Number":3645,"SramOffset":15727628,"Address":15789296,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"%"},{"Name":"ActualIn.p_AirInlet","Number":3472,"SramOffset":15727628,"Address":15788918,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001},{"Name":"In.p_AirBefThrottle","Number":3395,"SramOffset":15727628,"Address":15788784,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001,"Unit":"Bar"},{"Name":"ECMStat.p_Diff","Number":3759,"SramOffset":15727628,"Address":15789462,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001,"Unit":"Bar"},{"Name":"MAF.m_AirInlet","Number":452,"SramOffset":15727628,"Address":15775878,"Length":2,"Mask":0,"Type":32,"ExtendedType":0,"Correctionfactor":1,"Unit":"Mg/c"},{"Name":"m_Request","Number":59,"SramOffset":15727628,"Address":15775186,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1,"Unit":"Mg/c"},{"Name":"ECMStat.ST_ActiveAirDem","Number":3754,"SramOffset":15727628,"Address":15789444,"Length":1,"Mask":0,"Type":36,"ExtendedType":0,"Correctionfactor":1},{"Name":"DisplProt.LambdaScanner","Number":3316,"SramOffset":15727628,"Address":15788682,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.01},{"Name":"Lambda.LambdaInt","Number":2606,"SramOffset":15727628,"Address":15787094,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.01},{"Name":"AdpFuelProt.MulFuelAdapt","Number":2120,"SramOffset":15727628,"Address":15786290,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.01}]`
	Map["T8 Dash"] = `[{"Name":"ActualIn.n_Engine","Number":4181,"SramOffset":0,"Address":1066236,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":1},{"Name":"Out.X_AccPos","Number":4772,"SramOffset":0,"Address":1066522,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.1},{"Name":"In.v_Vehicle","Number":4024,"SramOffset":0,"Address":1065980,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"Km/h"},{"Name":"ActualIn.T_Engine","Number":4155,"SramOffset":0,"Address":1066180,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":1},{"Name":"ActualIn.T_AirInlet","Number":4171,"SramOffset":0,"Address":1066214,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":1},{"Name":"IgnMastProt.fi_Offset","Number":3235,"SramOffset":0,"Address":1065164,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.1},{"Name":"Out.fi_Ignition","Number":4870,"SramOffset":0,"Address":1066662,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"° BTDC"},{"Name":"Out.PWM_BoostCntrl","Number":4843,"SramOffset":0,"Address":1066622,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"%"},{"Name":"In.p_AirInlet","Number":4004,"SramOffset":0,"Address":1065938,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.001},{"Name":"ActualIn.p_AirBefThrottle","Number":4159,"SramOffset":0,"Address":1066188,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.001},{"Name":"MAF.m_AirInlet","Number":383,"SramOffset":0,"Address":1056888,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":1,"Unit":"Mg/c"},{"Name":"AirMassMast.m_Request","Number":260,"SramOffset":0,"Address":1056830,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":1},{"Name":"ECMStat.ST_ActiveAirDem","Number":4977,"SramOffset":0,"Address":1067070,"Length":1,"Mask":0,"Type":4,"ExtendedType":0,"Correctionfactor":1},{"Name":"Lambda.LambdaInt","Number":2729,"SramOffset":0,"Address":1064772,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.01}]`
}

func Save(app fyne.App) error {
	presets, err := json.Marshal(Map)
	if err != nil {
		return err
	}
	app.Preferences().SetString("presets", string(presets))
	return nil
}
