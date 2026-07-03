package bus

import (
	"sync"

	"github.com/roffe/txlogger/pkg/common"
)

// DIFFAggregator subscribes to the first and second topics and, once both have
// produced a value, publishes their difference (second - first) to the output
// topic. The internal state resets after each emission, so a fresh value must
// arrive on both inputs before the next diff is published.
//
// Unlike the bus itself, Publish may be called concurrently from several
// goroutines, so the aggregator guards its own state with a mutex. The diff is
// published outside the lock to avoid stalling other publishers and to keep the
// output safe to feed back into the bus.
//
// The returned unsubscribe function removes both input subscriptions; calling
// it more than once is safe and has no further effect.
func DIFFAggregator[K comparable, V common.Number](c *Controller[K, V], first, second, output K) (unsubscribe func()) {
	var (
		mu            sync.Mutex
		firstUpdated  bool
		secondUpdated bool
		firstValue    V
		secondValue   V
	)

	// combine reports the diff when both inputs are fresh, resetting state so the
	// next diff waits for new values. Callers must hold mu.
	combine := func() (V, bool) {
		if firstUpdated && secondUpdated {
			diff := secondValue - firstValue
			firstUpdated, secondUpdated = false, false
			return diff, true
		}
		var zero V
		return zero, false
	}

	stopFirst := c.SubscribeFunc(first, func(v V) {
		mu.Lock()
		firstValue = v
		firstUpdated = true
		diff, ok := combine()
		mu.Unlock()
		if ok {
			c.Publish(output, diff)
		}
	})

	stopSecond := c.SubscribeFunc(second, func(v V) {
		mu.Lock()
		secondValue = v
		secondUpdated = true
		diff, ok := combine()
		mu.Unlock()
		if ok {
			c.Publish(output, diff)
		}
	})

	var once sync.Once
	return func() {
		once.Do(func() {
			stopFirst()
			stopSecond()
		})
	}
}
