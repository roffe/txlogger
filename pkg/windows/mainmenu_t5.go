package windows

func (mw *MainWindow) t5Menu() []MenuItem {
	return []MenuItem{
		{Name: "Diagnostics", Children: []MenuItem{
			{Name: "DTC Reader", Func: mw.openDTCReader},
			{Name: "Pgm_status", Func: mw.openPgmStatus},
		}},
		{Name: "Options", Children: []MenuItem{
			{Name: "Pgm_mod!", Func: mw.openPgmMod},
		}},
		{Name: "Injection [Fuel]", Children: []MenuItem{
			{Name: "VE map - normal", Data: "Insp_mat!"},
			{Name: "VE map - knock", Data: "Fuel_knock_mat!"},
			{Name: "Injector scaling", Data: "Inj_konst!"},
			{Name: "Battery correction map", Data: "Batt_korr_tab!"},
			{Name: "Fuel cut in overboost", Data: "Tryck_vakt_tab!"},
		}},
		{Name: "Ignition", Children: []MenuItem{
			{Name: "Ignition normal", Data: "Ign_map_0!"},
			{Name: "Ignition knock", Data: "Ign_map_2!"},
			{Name: "Ignition warmup", Data: "Ign_map_4!"},
		}},
		{Name: "Turbo control [M]", Children: []MenuItem{
			{Name: "Boost request map", Data: "Tryck_mat!"},
			{Name: "Boost control bias", Data: "Reg_kon_mat!"},
			{Name: "P factors", Data: "P_fors!"},
			{Name: "I factors", Data: "I_fors!"},
			{Name: "D factors", Data: "D_fors!"},
			{Name: "Boost limit in 1st gear", Data: "Regl_tryck_fgm!"},
			{Name: "Boost limit in 2nd gear", Data: "Regl_tryck_sgm!"},
		}},
		{Name: "Turbo control [A]", Children: []MenuItem{
			{Name: "Boost request map", Data: "Tryck_mat_a!"},
			{Name: "Boost control bias", Data: "Reg_kon_mat_a!"},
			{Name: "P factors", Data: "P_fors_a!"},
			{Name: "I factors", Data: "I_fors_a!"},
			{Name: "D factors", Data: "D_fors_a!"},
			{Name: "Boost limit in 1st gear", Data: "Regl_tryck_fgaut!"},
		}},
		{Name: "Knock detection", Children: []MenuItem{
			{Name: "Knock sensitivity map", Data: "Knock_ref_matrix!"},
			{Name: "Ignition retard limit", Data: "Knock_lim_tab!"},
			{Name: "Boost reduction map", Data: "Apc_knock_tab!"},
		}},
		{Name: "Warmup", Children: []MenuItem{
			{Name: "Afterstart enrichment (1)", Data: "Eftersta_fak!"},
			{Name: "Afterstart enrichment (2)", Data: "Eftersta_fak2!"},
		}},
		{Name: "Idle", Children: []MenuItem{
			{Name: "Idle target RPM", Data: "Idle_rpm_tab!"},
			{Name: "Idle ignition", Data: "Ign_idle_angle!"},
			{Name: "Idle ignition correction", Data: "Ign_map_1!"},
			{Name: "Idle fuel map", Data: "Idle_fuel_korr!"},
		}},
	}
}
