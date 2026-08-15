package ebus

import (
	"strconv"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// BenchmarkPublishToLabel is the real logging hot path: a symbol value published
// on the ebus, delivered through the fyne.Do wrapper to a widget that renders
// it. One iteration is one symbol update on screen.
func BenchmarkPublishToLabel(b *testing.B) {
	test.NewTempApp(b)
	l := widget.NewLabel("0.00")
	test.NewTempWindow(b, l)

	unsub := SubscribeFunc("bench.symbol", func(v float64) {
		l.SetText(strconv.FormatFloat(v, 'f', 2, 64))
	})
	defer unsub()

	vals := [...]float64{12.34, 56.78, 90.12, 34.56}
	for _, v := range vals {
		Publish("bench.symbol", v) // warm the measure cache
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Publish("bench.symbol", vals[i%len(vals)])
	}
}

// BenchmarkPublishNoSubscriber is the cost of publishing a symbol nothing
// listens to - the common case, since most logged symbols have no open widget.
func BenchmarkPublishNoSubscriber(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Publish("bench.nolistener", 1.0)
	}
}
