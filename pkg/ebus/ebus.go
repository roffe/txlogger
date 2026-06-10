package ebus

import (
	"sync"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/bus"
)

var (
	once       sync.Once
	CONTROLLER *bus.Controller[string, float64]
)

const (
	TOPIC_COLORBLINDMODE = "color_blind_mode"
	TOPIC_ECU            = "selected_ecu"
)

func init() {
	once.Do(func() {
		CONTROLLER = bus.NewBus[string, float64]()

		// AirDIFF: m_AirInlet vs the requested air mass. Two instances cover the
		// differing request topic names across ECU types; both publish AirDIFF.
		bus.DIFFAggregator(CONTROLLER, "MAF.m_AirInlet", "m_Request", "AirDIFF")
		bus.DIFFAggregator(CONTROLLER, "MAF.m_AirInlet", "AirMassMast.m_Request", "AirDIFF")
	})
}

func Publish(topic string, data float64) {
	CONTROLLER.Publish(topic, data)
}

func SubscribeFunc(topic string, f func(float64)) func() {
	wrapFN := func(v float64) {
		fyne.Do(func() {
			f(v)
		})
	}
	return CONTROLLER.SubscribeFunc(topic, wrapFN)
}

func SetOnMessage(f func(string, float64)) {
	// CONTROLLER.SetOnMessage(f)
	// noop for now, the bus doesn't support this and we don't need it yet. If we do, we can add it to the bus package and call it here.
}
