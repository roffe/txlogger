package gauge

import (
	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/cbar"
	"github.com/roffe/txlogger/pkg/widgets/dial"
	"github.com/roffe/txlogger/pkg/widgets/dualdial"
	"github.com/roffe/txlogger/pkg/widgets/hbar"
	"github.com/roffe/txlogger/pkg/widgets/vbar"
)

func New(cfg widgets.GaugeConfig) (fyne.Widget, []func()) {
	switch cfg.Type {
	case "Dial":
		dial := dial.New(cfg)
		cancel := ebus.SubscribeFunc(cfg.SymbolName, dial.SetValue)
		return dial, []func(){cancel}
	case "DualDial":
		ddial := dualdial.New(cfg)
		cancel1 := ebus.SubscribeFunc(cfg.SymbolName, ddial.SetValue)
		cancel2 := ebus.SubscribeFunc(cfg.SymbolNameSecondary, ddial.SetValue2)
		return ddial, []func(){cancel1, cancel2}
	case "VBar":
		vb := vbar.New(cfg)
		cancel := ebus.SubscribeFunc(cfg.SymbolName, vb.SetValue)
		return vb, []func(){cancel}
	case "HBar":
		hb := hbar.New(cfg)
		cancel := ebus.SubscribeFunc(cfg.SymbolName, hb.SetValue)
		return hb, []func(){cancel}
	case "CBar":
		cb := cbar.New(cfg)
		cancel := ebus.SubscribeFunc(cfg.SymbolName, cb.SetValue)
		return cb, []func(){cancel}
	}
	return nil, nil
}
