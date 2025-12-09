package windows

var T5SymbolsTuningOrder = []string{
	"Diagnostics",
	"Options",
	"Injection [Fuel]",
	"Ignition",
	"Turbo control [M]",
	"Turbo control [A]",
	"Knock detection",
	"Warmup",
	"Idle",
}

var T5SymbolsTuning = map[string][]string{
	"Diagnostics": {
		"DTC Reader",
		"Pgm_status",
	},
	"Options": {
		"Pgm_mod!",
	},
	"Injection [Fuel]": {
		"VE map - normal|Insp_mat!",
		"VE map - knock|Fuel_knock_mat!",
		"Injector scaling|Inj_konst!",
		"Battery correction map|Batt_korr_tab!",
		"Fuel cut in overboost|Tryck_vakt_tab!",
	},
	"Ignition": {
		"Ignition normal|Ign_map_0!",
		"Ignition knock|Ign_map_2!",
		"Ignition warmup|Ign_map_4!",
	},
	"Turbo control [M]": {
		"Boost request map|Tryck_mat!",
		"Boost control bias|Reg_kon_mat!",
		"P factors|P_fors!",
		"I factors|I_fors!",
		"D factors|D_fors!",
		"Boost limit in 1st gear|Regl_tryck_fgm!",
		"Boost limit in 2nd gear|Regl_tryck_sgm!",
	},
	"Turbo control [A]": {
		"Boost request map|Tryck_mat_a!",
		"Boost control bias|Reg_kon_mat_a!",
		"P factors|P_fors_a!",
		"I factors|I_fors_a!",
		"D factors|D_fors_a!",
		"Boost limit in 1st gear|Regl_tryck_fgaut!",
	},
	"Knock detection": {
		"Knock sensitivity map|Knock_ref_matrix!",
		"Ignition retard limit|Knock_lim_tab!",
		"Boost reduction map|Apc_knock_tab!",
	},
	"Warmup": {
		"Afterstart enrichment (1)|Eftersta_fak!",
		"Afterstart enrichment (2)|Eftersta_fak2!",
	},
	"Idle": {
		"Idle target RPM|Idle_rpm_tab!",
		"Idle ignition|Ign_idle_angle!",
		"Idle ignition correction|Ign_map_1!",
		"Idle fuel map|Idle_fuel_korr!",
	},
}
