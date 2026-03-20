package t8

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/txlogger/pkg/ecu/t8/t8file"
	"github.com/roffe/txlogger/pkg/ecu/t8legion"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
	"github.com/roffe/txlogger/pkg/ecu/t8util"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

func (t *Client) CheckIfCanChangeVIN(ctx context.Context) error {
	if err := t.gm.RequestSecurityAccess(ctx, 0x01, 1, t8sec.CalculateAccessKey); err != nil {
		return err
	}
	currentVIN, err := t.gm.ReadDataByIdentifier(ctx, pidVIN)
	if err != nil {
		return err
	}
	if err = t.gm.WriteDataByIdentifier(ctx, pidVIN, currentVIN); err != nil {
		return err
	}
	return nil
}

func (t *Client) MarryECU(ctx context.Context, pin string) error {
	t.cfg.OnMessage("Marry ECU")
	if err := t.CheckIfCanChangeVIN(ctx); err != nil {
		t.cfg.OnMessage("Virginize ECU by pin stored in ECU")
		if err := t.legion.Bootstrap(ctx, false); err != nil {
			return err
		}

		bin, err := t.legion.ReadFlashRange(ctx, t8legion.EcuByte_T8, 0x4000, 0x8000)
		if err != nil {
			return err
		}
		if err := t.ResetECU(ctx); err != nil {
			return err
		}
		tf := new(t8file.T8Header)
		tf.DecodeExtraInfo(bin)
		t.cfg.OnMessage("PIN stored in ECU: " + tf.PIN() + ", security access delay 30s")
		time.Sleep(30 * time.Second)

		pin := []byte(tf.PIN())
		pinCmd := []byte{0x00}
		pinCmd = append(pinCmd, pin...)

		if err := t.gm.RequestSecurityAccess(ctx, 0x01, 1, t8sec.CalculateAccessKey); err != nil {
			return err
		}
		t.gm.DeviceControlWithCode(ctx, 0x60, pinCmd)
		t.gm.DeviceControlWithCode(ctx, 0x6e, pinCmd)
	}

	if err := t.CheckIfCanChangeVIN(ctx); err != nil {
		t.cfg.OnMessage("Virginize by pin stored in ECU failed, trying by flashing T8 NVDM")
		nvdm := fyne.CurrentApp().Preferences().BoolWithFallback(settings.PrefsNvdm, false)
		if !nvdm {
			return errors.New("System partition not unlocked, try again with checked this option")
		}
		// ---------- Virginize ECU -----------------
		nvdmBytes := t8util.GetVirginNVDM()
		fmask := uint64(0b110) //format nvdm

		if err := t.legion.Bootstrap(ctx, false); err != nil {
			return err
		}
		if err := t.legion.EraseFlash(ctx, t8legion.EcuByte_T8, fmask); err != nil {
			return err
		}
		if err := t.legion.WriteFlash(ctx, t8legion.EcuByte_T8, 0x008000, nvdmBytes, fmask); err != nil {
			return err
		}
		status, err := t.legion.VerifyFlash(ctx, nvdmBytes, t8legion.EcuByte_T8, fmask)
		if err != nil {
			return err
		}
		if !status {
			return errors.New("nvdm verification failed")
		}
		if err = t.legion.MarryMCP(ctx); err != nil {
			return err
		}
		t.cfg.OnMessage("NVDM virginized")

		time.Sleep(200 * time.Millisecond)

		t.cfg.OnMessage("Reset ECU")
		if err := t.ResetECU(ctx); err != nil {
			return err
		}
		time.Sleep(1 * time.Second)
	}

	// ---------- Marry -----------------
	t.cfg.OnMessage("Clear ECU DTC's")
	_ = t.gm.TesterPresentNoResponseAllowed()
	time.Sleep(25 * time.Millisecond)
	if err := t.gm.InitiateDiagnosticOperation(ctx, gmlan.LEV_DADTC); err != nil {
		return err
	}
	if err := t.gm.ClearDiagnosticInformation(ctx, 0x7DF); err != nil {
		return err
	}
	_ = t.gm.ReturnToNormalMode(ctx)

	t.cfg.OnMessage("Clear CIM DTC's")

	if err := t.gmc.ClearDiagnosticInformation(ctx, 0x245); err != nil {
		return err
	}
	_ = t.gm.TesterPresentNoResponseAllowed()
	time.Sleep(200 * time.Millisecond)
	currentVIN, err := t.gm.ReadDataByIdentifier(ctx, pidVIN)
	if err != nil {
		return err
	}
	t.cfg.OnMessage("Current VIN ECU: " + strings.ReplaceAll(string(currentVIN[:]), "\x00", ""))

	t.cfg.OnMessage("Getting security access to CIM")
	if err := t.gmc.RequestSecurityAccess(ctx, gmlan.AccessLevel01, 0, t8sec.CalculateKeyForCIM); err != nil {
		return err
	}
	t.cfg.OnMessage("Security access to CIM OK")

	currentVinCim, err := t.gmc.ReadDataByIdentifier(ctx, 0x90)
	if err != nil {
		return err
	}
	t.cfg.OnMessage("Current VIN CIM: " + strings.ReplaceAll(string(currentVinCim[:]), "\x00", ""))

	if err = t.gm.WriteDataByIdentifier(ctx, pidVIN, currentVinCim); err != nil {
		return err
	}
	t.cfg.OnMessage("VIN programed to ECU")

	t.cfg.OnMessage("Sending Pin to CIM")
	if err := t.gmc.WriteDataByIdentifier(ctx, 0x60, []byte(pin)); err != nil {
		return fmt.Errorf("error writing PIN 0x01: %w", err)
	}

	time.Sleep(1 * time.Second)
	_ = t.gm.TesterPresentNoResponseAllowed()

	t.cfg.OnMessage("Getting married")
	if err := t.gmc.WriteDataByIdentifier(ctx, 0x63, []byte{}); err != nil {
		return fmt.Errorf("marry faild badly 0x01: %w", err)
	}

	t.cfg.OnMessage("Waiting for marry finish")
	for range 5 {
		_ = t.gm.TesterPresentNoResponseAllowed()
		time.Sleep(1 * time.Second)
	}

	t.cfg.OnMessage("Getting security access to ECU")
	err = t.gm.RequestSecurityAccess(ctx, gmlan.AccessLevel01, 0, t8sec.CalculateAccessKey)
	if err != nil {
		t.cfg.OnMessage(err.Error())
		return err
	}
	t.cfg.OnMessage("Security access to ECU OK")

	currentVIN, err = t.gm.ReadDataByIdentifier(ctx, pidVIN)
	t.cfg.OnMessage("Current VIN ECU: " + strings.ReplaceAll(string(currentVIN[:]), "\x00", ""))
	if err != nil {
		return err
	}
	_ = t.gm.TesterPresentNoResponseAllowed()
	t.cfg.OnMessage("Setting Oil to 50%, Speed limiter to 240, E85 to 10%")
	if err := t.SetOilQuality(ctx, 50); err != nil {
		return err
	}
	if err := t.SetTopSpeed(ctx, 240); err != nil {
		return err
	}
	if err := t.SetE85Percent(ctx, 10.0); err != nil {
		t.cfg.OnMessage("setting E85 to 10% failed")
	}

	t.gmc.ReturnToNormalMode(ctx)
	t.gm.ReturnToNormalMode(ctx)
	t.cfg.OnMessage("Done")
	return nil
}
