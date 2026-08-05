package j1979diag

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/dtc/wis"
	"github.com/roffe/txlogger/pkg/j1979"
)

// ---- Mode $01: current data ----

func (w *Widget) readCurrentData(ctx context.Context, cl *j1979.Client) error {
	pids, err := cl.SupportedPIDs(ctx)
	if err != nil {
		return err
	}
	lines, err := sweep(ctx, cl, pids)
	if err != nil {
		return err
	}
	w.setText(w.liveOut, fmt.Sprintf("Supported PIDs: % X\n\n%s", pids, strings.Join(lines, "\n")))
	return nil
}

func (w *Widget) liveLoop(ctx context.Context, cl *j1979.Client) error {
	pids, err := cl.SupportedPIDs(ctx)
	if err != nil {
		return err
	}
	for {
		lines := make([]string, 0, len(pids))
		var answered bool
		var lastErr error
		for _, pid := range pids {
			if pid%0x20 == 0 {
				continue // supported-PID range masks, nothing to display
			}
			v, err := cl.ReadPIDValue(ctx, pid)
			if err != nil {
				if ctx.Err() != nil {
					return nil // stopped mid-sweep
				}
				// tolerate spurious misses (a dropped frame costs one
				// request timeout); only a fully dead sweep aborts live
				lastErr = err
				lines = append(lines, fmt.Sprintf("PID $%02X: no response", pid))
				continue
			}
			answered = true
			lines = append(lines, v.String())
		}
		if !answered {
			return lastErr
		}
		w.setText(w.liveOut, strings.Join(lines, "\n"))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func sweep(ctx context.Context, cl *j1979.Client, pids []byte) ([]string, error) {
	lines := make([]string, 0, len(pids))
	for _, pid := range pids {
		if pid%0x20 == 0 {
			continue // supported-PID range masks, nothing to display
		}
		v, err := cl.ReadPIDValue(ctx, pid)
		if err != nil {
			return nil, err
		}
		lines = append(lines, v.String())
	}
	return lines, nil
}

// ---- Modes $03/$07 + $04: DTCs ----

func (w *Widget) readDTCs(ctx context.Context, cl *j1979.Client) error {
	var sb strings.Builder
	if ms, err := cl.MonitorStatus(ctx); err == nil {
		sb.WriteString(ms.String() + "\n\n")
	}
	stored, err := cl.StoredDTCs(ctx)
	if err != nil {
		return err
	}
	sb.WriteString("Stored DTCs (mode $03):\n" + dtcLines(stored))
	pending, err := cl.PendingDTCs(ctx)
	if err != nil {
		return err
	}
	sb.WriteString("\nPending DTCs (mode $07):\n" + dtcLines(pending))
	w.setText(w.dtcOut, sb.String())
	return nil
}

func (w *Widget) clearDTCs(ctx context.Context, cl *j1979.Client) error {
	if err := cl.ClearDTCs(ctx); err != nil {
		return err
	}
	w.log("J1979: diagnostic information cleared")
	w.setText(w.dtcOut, "Diagnostic information cleared.")
	return nil
}

func dtcLines(codes []dtc.DTC) string {
	if len(codes) == 0 {
		return "  none\n"
	}
	var sb strings.Builder
	for _, c := range codes {
		info := c.Info()
		name := info.Name
		if name == "" {
			// same WIS models the DTC reader uses for T7
			if res := wis.Search(c.Code, "9400", "9600"); len(res) > 0 {
				name = res[0].Title
			}
		}
		sb.WriteString("  " + c.Code)
		if name != "" {
			sb.WriteString(" - " + name)
		}
		sb.WriteByte('\n')
		if info.Description != "" {
			sb.WriteString("      " + info.Description + "\n")
		}
	}
	return sb.String()
}

// ---- Mode $02: freeze frame ----

func (w *Widget) readFreezeFrame(ctx context.Context, cl *j1979.Client) error {
	ff, err := cl.FreezeFrame(ctx, 0)
	if err != nil {
		return err
	}
	if ff == nil {
		w.setText(w.ffOut, "No freeze frame stored.")
		return nil
	}
	var sb strings.Builder
	sb.WriteString("Freeze frame DTC: " + ff.DTC.Code)
	if name := ff.DTC.Info().Name; name != "" {
		sb.WriteString(" - " + name)
	}
	sb.WriteString("\n\n")
	for _, v := range ff.Values {
		sb.WriteString(v.String() + "\n")
	}
	w.setText(w.ffOut, sb.String())
	return nil
}

// ---- Mode $05: O2 sensor tests ----

var o2TIDs = map[byte]struct {
	name  string
	volts bool
}{
	0x01: {"Rich→lean threshold voltage", true},
	0x02: {"Lean→rich threshold voltage", true},
	0x03: {"Low voltage for switch time", true},
	0x04: {"High voltage for switch time", true},
	0x05: {"Rich→lean switch time", false},
	0x06: {"Lean→rich switch time", false},
	0x07: {"Minimum sensor voltage", true},
	0x08: {"Maximum sensor voltage", true},
	0x09: {"Transition time", false},
}

func (w *Widget) readO2Tests(ctx context.Context, cl *j1979.Client) error {
	var sb strings.Builder
	for _, s := range []struct {
		id   byte
		name string
	}{{1, "front"}, {2, "rear"}} {
		fmt.Fprintf(&sb, "O2 sensor %d (%s):\n", s.id, s.name)
		tids, err := cl.O2SupportedTests(ctx, s.id)
		if err != nil {
			if ctx.Err() != nil {
				return err
			}
			fmt.Fprintf(&sb, "  %s\n\n", err)
			continue
		}
		if len(tids) == 0 {
			sb.WriteString("  no supported tests\n")
		}
		for _, tid := range tids {
			if tid%0x20 == 0 {
				continue // supported-test range masks
			}
			r, err := cl.O2SensorTest(ctx, tid, s.id)
			if err != nil {
				if ctx.Err() != nil {
					return err
				}
				fmt.Fprintf(&sb, "  TID $%02X: %s\n", tid, err)
				continue
			}
			def, known := o2TIDs[tid]
			switch {
			case known && def.volts:
				fmt.Fprintf(&sb, "  TID $%02X %s: %.3f V (min %.3f, max %.3f)\n",
					tid, def.name, j1979.Volts(r.Value), j1979.Volts(r.Min), j1979.Volts(r.Max))
			case known:
				fmt.Fprintf(&sb, "  TID $%02X %s: %d (min %d, max %d)\n", tid, def.name, r.Value, r.Min, r.Max)
			default:
				fmt.Fprintf(&sb, "  TID $%02X: %d (min %d, max %d)\n", tid, r.Value, r.Min, r.Max)
			}
		}
		sb.WriteByte('\n')
	}
	w.setText(w.testOut, sb.String())
	return nil
}

// ---- Mode $06: on-board monitoring test results ----

// T7 test IDs beyond the generic continuous monitors 01-08.
var mode6TIDs = map[byte]string{
	0x13: "EVAP leak",
	0x14: "Tank pressure",
	0x15: "Brake light",
	0x19: "Catalyst",
	0x1D: "Cooling system",
	0x1E: "Tank adaptation",
}

func (w *Widget) readTestResults(ctx context.Context, cl *j1979.Client) error {
	adv, err := cl.SupportedTests(ctx)
	if err != nil {
		return err
	}
	// The advertised bitmask is misaligned in the firmware (see
	// j1979.T7TestIDs), so probe the union of both sets.
	tids := slices.Clone(adv)
	for _, tid := range j1979.T7TestIDs {
		if !slices.Contains(tids, tid) {
			tids = append(tids, tid)
		}
	}
	slices.Sort(tids)

	var sb strings.Builder
	sb.WriteString("On-board monitoring test results (mode $06):\n")
	var noResult []byte
	for _, tid := range tids {
		results, err := cl.TestResults(ctx, tid)
		if err != nil {
			if ctx.Err() != nil {
				return err
			}
			noResult = append(noResult, tid)
			continue
		}
		for _, r := range results {
			line := r.String()
			if name, ok := mode6TIDs[r.TestID]; ok {
				line += " — " + name
			}
			sb.WriteString("  " + line + "\n")
		}
	}
	if len(noResult) > 0 {
		parts := make([]string, len(noResult))
		for i, tid := range noResult {
			parts[i] = fmt.Sprintf("$%02X", tid)
		}
		sb.WriteString("\nNo result for TIDs: " + strings.Join(parts, " ") + "\n")
	}
	w.setText(w.testOut, sb.String())
	return nil
}

// ---- Mode $08: control of on-board system ----

func (w *Widget) runEvapTest(ctx context.Context, cl *j1979.Client) error {
	data, err := cl.ControlTest(ctx, 0x01)
	if err != nil {
		return err
	}
	status := "EVAP purge valve test did not start"
	if data[0] == 0 {
		status = "EVAP purge valve closed, leak test running"
	}
	w.setText(w.testOut, fmt.Sprintf("%s (response % X)", status, data))
	return nil
}

// ---- Mode $09: vehicle information ----

func (w *Widget) readVehicleInfo(ctx context.Context, cl *j1979.Client) error {
	var sb strings.Builder
	if vin, err := cl.VIN(ctx); err == nil {
		sb.WriteString("VIN:            " + vin + "\n")
	} else if ctx.Err() != nil {
		return err
	} else {
		sb.WriteString("VIN:            " + err.Error() + "\n")
	}
	if cal, err := cl.CalibrationID(ctx); err == nil {
		sb.WriteString("Calibration ID: " + cal + "\n")
	} else if ctx.Err() != nil {
		return err
	} else {
		sb.WriteString("Calibration ID: " + err.Error() + "\n")
	}
	if cvn, err := cl.CVN(ctx); err == nil {
		fmt.Fprintf(&sb, "CVN:            %X\n", cvn)
	} else if ctx.Err() != nil {
		return err
	} else {
		sb.WriteString("CVN:            " + err.Error() + "\n")
	}
	w.setText(w.infoOut, sb.String())
	return nil
}
