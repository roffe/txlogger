package gauge

import (
	"errors"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/cbar"
	"github.com/roffe/txlogger/pkg/widgets/dial"
	"github.com/roffe/txlogger/pkg/widgets/dualdial"
	"github.com/roffe/txlogger/pkg/widgets/hbar"
	"github.com/roffe/txlogger/pkg/widgets/vbar"
	"github.com/roffe/txlogger/pkg/widgets/widebandgauge"
)

func New(cfg *widgets.GaugeConfig) (fyne.CanvasObject, func(), error) {
	switch cfg.Type {
	case "Dial":
		dial := dial.New(cfg)
		cancel := ebus.SubscribeFunc(cfg.SymbolName, dial.SetValue)
		return dial, cancel, nil
	case "DualDial":
		ddial := dualdial.New(cfg)
		cancel1 := ebus.SubscribeFunc(cfg.SymbolName, ddial.SetValue)
		cancel2 := ebus.SubscribeFunc(cfg.SymbolNameSecondary, ddial.SetValue2)
		cancelFn := func() {
			cancel1()
			cancel2()
		}
		return ddial, cancelFn, nil
	case "VBar":
		vb := vbar.New(cfg)
		cancel := ebus.SubscribeFunc(cfg.SymbolName, vb.SetValue)
		return vb, cancel, nil
	case "HBar":
		hb := hbar.New(cfg)
		cancel := ebus.SubscribeFunc(cfg.SymbolName, hb.SetValue)
		return hb, cancel, nil
	case "CBar":
		cb := cbar.New(cfg)
		cancel := ebus.SubscribeFunc(cfg.SymbolName, cb.SetValue)
		return cb, cancel, nil
	case "Wideband":
		wb := widebandgauge.New(cfg)
		cancel := ebus.SubscribeFunc(cfg.SymbolName, wb.SetValue)
		return wb, cancel, nil
	}
	return nil, nil, errors.New("unknown gauge type")
}
