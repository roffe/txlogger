package ebus

import (
	"sync"
	"time"

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
	// TOPIC_FRAME fires once per completed log frame, carrying the frame's
	// timestamp as Unix milliseconds (float64). Subscribers use it as the frame
	// boundary to sample the latest value of every symbol with a shared, real
	// timestamp (see the live plotter).
	TOPIC_FRAME = "__frame__"
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

// PublishFrame signals that a log frame completed at time t. The timestamp is
// carried as Unix milliseconds, which fits exactly in a float64.
func PublishFrame(t time.Time) {
	CONTROLLER.Publish(TOPIC_FRAME, float64(t.UnixMilli()))
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
