package dtcreader

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/t5can"
)

func (d *DTCReader) getT5DTCSymbols() ([]*symbol.Symbol, error) {
	var symbolsToCheck []*symbol.Symbol
	fw := d.getFW()
	if fw == nil {
		return nil, fmt.Errorf("it is required to load a binary file to read DTCs")
	}
	for _, s := range fw.Symbols() {
		if strings.HasSuffix(s.Name, "_error") || strings.HasSuffix(s.Name, "_fel") {
			// log.Println(s.Name, s.Length, s.SramOffset)
			if s.Length == 1 {
				symbolsToCheck = append(symbolsToCheck, s)
			}
		}
	}
	sort.Slice(symbolsToCheck, func(i, j int) bool {
		return strings.ToLower(symbolsToCheck[i].Name) < strings.ToLower(symbolsToCheck[j].Name)
	})
	return symbolsToCheck, nil
}

func (d *DTCReader) readT5DTCS(ctx context.Context, cl *gocan.Client) {
	symbolsToCheck, err := d.getT5DTCSymbols()
	if err != nil {
		d.err(err)
		return
	}
	t5 := t5can.NewClient(cl)
	var dtcs []dtc.DTC
	for _, sym := range symbolsToCheck {
		val, err := t5.ReadRam(ctx, sym.SramOffset, uint32(sym.Length))
		if err != nil {
			d.err(fmt.Errorf("error reading %s: %w", sym.Name, err))
			return
		}
		// log.Printf("%s % 02X", sym.Name, val)
		if len(val) != 1 {
			d.err(fmt.Errorf("unexpected DTC length for symbol %s: %d", sym.Name, len(val)))
			return
		}
		value := val[0]
		if value == 0 {
			continue
		}
		dtcs = append(dtcs, dtc.DTC{
			ECU:    dtc.ECU_T5,
			Code:   sym.Name,
			Status: value,
		})
	}

	d.dtcs = dtcs
	fyne.Do(func() {
		d.Refresh()
	})
}

func (d *DTCReader) clearT5DTCS(ctx context.Context, cl *gocan.Client) {
	symbolsToClear, err := d.getT5DTCSymbols()
	if err != nil {
		d.err(err)
		return
	}
	t5 := t5can.NewClient(cl)
	for _, sym := range symbolsToClear {
		res, err := t5.ReadRam(ctx, sym.SramOffset, uint32(sym.Length))
		if err != nil {
			d.err(fmt.Errorf("error reading %s before clearing: %w", sym.Name, err))
			continue
		}
		if len(res) != 1 {
			d.err(fmt.Errorf("unexpected DTC length for symbol %s: %d", sym.Name, len(res)))
			continue
		}
		if res[0] == 0 {
			continue // already cleared
		}
		if err := t5.WriteRam(ctx, sym.SramOffset, []byte{0x00}); err != nil {
			d.err(fmt.Errorf("error clearing %s: %w", sym.Name, err))
			continue
		}
	}
	d.dtcs = []dtc.DTC{}
	fyne.Do(d.Refresh)
}
