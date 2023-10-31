package presets

import "sort"

var Map = map[string]string{
	// "LJR":          `[{"name":"ActualIn.T_Engine","method":2,"value":3469,"type":33,"length":2,"correctionfactor":1,"group":"Temperature"},{"name":"ActualIn.T_AirInlet","method":2,"value":3470,"type":33,"length":2,"correctionfactor":1,"group":"Temperature"},{"name":"IgnProt.fi_Offset","method":2,"value":3045,"type":33,"length":2,"correctionfactor":0.1,"group":"Ignition"},{"name":"Out.fi_Ignition","method":2,"value":3686,"type":33,"length":2,"correctionfactor":0.1,"group":"Ignition"},{"name":"Out.PWM_BoostCntrl","method":2,"value":3645,"type":33,"length":2,"correctionfactor":0.1,"group":"%"},{"name":"ActualIn.p_AirInlet","method":2,"value":3472,"type":33,"length":2,"correctionfactor":0.001,"group":"Boost"},{"name":"In.p_AirBefThrottle","method":2,"value":3395,"type":33,"length":2,"correctionfactor":0.001,"group":"Boost"},{"name":"MAF.m_AirInlet","method":2,"value":452,"type":32,"length":2,"correctionfactor":1,"group":"Air mg/c"},{"name":"m_Request","method":2,"value":59,"type":0,"length":2,"correctionfactor":1,"group":"Air mg/c"},{"name":"ECMStat.ST_ActiveAirDem","method":2,"value":3754,"type":36,"length":1,"correctionfactor":1,"group":"Limiters"}]`,
	// "JSH":          `[{"name":"ActualIn.T_Engine","method":2,"value":3469,"type":33,"length":2,"correctionfactor":1,"group":"Temperature"},{"name":"ActualIn.T_AirInlet","method":2,"value":3470,"type":33,"length":2,"correctionfactor":1,"group":"Temperature"},{"name":"IgnProt.fi_Offset","method":2,"value":3045,"type":33,"length":2,"correctionfactor":0.1,"group":"Ignition"},{"name":"Out.fi_Ignition","method":2,"value":3686,"type":33,"length":2,"correctionfactor":0.1,"group":"Ignition"},{"name":"Out.PWM_BoostCntrl","method":2,"value":3645,"type":33,"length":2,"correctionfactor":0.1,"group":"%"},{"name":"ActualIn.p_AirInlet","method":2,"value":3472,"type":33,"length":2,"correctionfactor":0.001,"group":"Bar"},{"name":"In.p_AirBefThrottle","method":2,"value":3395,"type":33,"length":2,"correctionfactor":0.001,"group":"Bar"},{"name":"ECMStat.p_Diff","method":2,"value":3759,"type":33,"length":2,"correctionfactor":0.001,"group":"Bar"},{"name":"MAF.m_AirInlet","method":2,"value":452,"type":32,"length":2,"correctionfactor":1,"group":"Air mg/c"},{"name":"m_Request","method":2,"value":59,"type":0,"length":2,"correctionfactor":1,"group":"Air mg/c"},{"name":"ECMStat.ST_ActiveAirDem","method":2,"value":3754,"type":36,"length":1,"correctionfactor":1,"group":"Limiters"}]`,
	"T7 Dash":                    `[{"Name":"ActualIn.T_Engine","Number":3469,"SramOffset":0,"Address":15788916,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":1},{"Name":"ActualIn.T_AirInlet","Number":3470,"SramOffset":0,"Address":15788918,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":1},{"Name":"IgnProt.fi_Offset","Number":3045,"SramOffset":0,"Address":15787464,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"Degrees"},{"Name":"Out.fi_Ignition","Number":3686,"SramOffset":0,"Address":15789366,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"° BTDC"},{"Name":"Out.PWM_BoostCntrl","Number":3645,"SramOffset":0,"Address":15789300,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"%"},{"Name":"ActualIn.p_AirInlet","Number":3472,"SramOffset":0,"Address":15788922,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001},{"Name":"In.p_AirBefThrottle","Number":3395,"SramOffset":0,"Address":15788788,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001,"Unit":"Bar"},{"Name":"ECMStat.p_Diff","Number":3759,"SramOffset":0,"Address":15789466,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001,"Unit":"Bar"},{"Name":"MAF.m_AirInlet","Number":452,"SramOffset":0,"Address":15775882,"Length":2,"Mask":0,"Type":32,"ExtendedType":0,"Correctionfactor":1,"Unit":"Mg/c"},{"Name":"m_Request","Number":59,"SramOffset":0,"Address":15775190,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1,"Unit":"Mg/c"},{"Name":"ECMStat.ST_ActiveAirDem","Number":3754,"SramOffset":0,"Address":15789448,"Length":1,"Mask":0,"Type":36,"ExtendedType":0,"Correctionfactor":1},{"Name":"DisplProt.LambdaScanner","Number":3316,"SramOffset":0,"Address":15788686,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.01},{"Name":"Lambda.LambdaInt","Number":2606,"SramOffset":0,"Address":15787098,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.01}]`,
	"T7 Dash for OBDLink Cables": `[{"Name":"ActualIn.T_Engine","Number":3469,"SramOffset":0,"Address":15788916,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":1},{"Name":"ActualIn.T_AirInlet","Number":3470,"SramOffset":0,"Address":15788918,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":1},{"Name":"IgnProt.fi_Offset","Number":3045,"SramOffset":0,"Address":15787464,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"Degrees"},{"Name":"Out.fi_Ignition","Number":3686,"SramOffset":0,"Address":15789366,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"° BTDC"},{"Name":"Out.PWM_BoostCntrl","Number":3645,"SramOffset":0,"Address":15789300,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"%"},{"Name":"ActualIn.p_AirInlet","Number":3472,"SramOffset":0,"Address":15788922,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001},{"Name":"In.p_AirBefThrottle","Number":3395,"SramOffset":0,"Address":15788788,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001,"Unit":"Bar"},{"Name":"ECMStat.p_Diff","Number":3759,"SramOffset":0,"Address":15789466,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.001,"Unit":"Bar"},{"Name":"MAF.m_AirInlet","Number":452,"SramOffset":0,"Address":15775882,"Length":2,"Mask":0,"Type":32,"ExtendedType":0,"Correctionfactor":1,"Unit":"Mg/c"},{"Name":"m_Request","Number":59,"SramOffset":0,"Address":15775190,"Length":2,"Mask":0,"Type":0,"ExtendedType":0,"Correctionfactor":1,"Unit":"Mg/c"},{"Name":"ECMStat.ST_ActiveAirDem","Number":3754,"SramOffset":0,"Address":15789448,"Length":1,"Mask":0,"Type":36,"ExtendedType":0,"Correctionfactor":1},{"Name":"DisplProt.LambdaScanner","Number":3316,"SramOffset":0,"Address":15788686,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.01},{"Name":"Lambda.LambdaInt","Number":2606,"SramOffset":0,"Address":15787098,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.01},{"Name":"ActualIn.n_Engine","Number":3462,"SramOffset":0,"Address":15788900,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":1},{"Name":"Out.X_AccPedal","Number":3672,"SramOffset":0,"Address":15789336,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"%"},{"Name":"In.v_Vehicle","Number":3409,"SramOffset":0,"Address":15788816,"Length":2,"Mask":0,"Type":33,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"Km/h"}]`,
	"T8 Dash":                    `[{"Name":"ActualIn.T_Engine","Number":4155,"SramOffset":0,"Address":1066180,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":1},{"Name":"ActualIn.T_AirInlet","Number":4171,"SramOffset":0,"Address":1066214,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":1},{"Name":"IgnMastProt.fi_Offset","Number":3235,"SramOffset":0,"Address":1065164,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.1},{"Name":"Out.fi_Ignition","Number":4870,"SramOffset":0,"Address":1066662,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"° BTDC"},{"Name":"Out.PWM_BoostCntrl","Number":4843,"SramOffset":0,"Address":1066622,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"%"},{"Name":"In.p_AirInlet","Number":4004,"SramOffset":0,"Address":1065938,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.001},{"Name":"ActualIn.p_AirBefThrottle","Number":4159,"SramOffset":0,"Address":1066188,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.001},{"Name":"MAF.m_AirInlet","Number":383,"SramOffset":0,"Address":1056888,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":1,"Unit":"Mg/c"},{"Name":"AirMassMast.m_Request","Number":260,"SramOffset":0,"Address":1056830,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":1},{"Name":"ECMStat.ST_ActiveAirDem","Number":4977,"SramOffset":0,"Address":1067070,"Length":1,"Mask":0,"Type":4,"ExtendedType":0,"Correctionfactor":1},{"Name":"Lambda.LambdaInt","Number":2729,"SramOffset":0,"Address":1064772,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.01},{"Name":"ActualIn.n_Engine","Number":4181,"SramOffset":0,"Address":1066236,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":1},{"Name":"Out.X_AccPos","Number":4772,"SramOffset":0,"Address":1066522,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.1},{"Name":"In.v_Vehicle","Number":4024,"SramOffset":0,"Address":1065980,"Length":2,"Mask":0,"Type":1,"ExtendedType":0,"Correctionfactor":0.1,"Unit":"Km/h"}]`,
}

func Names() []string {
	var names []string
	for name := range Map {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
