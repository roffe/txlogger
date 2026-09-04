// Package t7fwinfo is T7Suite's "Firmware information" dialog: PI-area identity
// of the loaded Trionic 7 binary plus the calibration switches a tuner flips
// (SID info, torque limiters, OBDII, second lambda, throttle response,
// catalyst light-off, BioPower, SID and emission patches). Identity fields are
// read-only here; the PI area editor edits them.
package t7fwinfo

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
)

type Config struct {
	FW   *symbol.T7File
	Save func() error // writes FW back to disk (takes the .bak); nil = read-only
	Log  func(string)
}

type check struct {
	w       *widget.Check
	present bool
	get     func(symbol.T7FirmwareInfo) bool
	set     func(bool) error
}

type Widget struct {
	widget.BaseWidget
	cfg    Config
	checks []*check
	idents map[byte]*widget.Entry // PI-area text fields by id
	status *widget.Label
	root   fyne.CanvasObject
}

func New(cfg Config) *Widget {
	t := &Widget{cfg: cfg, status: widget.NewLabel(""), idents: map[byte]*widget.Entry{}}
	t.ExtendBaseWidget(t)
	t.build()
	return t
}

func (t *Widget) add(label string, present bool, get func(symbol.T7FirmwareInfo) bool, set func(bool) error) *widget.Check {
	c := &check{w: widget.NewCheck(label, nil), present: present, get: get, set: set}
	if !present || set == nil {
		c.w.Disable()
	}
	t.checks = append(t.checks, c)
	return c.w
}

func (t *Widget) load() {
	fi := t.cfg.FW.GetInfo()
	for _, c := range t.checks {
		c.w.SetChecked(c.get(fi))
	}
}

func (t *Widget) build() {
	fw := t.cfg.FW
	fi := fw.GetInfo()
	ro := func(v string) *widget.Entry {
		e := widget.NewEntry()
		e.SetText(v)
		e.Disable()
		return e
	}
	// editable PI-area text field; nil Save makes it read-only
	rw := func(id byte, v string) *widget.Entry {
		e := widget.NewEntry()
		e.SetText(v)
		if t.cfg.Save == nil {
			e.Disable()
		}
		t.idents[id] = e
		return e
	}
	progDate := "not set"
	if !fi.ProgrammingDateTime.IsZero() {
		progDate = fi.ProgrammingDateTime.Format(time.DateTime)
	}
	identity := widget.NewForm(
		widget.NewFormItem("Engine type", rw(0x97, fi.EngineType)),
		widget.NewFormItem("Software version", rw(0x95, fi.SoftwareVersion)),
		widget.NewFormItem("Partnumber", ro(fi.Partnumber)),
		widget.NewFormItem("Immobilizer code", rw(0x92, fi.ImmobilizerCode)),
		widget.NewFormItem("Chassis ID", rw(0x90, fi.ChassisID)),
		widget.NewFormItem("Programming date", ro(progDate)),
		widget.NewFormItem("SID date", rw(0x99, fi.SIDDate)),
	)

	details := container.NewGridWithColumns(
		3,
		t.add("Checksum enabled", true, func(i symbol.T7FirmwareInfo) bool { return i.ChecksumEnabled }, nil),
		t.add("Compressed symboltable", true, func(i symbol.T7FirmwareInfo) bool { return i.CompressedSymboltable }, nil),
		t.add("No symboltable present", true, func(i symbol.T7FirmwareInfo) bool { return i.MissingSymbolTable }, nil),
	)
	var fast, extra *widget.Check
	fast = t.add("Fast throttle response", fi.FastThrottlePresent, func(i symbol.T7FirmwareInfo) bool { return i.FastThrottle },
		func(b bool) error { return fw.SetFastThrottle(b, b && extra.Checked) })
	extra = t.add("Extra fast throttle response", fi.FastThrottlePresent, func(i symbol.T7FirmwareInfo) bool { return i.ExtraFastThrottle },
		func(b bool) error { return fw.SetFastThrottle(fast.Checked || b, b) })
	openChk := t.add("Open SID info", fi.SIDInfoPresent, func(i symbol.T7FirmwareInfo) bool { return i.SIDInfoOpen }, fw.SetSIDInfoOpen)
	trq := t.add("Torque limiters enabled", fi.TorqueLimitersPresent, func(i symbol.T7FirmwareInfo) bool { return i.TorqueLimitersEnabled }, fw.SetTorqueLimiters)
	cat := t.add("Catalyst lightoff", fi.CatalystLightOffPresent, func(i symbol.T7FirmwareInfo) bool { return i.CatalystLightOff }, fw.SetCatalystLightOff)
	eth := t.add("Ethanol sensor (\"No TCS\" in T7Suite)", fi.BioPowerSoftware, func(i symbol.T7FirmwareInfo) bool { return i.EthanolSensor }, fw.SetEthanolSensor)
	l2 := t.add("Second lambda sonde enabled", fi.SecondLambdaPresent, func(i symbol.T7FirmwareInfo) bool { return i.SecondLambdaEnabled }, fw.SetSecondLambda)
	obd := t.add("OBDII functions enabled", fi.OBDIIPresent, func(i symbol.T7FirmwareInfo) bool { return i.OBDIIEnabled }, fw.SetOBDII)
	bio := t.add("BioPower enabled", fi.BioPowerSoftware, func(i symbol.T7FirmwareInfo) bool { return i.BioPowerEnabled }, fw.SetBioPower)
	em := t.add("Disable emission limiting function", fi.EmissionLimitingPatchPresent, func(i symbol.T7FirmwareInfo) bool { return i.EmissionLimitingDisabled }, fw.SetEmissionLimitingDisabled)
	options := container.NewGridWithColumns(
		3,
		openChk, l2, fast,
		trq, obd, extra,
		cat, bio, em,
		eth,
	)
	var sidStart, sidAdapt *widget.Check
	sidStart = t.add("Disable startscreen", fi.SIDOptionsPresent, func(i symbol.T7FirmwareInfo) bool { return i.SIDDisableStartScreen },
		func(b bool) error { return fw.SetSIDOptions(b, sidAdapt.Checked) })
	sidAdapt = t.add("Disable adaption messages", fi.SIDOptionsPresent, func(i symbol.T7FirmwareInfo) bool { return i.SIDDisableAdaptionMessages },
		func(b bool) error { return fw.SetSIDOptions(sidStart.Checked, b) })
	sid := container.NewGridWithColumns(3, sidStart, sidAdapt)

	apply := widget.NewButton("Apply", t.apply)
	if t.cfg.Save == nil {
		apply.Disable()
	}
	revert := widget.NewButton("Revert", t.load)
	t.load()

	t.root = container.NewVScroll(container.NewVBox(
		widget.NewCard("Firmware details", "", identity),
		details,
		widget.NewCard("Options", "", options),
		widget.NewCard("Advanced SID options", "EU0AF01C, ET03F01C.46S and ET02U01C only", sid),
		container.NewHBox(apply, revert, t.status),
	))
}

// apply pushes every editable checkbox that differs from the binary into it,
// then saves. Order does not matter: each setter touches its own bytes.
func (t *Widget) apply() {
	fi := t.cfg.FW.GetInfo()
	changed := 0
	for _, c := range t.checks {
		if c.set == nil || !c.present || c.w.Checked == c.get(fi) {
			continue
		}
		if err := c.set(c.w.Checked); err != nil {
			t.status.SetText(fmt.Sprintf("%s: %v", c.w.Text, err))
			return
		}
		changed++
	}
	if n, err := t.applyIdentity(); err != nil {
		t.status.SetText(err.Error())
		return
	} else {
		changed += n
	}
	if changed == 0 {
		t.status.SetText("nothing changed")
		return
	}
	if err := t.cfg.Save(); err != nil {
		t.status.SetText("save failed: " + err.Error())
		return
	}
	t.load()
	t.status.SetText(fmt.Sprintf("saved %d change(s)", changed))
	if t.cfg.Log != nil {
		t.cfg.Log(fmt.Sprintf("firmware information: saved %d change(s)", changed))
	}
}

// applyIdentity writes edited PI-area text fields (padded/truncated to the
// field's existing length, as the ECU reads fixed-width strings) via SetPIArea.
func (t *Widget) applyIdentity() (int, error) {
	fields := t.cfg.FW.GetHeaders()
	changed := 0
	for _, f := range fields {
		e, ok := t.idents[f.ID]
		if !ok || e.Text == f.String() {
			continue
		}
		b := []byte(fmt.Sprintf("%-*s", len(f.Data), e.Text))[:len(f.Data)]
		f.Data = b
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	if err := t.cfg.FW.SetPIArea(fields); err != nil {
		return 0, fmt.Errorf("PI area: %w", err)
	}
	return changed, nil
}

func (t *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.root)
}
