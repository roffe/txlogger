package combinedlogplayer

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/eventbus"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets/dashboard"
	"github.com/roffe/txlogger/pkg/widgets/logplayer"
)

var _ fyne.Focusable = (*Widget)(nil)

type Widget struct {
	widget.BaseWidget
	db *dashboard.Dashboard
	lp *logplayer.Logplayer

	cancelFuncs []func()
	closeOnce   sync.Once
}

type CombinedLogplayerConfig struct {
	Logfile logfile.Logfile
	DBcfg   *dashboard.Config
}

func New(cfg *CombinedLogplayerConfig) *Widget {

	cp := &Widget{}

	if cfg.DBcfg.AirDemToString == nil {
		cfg.DBcfg.AirDemToString = func(f float64) string {
			return "Undefined"
		}
	}
	bus := eventbus.New(eventbus.DefaultConfig)

	db := dashboard.NewDashboard(cfg.DBcfg)

	for _, name := range db.GetMetricNames() {
		cancel := bus.SubscribeFunc(name, func(f float64) {
			fyne.Do(func() {
				db.SetValue(name, f)
			})
		})
		cp.cancelFuncs = append(cp.cancelFuncs, cancel)
	}

	cp.db = db
	cp.lp = logplayer.New(&logplayer.Config{
		EBus:       bus,
		Logfile:    cfg.Logfile,
		TimeSetter: db.SetTime,
	})

	cp.ExtendBaseWidget(cp)

	return cp
}

func (cp *Widget) FocusGained() {

}

func (cp *Widget) FocusLost() {

}

func (cp *Widget) TypedRune(r rune) {
	cp.lp.TypedRune(r)
}

func (cp *Widget) TypedKey(key *fyne.KeyEvent) {
	cp.lp.TypedKey(key)
}

func (cp *Widget) Close() {
	cp.closeOnce.Do(func() {
		for _, cancel := range cp.cancelFuncs {
			cancel()
		}
		cp.lp.Close()
		cp.db.Close()
	})
}

func (cp *Widget) CreateRenderer() fyne.WidgetRenderer {
	split := container.NewVSplit(cp.db, cp.lp)
	split.Offset = 0.8
	return widget.NewSimpleRenderer(split)
}
