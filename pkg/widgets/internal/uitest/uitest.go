// Package uitest stands in for the GLFW main loop in tests: fyne.Do and
// fyne.DoAndWait run their functions in order on one dedicated goroutine. The
// stock test driver runs them inline on the caller instead, which hides the
// races between publishing goroutines and the UI goroutine that -race should
// catch.
package uitest

import (
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

type app struct {
	fyne.App
	drv *driver
}

func (a *app) Driver() fyne.Driver { return a.drv }

type driver struct {
	fyne.Driver
	mu   sync.Mutex
	q    []func()
	wake chan struct{}
}

func (d *driver) DoFromGoroutine(f func(), wait bool) {
	if !wait {
		d.post(f)
		return
	}
	done := make(chan struct{})
	d.post(func() { f(); close(done) })
	<-done
}

// post queues f without blocking, like Fyne's unbounded function queue, so
// the UI goroutine can post to itself.
func (d *driver) post(f func()) {
	d.mu.Lock()
	d.q = append(d.q, f)
	d.mu.Unlock()
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Start installs a test app whose fyne.Do runs on a dedicated UI goroutine and
// returns onUI, which runs f there and waits for it. Build and lay out widgets
// through onUI, never from the test goroutine. When the test ends, the queue
// is flushed and the loop stops, so stop any publishing goroutines before
// returning.
func Start(t testing.TB) (onUI func(f func())) {
	a := test.NewTempApp(t)
	d := &driver{Driver: a.Driver(), wake: make(chan struct{}, 1)}
	fyne.SetCurrentApp(&app{App: a, drv: d})

	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-d.wake:
			case <-stop:
				return
			}
			for {
				d.mu.Lock()
				batch := d.q
				d.q = nil
				d.mu.Unlock()
				if len(batch) == 0 {
					break
				}
				for _, f := range batch {
					f()
				}
			}
		}
	}()
	t.Cleanup(func() {
		fyne.DoAndWait(func() {}) // runs after everything already queued
		close(stop)
		<-stopped
	})
	return fyne.DoAndWait
}
