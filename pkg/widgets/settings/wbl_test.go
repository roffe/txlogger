package settings

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/wbl/aem"
)

// An open dashboard re-points its wideband gauge when the source changes, which
// only works if the change is announced on the bus.
func TestWidebandSourceChangeIsAnnounced(t *testing.T) {
	test.NewApp()
	sw := New(&Config{
		Logger:          func(string) {},
		SelectedEcuFunc: func() string { return "T7" },
	})
	test.NewWindow(sw) // the controls are built in CreateRenderer

	var pulses int
	// subscribe on the controller directly: ebus.SubscribeFunc defers to the
	// Fyne event loop, which is not running here
	defer ebus.CONTROLLER.SubscribeFunc(ebus.TOPIC_WBLSYMBOL, func(float64) { pulses++ })()

	sw.wblSource.SetSelected(aem.ProductString)
	if pulses != 1 {
		t.Fatalf("pulses after selecting %s = %d, want 1", aem.ProductString, pulses)
	}
	if got := sw.GetWidebandSymbolName(); got != datalogger.EXTERNALWBLSYM {
		t.Fatalf("symbol for %s = %q, want %q", aem.ProductString, got, datalogger.EXTERNALWBLSYM)
	}

	sw.wblSource.SetSelected("ECU")
	if pulses != 2 {
		t.Fatalf("pulses after selecting ECU = %d, want 2", pulses)
	}
	if got := sw.GetWidebandSymbolName(); got != "DisplProt.LambdaScanner" {
		t.Fatalf("symbol for ECU on T7 = %q, want DisplProt.LambdaScanner", got)
	}

	sw.wblADscanner.SetChecked(true)
	if pulses != 3 {
		t.Fatalf("pulses after enabling the AD scanner = %d, want 3", pulses)
	}
	if got := sw.GetWidebandSymbolName(); got != datalogger.LAMBDAADSCANNER {
		t.Fatalf("symbol with AD scanner = %q, want %q", got, datalogger.LAMBDAADSCANNER)
	}
}
