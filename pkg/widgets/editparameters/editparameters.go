package editparameters

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t8"
)

type EditParameters struct {
	widget.BaseWidget

	vin            binding.String
	e85Percent     binding.String
	topSpeed       binding.String
	oilQuality     binding.String
	diagnosticType binding.String
	tankType       binding.String
	convertible    binding.Bool
	sai            binding.Bool
	highOutput     binding.Bool
	bioPower       binding.Bool
	clutchStart    binding.Bool

	getAdapter func() (gocan.Adapter, error)
	err        func(error)
	log        func(string)

	hasBeenRead bool
}

func NewEditParameters(getAdapter func() (gocan.Adapter, error), errFn func(error), logFn func(string)) *EditParameters {
	t := &EditParameters{
		vin:            binding.NewString(),
		e85Percent:     binding.NewString(),
		topSpeed:       binding.NewString(),
		oilQuality:     binding.NewString(),
		diagnosticType: binding.NewString(),
		tankType:       binding.NewString(),
		convertible:    binding.NewBool(),
		sai:            binding.NewBool(),
		highOutput:     binding.NewBool(),
		bioPower:       binding.NewBool(),
		clutchStart:    binding.NewBool(),

		getAdapter: getAdapter,
		err:        errFn,
		log:        logFn,
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *EditParameters) CreateRenderer() fyne.WidgetRenderer {
	vinEntry := widget.NewEntry()
	vinEntry.Bind(t.vin)
	vinEntry.OnChanged = func(s string) {
		_ = t.vin.Set(s)
	}

	vinEntry.Validator = func(s string) error {
		if len(s) != 17 {
			return errors.New("VIN must be 17 characters long")
		}
		return nil
	}

	e85percentEntry := widget.NewEntry()
	e85percentEntry.Bind(t.e85Percent)
	e85percentEntry.OnChanged = func(s string) {
		_ = t.e85Percent.Set(s)
	}
	e85percentEntry.Validator = func(s string) error {
		val, err := strconv.Atoi(s)
		if err != nil {
			return errors.New("Must be between 0 and 85")
		}
		if val < 0 || val > 85 {
			return errors.New("Must be between 0 and 85")
		}
		return nil
	}

	topSpeedEntry := widget.NewEntry()
	topSpeedEntry.Bind(t.topSpeed)
	topSpeedEntry.OnChanged = func(s string) {
		_ = t.topSpeed.Set(s)
	}
	topSpeedEntry.Validator = func(s string) error {
		val, err := strconv.Atoi(s)
		if err != nil {
			return errors.New("Must be between 0 and 3276")
		}
		if val < 0 || val > 3276 {
			return errors.New("Must be between 0 and 3276")
		}
		return nil
	}

	oilQualityEntry := widget.NewEntry()
	oilQualityEntry.Bind(t.oilQuality)
	oilQualityEntry.OnChanged = func(s string) {
		_ = t.oilQuality.Set(s)
	}
	oilQualityEntry.Validator = func(s string) error {
		val, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return errors.New("Must be between 0 and 100")
		}
		if val < 0 || val > 100 {
			return errors.New("Must be between 0 and 100")
		}
		return nil
	}

	diagnosticTypeSelect := widget.NewSelect([]string{"None", "OBD2", "EOBD", "LOBD"}, func(s string) {
		_ = t.diagnosticType.Set(s)
	})
	diagnosticTypeSelect.Bind(t.diagnosticType)

	tankTypeSelect := widget.NewSelect([]string{"US", "EU", "AWD"}, func(s string) {
		_ = t.tankType.Set(s)
	})
	tankTypeSelect.Bind(t.tankType)

	convertibleCheck := widget.NewCheck("", func(b bool) {
		_ = t.convertible.Set(b)
	})
	convertibleCheck.Bind(t.convertible)

	saiCheck := widget.NewCheck("", func(b bool) {
		_ = t.sai.Set(b)
	})
	saiCheck.Bind(t.sai)

	highOutputCheck := widget.NewCheck("", func(b bool) {
		_ = t.highOutput.Set(b)
	})
	highOutputCheck.Bind(t.highOutput)

	bioPowerCheck := widget.NewCheck("", func(b bool) {
		_ = t.bioPower.Set(b)
	})
	bioPowerCheck.Bind(t.bioPower)

	clutchStartCheck := widget.NewCheck("", func(b bool) {
		_ = t.clutchStart.Set(b)
	})
	clutchStartCheck.Bind(t.clutchStart)

	formItems := []*widget.FormItem{
		{Text: "VIN", Widget: vinEntry},
		{Text: "E85%", Widget: e85percentEntry},
		{Text: "Top Speed (km/h)", Widget: topSpeedEntry},
		{Text: "Oil Quaility %", Widget: oilQualityEntry},
		{Text: "Diagnostic Type", Widget: diagnosticTypeSelect},
		{Text: "Tank Type", Widget: tankTypeSelect},
		{Text: "Convertible", Widget: convertibleCheck},
		{Text: "SAI", Widget: saiCheck},
		{Text: "High Output", Widget: highOutputCheck},
		{Text: "BioPower", Widget: bioPowerCheck},
		{Text: "Clutch Start", Widget: clutchStartCheck},
	}

	form := widget.NewForm(formItems...)

	form.SubmitText = "Write fields to ECU"
	form.CancelText = "Read fields from ECU"
	form.OnCancel = t.readParameters
	form.OnSubmit = t.writeParameters

	return widget.NewSimpleRenderer(form)
}

func (t *EditParameters) readParameters() {
	t.log("Reading parameters from ECU...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dev, err := t.getAdapter()
	if err != nil {
		t.err(err)
		return
	}
	eventHandler := func(e gocan.Event) {
		log.Printf("EVENT: %v", e)
	}

	cl, err := gocan.NewWithOpts(ctx, dev, gocan.WithEventHandler(eventHandler))
	if err != nil {
		t.err(err)
		return
	}
	gm := gmlan.New(cl, 0x7e0, 0x5e8, 0x7e8)
	t8c := &T8{gm: gm}

	go func() {
		defer cl.Close()

		defer func() {
			_ = gm.ReturnToNormalMode(ctx)
			time.Sleep(75 * time.Millisecond)
		}()

		data, err := gm.ReadDataByIdentifier(ctx, 0x01)
		if err != nil {
			t.err(err)
			return
		}

		status, err := t8.DecodePI01(data)
		if err != nil {
			t.err(err)
			return
		}

		t.SetDiagnosticType(status.DiagnosticType.String())
		t.SetTankType(status.TankType.String())
		t.SetConvertible(status.Convertible)
		t.SetSAI(status.SAI)
		t.SetHighOutput(status.HighOutput)
		t.SetBioPower(status.BioPower)
		t.SetClutchStart(status.ClutchStart)

		oilQuality, err := t8c.GetOilQuality(ctx)
		if err != nil {
			t.err(err)
			return
		}
		t.SetOilQuality(strconv.FormatFloat(oilQuality, 'f', 2, 64))

		vin, err := t8c.GetVehicleVIN(ctx)
		if err != nil {
			t.err(err)
			return
		}
		t.SetVIN(vin)

		topSpeed, err := t8c.GetTopSpeed(ctx)
		if err != nil {
			t.err(err)
			return
		}
		t.SetTopSpeed(strconv.Itoa(int(topSpeed)))

		time.Sleep(5 * time.Millisecond)

		e85percentage, err := t8c.GetE85Percent(ctx)
		if err != nil {
			t.err(err)
			return
		}
		t.SetE85Percent(strconv.FormatFloat(e85percentage, 'f', 0, 64))
	}()

	if err := cl.Wait(ctx); err != nil {
		t.err(err)
		return
	}
	t.hasBeenRead = true
}

func (t *EditParameters) writeParameters() {
	if !t.hasBeenRead {
		t.err(errors.New("You must read the parameters from the ECU before writing"))
		return
	}

	t.log("Writing parameters to ECU...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dev, err := t.getAdapter()
	if err != nil {
		t.err(err)
		return
	}
	eventHandler := func(e gocan.Event) {
		log.Printf("EVENT: %v", e)
	}

	cl, err := gocan.NewWithOpts(ctx, dev, gocan.WithEventHandler(eventHandler))
	if err != nil {
		t.err(err)
		return
	}
	gm := gmlan.New(cl, 0x7e0, 0x5e8, 0x7e8)
	t8c := &T8{gm: gm}

	go func() {
		defer cl.Close()

		//if err := gm.InitiateDiagnosticOperation(ctx, gmlan.LEV_EDDDC); err != nil {
		//	log.Println(err)
		//	return
		//}

		defer func() {
			_ = gm.ReturnToNormalMode(ctx)
			time.Sleep(75 * time.Millisecond)
		}()

		if err := gm.RequestSecurityAccess(ctx, 0xFD, 1, ecu.CalculateT8AccessKey); err != nil {
			t.err(err)
			return
		}

		vin, err := t.GetVIN()
		if err != nil {
			t.err(fmt.Errorf("Error getting VIN: %w", err))
			return
		}
		if err := t8c.SetVehicleVIN(ctx, vin); err != nil {
			t.err(fmt.Errorf("Error setting VIN: %w", err))
			return
		}

		e85content, err := t.GetE85Percent()
		if err != nil {
			t.err(fmt.Errorf("Error getting E85 content: %w", err))
			return
		}
		e85percent, err := strconv.ParseFloat(e85content, 64)
		if err != nil {
			t.err(fmt.Errorf("Error parsing E85 content: %w", err))
			return
		}
		if err := t8c.SetE85Percent(ctx, e85percent); err != nil {
			t.err(fmt.Errorf("Error setting E85 percent: %w", err))
			return
		}

		topSpeed, err := t.GetTopSpeed()
		if err != nil {
			t.err(fmt.Errorf("Error getting Top Speed: %w", err))
			return
		}
		topSpeedVal, err := strconv.Atoi(topSpeed)
		if err != nil {
			t.err(fmt.Errorf("Error parsing Top Speed: %w", err))
			return
		}
		if err := t8c.SetTopSpeed(ctx, uint16(topSpeedVal)); err != nil {
			t.err(fmt.Errorf("Error setting Top Speed: %w", err))
			return
		}

		oilQuality, err := t.GetOilQuality()
		if err != nil {
			t.err(fmt.Errorf("Error getting oil quality: %w", err))
			return
		}
		oilQualityVal, err := strconv.ParseFloat(oilQuality, 64)
		if err != nil {
			t.err(fmt.Errorf("Error parsing oil quality: %w", err))
			return
		}
		if err := t8c.SetOilQuality(ctx, oilQualityVal); err != nil {
			t.err(fmt.Errorf("Error setting oil quality: %w", err))
			return
		}

		data, err := gm.ReadDataByIdentifier(ctx, 0x01)
		if err != nil {
			t.err(fmt.Errorf("Error reading PI01: %w", err))
			return
		}

		pi01, err := t.GetPI01Data()
		if err != nil {
			t.err(fmt.Errorf("Error getting PI 0x01 data: %w", err))
			return
		}

		// -------C
		data[0] = setBit(data[0], 0, pi01.BioPower)

		// -----C--
		data[0] = setBit(data[0], 2, pi01.Convertible)

		// ---01--- US
		// ---10--- EU
		// ---11--- AWD
		switch pi01.TankType {
		case t8.TankTypeUS:
			data[0] = setBit(data[0], 3, true)
			data[0] = setBit(data[0], 4, false)
		case t8.TankTypeEU:
			data[0] = setBit(data[0], 3, false)
			data[0] = setBit(data[0], 4, true)
		case t8.TankTypeAWD:
			data[0] = setBit(data[0], 3, true)
			data[0] = setBit(data[0], 4, true)
		}

		// -01----- OBD2
		// -10----- EOBD
		// -11----- LOBD
		switch pi01.DiagnosticType {
		case t8.DiagnosticTypeOBD2:
			data[0] = setBit(data[0], 5, true)
			data[0] = setBit(data[0], 6, false)
		case t8.DiagnosticTypeEOBD:
			data[0] = setBit(data[0], 5, false)
			data[0] = setBit(data[0], 6, true)
		case t8.DiagnosticTypeLOBD:
			data[0] = setBit(data[0], 5, true)
			data[0] = setBit(data[0], 6, true)
		case t8.DiagnosticTypeNone:
			data[0] = setBit(data[0], 5, false)
			data[0] = setBit(data[0], 6, false)
		}

		// on = -----10-
		// off= -----01-
		data[1] = setBit(data[1], 1, !pi01.ClutchStart)
		data[1] = setBit(data[1], 2, pi01.ClutchStart)

		// on = ---10---
		// off= ---01---
		data[1] = setBit(data[1], 3, !pi01.SAI)
		data[1] = setBit(data[1], 4, pi01.SAI)

		// high= -01-----
		// low = -10-----
		data[1] = setBit(data[1], 5, pi01.HighOutput)
		data[1] = setBit(data[1], 6, !pi01.HighOutput)

		if err := gm.WriteDataByIdentifier(ctx, 0x01, data); err != nil {
			t.err(fmt.Errorf("Error writing PI 0x01: %w", err))
			return
		}

		if err := gm.DeviceControl(ctx, 0x16); err != nil {
			t.err(fmt.Errorf("Error performing device control 0x16: %w", err))
			return
		}
	}()
	if err := cl.Wait(ctx); err != nil {
		t.err(err)
	}
}

func setBit(value byte, bitPosition uint, state bool) byte {
	if state {
		return value | (1 << bitPosition)
	}
	return value & ^(1 << bitPosition)
}

func (t *EditParameters) SetVIN(vin string) {
	_ = t.vin.Set(vin)
}

func (t *EditParameters) GetVIN() (string, error) {
	return t.vin.Get()
}

func (t *EditParameters) SetE85Percent(e85percent string) {
	_ = t.e85Percent.Set(e85percent)
}

func (t *EditParameters) GetE85Percent() (string, error) {
	return t.e85Percent.Get()
}

func (t *EditParameters) SetTopSpeed(topSpeed string) {
	_ = t.topSpeed.Set(topSpeed)
}

func (t *EditParameters) GetTopSpeed() (string, error) {
	return t.topSpeed.Get()
}

func (t *EditParameters) SetOilQuality(oilQuality string) {
	_ = t.oilQuality.Set(oilQuality)
}

func (t *EditParameters) GetOilQuality() (string, error) {
	return t.oilQuality.Get()
}

func (t *EditParameters) SetDiagnosticType(diagType string) {
	_ = t.diagnosticType.Set(diagType)
}

func (t *EditParameters) GetDiagnosticType() (string, error) {
	return t.diagnosticType.Get()
}

func (t *EditParameters) SetTankType(tankType string) {
	_ = t.tankType.Set(tankType)
}

func (t *EditParameters) GetTankType() (string, error) {
	return t.tankType.Get()
}

func (t *EditParameters) SetConvertible(convertible bool) {
	_ = t.convertible.Set(convertible)
}

func (t *EditParameters) GetConvertible() (bool, error) {
	return t.convertible.Get()
}

func (t *EditParameters) SetSAI(sai bool) {
	_ = t.sai.Set(sai)
}

func (t *EditParameters) GetSAI() (bool, error) {
	return t.sai.Get()
}

func (t *EditParameters) SetHighOutput(highOutput bool) {
	_ = t.highOutput.Set(highOutput)
}

func (t *EditParameters) GetHighOutput() (bool, error) {
	return t.highOutput.Get()
}

func (t *EditParameters) SetBioPower(bioPower bool) {
	_ = t.bioPower.Set(bioPower)
}

func (t *EditParameters) GetBioPower() (bool, error) {
	return t.bioPower.Get()
}

func (t *EditParameters) SetClutchStart(clutchStart bool) {
	_ = t.clutchStart.Set(clutchStart)
}

func (t *EditParameters) GetPI01Data() (t8.PI01Data, error) {
	var out t8.PI01Data

	diagType, err := t.diagnosticType.Get()
	if err != nil {
		log.Println("Error:", err)
		return out, err
	}
	out.DiagnosticType = t8.DiagnosticTypeFromString(diagType)

	tankType, err := t.tankType.Get()
	if err != nil {
		log.Println("Error:", err)
		return out, err
	}
	out.TankType = t8.TankTypeFromString(tankType)

	convertible, err := t.convertible.Get()
	if err != nil {
		log.Println("Error:", err)
		return out, err
	}
	out.Convertible = convertible

	sai, err := t.sai.Get()
	if err != nil {
		log.Println("Error:", err)
		return out, err
	}
	out.SAI = sai

	highOutput, err := t.highOutput.Get()
	if err != nil {
		log.Println("Error:", err)
		return out, err
	}
	out.HighOutput = highOutput

	bioPower, err := t.bioPower.Get()
	if err != nil {
		log.Println("Error:", err)
		return out, err
	}
	out.BioPower = bioPower

	clutchStart, err := t.clutchStart.Get()
	if err != nil {
		log.Println("Error:", err)
		return out, err
	}
	out.ClutchStart = clutchStart

	return out, nil
}

const (
	pidRPMLimiter  = 0x29
	pidOilQuality  = 0x25
	pidTopSpeed    = 0x02
	pidVIN         = 0x90
	dpidE85Percent = 0x7A
)

type T8 struct {
	gm *gmlan.Client
}

func (t *T8) GetOilQuality(ctx context.Context) (float64, error) {
	resp, err := t.gm.ReadDataByIdentifier(ctx, pidOilQuality)
	if err != nil {
		return 0, err
	}
	quality := binary.BigEndian.Uint32(resp)
	return float64(quality) / 256, nil
}

func (t *T8) SetOilQuality(ctx context.Context, quality float64) error {
	return t.gm.WriteDataByIdentifierUint32(ctx, pidOilQuality, uint32(quality*256))
}

func (t *T8) GetTopSpeed(ctx context.Context) (uint16, error) {
	resp, err := t.gm.ReadDataByIdentifierUint16(ctx, pidTopSpeed)
	if err != nil {
		return 0, fmt.Errorf("GetTopSpeed[1]: %w", err)
	}
	speed := resp / 10
	return speed, nil
}

func (t *T8) SetTopSpeed(ctx context.Context, speed uint16) error {
	speed *= 10
	return t.gm.WriteDataByIdentifierUint16(ctx, pidTopSpeed, speed)
}

func (t *T8) GetRPMLimiter(ctx context.Context) (uint16, error) {
	return t.gm.ReadDataByIdentifierUint16(ctx, pidRPMLimiter)

}

func (t *T8) SetRPMLimit(ctx context.Context, limit uint16) error {
	return t.gm.WriteDataByIdentifierUint16(ctx, pidRPMLimiter, limit)
}

func (t *T8) GetVehicleVIN(ctx context.Context) (string, error) {
	return t.gm.ReadDataByIdentifierString(ctx, pidVIN)
}

func (t *T8) SetVehicleVIN(ctx context.Context, vin string) error {
	if len(vin) != 17 {
		return errors.New("invalid vin length")
	}
	return t.gm.WriteDataByIdentifier(ctx, pidVIN, []byte(vin))
}

func (t *T8) GetE85Percent(ctx context.Context) (float64, error) {
	resp, err := t.gm.ReadDataByPacketIdentifier(ctx, 0x01, dpidE85Percent)
	if err != nil {
		return 0, err
	}
	val := binary.LittleEndian.Uint32(resp[2:6])
	return float64(val), nil
}

func (t *T8) SetE85Percent(ctx context.Context, percent float64) error {
	val := uint32(percent)
	data := []byte{0x00, byte(val)}

	if err := t.gm.DeviceControlWithCode(ctx, 0x18, data); err != nil {
		return err
	}
	return nil
}
