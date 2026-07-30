package estimatedoutput

import (
	"fmt"

	symbol "github.com/roffe/ecusymbol"
)

// T5 estimator, ported from T5Suite2.0 frmDynoChart.cs and CommonSuite
// PressureToTorque.cs. Torque is a fixed lookup on WOT boost request per
// turbo family; power = torque * rpm / 7024 (hp, metric — 7024 = 60·735.5/2π). The injector DC curve
// replays the T5 firmware injection-time math at fixed IAT (~20°C) and
// 13.0 V battery.

type t5 struct{}

const (
	t5IatAD    = 163 // AD value, ~20°C intake temperature
	t5BattVolt = 13
)

var (
	t5Turbos     = []string{"Stock", "GT17", "TD04 15T", "TD04 19T", "GT28BB", "GT28RS", "GT3071R", "HX35w", "HX40w", "S400SX3-71"}
	t5BoostAxis  = []float64{0.2, 0.4, 0.6, 0.8, 1.0, 1.2, 1.4, 1.6, 1.8, 2.0}
	t5MapSensors = []string{"2.5 bar", "3.0 bar", "3.5 bar", "4.0 bar", "5.0 bar"}
	t5MapFactor  = []float64{1.0, 1.2, 1.4, 1.6, 2.0}

	// torque (Nm) at boost (bar) breakpoints, per turbo family
	t5TqStock  = []float64{160, 210, 240, 270, 300, 360, 410, 440, 450, 460}
	t5TqGT17   = []float64{197.5, 247.5, 287.5, 340, 365, 410, 460, 533.3, 600, 640}
	t5TqTD0415 = []float64{170, 220, 260, 310, 340, 400, 450, 490, 540, 560}
	t5TqTD0419 = []float64{210, 260, 300, 360, 400, 450, 490, 540, 590, 620}
	t5TqGT3071 = []float64{250, 300, 340, 390, 440, 480, 520, 570, 610, 650}
	t5TqHX40   = []float64{270, 320, 360, 410, 460, 500, 540, 580, 620, 670}

	// indexed by the turbo choice above
	t5TorqueTables = [][]float64{
		t5TqStock,  // Stock
		t5TqGT17,   // GT17 (the suite's default table)
		t5TqTD0415, // TD04 15T
		t5TqTD0419, // TD04 19T
		t5TqTD0419, // GT28BB
		t5TqTD0419, // GT28RS
		t5TqGT3071, // GT3071R
		t5TqGT3071, // HX35w
		t5TqHX40,   // HX40w
		t5TqHX40,   // S400SX3-71
	}
)

func (t5) Options(fw symbol.SymbolCollection) []Option {
	return []Option{
		{Key: "turbo", Label: "Turbo", Kind: OptionChoice, Choices: t5Turbos},
		{Key: "mapsensor", Label: "MAP sensor", Kind: OptionChoice, Choices: t5MapSensors},
		{Key: "automatic", Label: "Automatic gearbox", Kind: OptionBool, Disabled: !has(fw, "Tryck_mat_a!")},
	}
}

type t5calc struct {
	injKonst, minTid      int
	battKorr, luftKompfak []int
	lufttempSteg          []int
	insp, fuelX, fuelY    []int
}

func (t5) Calculate(fw symbol.SymbolCollection, opts map[string]float64) (*Result, error) {
	turbo := int(opts["turbo"])
	if turbo < 0 || turbo >= len(t5TorqueTables) {
		turbo = 0
	}
	mapIdx := int(opts["mapsensor"])
	if mapIdx < 0 || mapIdx >= len(t5MapFactor) {
		mapIdx = 0
	}

	boostMapName := "Tryck_mat!"
	if opts["automatic"] == 1 {
		boostMapName = "Tryck_mat_a!"
	}
	l := &symLoader{fw: fw}
	boostMap := l.ints(boostMapName)
	rpmAxis := l.ints("Pwm_ind_rpm!")   // stored as rpm/10
	trotAxis := l.ints("Pwm_ind_trot!") // throttle axis of the boost map
	c := &t5calc{
		injKonst:     l.first("Inj_konst!"),
		minTid:       l.first("Min_tid!"),
		battKorr:     l.ints("Batt_korr_tab!"),
		luftKompfak:  l.ints("Luft_kompfak!"),
		lufttempSteg: l.ints("Lufttemp_steg!"),
		// ponytail: LOLA binaries use Inj_map_0! instead; add when someone has one
		insp:  l.ints("Insp_mat!"),
		fuelX: l.ints("Fuel_map_xaxis!"),
		fuelY: l.ints("Fuel_map_yaxis!"), // stored as rpm/10
	}
	if err := l.err(); err != nil {
		return nil, err
	}
	for i := range c.fuelY {
		c.fuelY[i] *= 10
	}

	rows := len(rpmAxis)
	if rows == 0 || len(boostMap) < rows {
		return nil, fmt.Errorf("unexpected %s/%s dimensions", boostMapName, "Pwm_ind_rpm!")
	}
	cols := len(boostMap) / rows

	res := &Result{RPM: make([]float64, rows)}
	power := make([]float64, rows)
	torque := make([]float64, rows)
	injdc := make([]float64, rows)
	for i := 0; i < rows; i++ {
		rpm := rpmAxis[i] * 10
		kpa := float64(boostMap[i*cols+cols-1]) * t5MapFactor[mapIdx] // WOT column, absolute kPa
		injdc[i] = c.injectorDC(cint(kpa), rpm)
		bar := (kpa - 100) / 100 // gauge boost
		tq := t5PressureToTorque(bar, t5TorqueTables[turbo])
		res.RPM[i] = float64(rpm)
		torque[i] = tq
		power[i] = tq * float64(rpm) / 7024
	}
	res.Curves = []Curve{
		{Name: "Power (hp)", Values: power, Peak: true},
		{Name: "Torque (Nm)", Values: torque, Peak: true},
		{Name: "Injector DC (%)", Values: injdc},
	}
	// heat table: boost request in bar over throttle x rpm, WOT on top
	tbl := &Table{Title: "Boost request (bar)", Cols: res.RPM, Precision: 2}
	for tr := cols - 1; tr >= 0; tr-- {
		row := make([]float64, rows)
		for rc := 0; rc < rows; rc++ {
			kpa := float64(boostMap[rc*cols+tr]) * t5MapFactor[mapIdx]
			row[rc] = (kpa - 100) / 100
		}
		tbl.Cells = append(tbl.Cells, row)
		hdr := float64(tr)
		if tr < len(trotAxis) {
			hdr = float64(trotAxis[tr])
		}
		tbl.Rows = append(tbl.Rows, hdr)
	}
	res.Table = tbl
	return res, nil
}

// t5PressureToTorque ports CommonSuite PressureToTorque.Handle_tables: the
// y interpolation is computed and discarded in the original, so this is a
// 1-D lookup that extrapolates linearly below 0.2 bar and clamps flat
// above 2.0 bar.
func t5PressureToTorque(bar float64, tbl []float64) float64 {
	xi := 0
	for xi <= len(t5BoostAxis)-1 {
		if t5BoostAxis[xi] > bar {
			break
		}
		xi++
	}
	if xi > 0 {
		xi--
	}
	if xi < len(tbl)-1 {
		return (tbl[xi+1]-tbl[xi])*(bar-t5BoostAxis[xi])/(t5BoostAxis[xi+1]-t5BoostAxis[xi]) + tbl[xi]
	}
	return tbl[xi]
}

// injectorDC replays BoostRpmToInjectorDuration: load from boost and IAT,
// VE cell from the fuel map, base injection time from the injector
// constant, battery correction, then duty cycle in percent.
func (c *t5calc) injectorDC(pressure, rpm int) float64 {
	ltf := t5TempTable(t5IatAD, c.luftKompfak, c.lufttempSteg)
	pm10 := pressure * 10
	last := ((ltf + 384) * (pm10 / 10)) / 512
	if last >= 255 {
		last = 255
	}
	cell := int(t5FuelCell(rpm, uint8(last), c.insp, c.fuelY, c.fuelX))
	injDur := c.injKonst * ((ltf + 384) * pressure) / 512 // 4µs ticks
	ms := float64(injDur) * ((float64(cell) + 128) / 256)
	if len(c.battKorr) > 14-t5BattVolt {
		ms += float64(c.battKorr[14-t5BattVolt])
	}
	if ms >= 32500 {
		ms = 32500
	}
	if ms <= float64(c.minTid) {
		ms = float64(c.minTid)
	}
	ms /= 250 // ticks -> ms
	return float64(rpm) * ms / 1200
}

// t5TempTable ports Handle_temp_tables: breakpoint interpolation in pure
// integer math, terminating on the 255 sentinel in the steg table.
func t5TempTable(ad int, tab, steg []int) int {
	if len(tab) == 0 || len(steg) == 0 {
		return 0
	}
	i := 0
	for i < len(steg) && steg[i] != 255 {
		if steg[i] >= ad {
			break
		}
		i++
	}
	if i == 0 {
		return tab[0]
	}
	i--
	if i+1 >= len(tab) || i+1 >= len(steg) {
		return tab[min(i, len(tab)-1)]
	}
	d := steg[i+1] - steg[i]
	if d == 0 {
		return tab[i]
	}
	if tab[i+1] > tab[i] {
		return tab[i] + ((tab[i+1]-tab[i])*(ad-steg[i]))/d
	}
	return tab[i] - ((tab[i]-tab[i+1])*(ad-steg[i]))/d
}

// t5FuelCell ports the byte flavor of Handle_tables, including its uint8
// truncation semantics; yaxis is RPM, xaxis is load.
func t5FuelCell(rpm int, load uint8, matrix, yaxis, xaxis []int) uint8 {
	yc, xc := len(yaxis), len(xaxis)
	if yc == 0 || xc == 0 || len(matrix) == 0 {
		return 0
	}
	yi := 0
	for yi <= yc-1 {
		if yaxis[yi] > rpm {
			break
		}
		yi++
	}
	xi := 0
	for xi <= xc-1 {
		if xaxis[xi] > int(load) {
			break
		}
		xi++
	}
	if xi > 0 {
		xi--
	}
	if yi > 0 {
		yi--
	}
	at := func(i int) int {
		if i < 0 || i >= len(matrix) {
			return 0
		}
		return matrix[i]
	}
	tmp1 := at(yi*xc + xi)
	tmp2 := tmp1
	if yi < yc-1 {
		tmp2 = at((yi+1)*xc + xi)
	}
	tmp3 := tmp1
	if xi < xc-1 {
		tmp3 = at(yi*xc + xi + 1)
	}
	var vx, vy uint8
	if xi < xc-1 && yi < yc-1 {
		vx = t5Interp2(tmp3-tmp1, xaxis[xi+1]-xaxis[xi], int(load)-xaxis[xi], tmp1)
		vy = t5Interp2(tmp2-tmp1, yaxis[yi+1]-yaxis[yi], rpm-yaxis[yi], tmp1)
	} else {
		vx = t5Interp2(tmp3-tmp1, 0, int(load)-xaxis[xi], tmp1)
		vy = t5Interp2(tmp2-tmp1, 0, rpm-yaxis[yi], tmp1)
	}
	return uint8((int(vy) + int(vx)) / 2)
}

func t5Interp2(diff, axisDiff, offset, base int) uint8 {
	if axisDiff != 0 {
		return uint8((diff*offset)/axisDiff + base)
	}
	return uint8(base)
}
