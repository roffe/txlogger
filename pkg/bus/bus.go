// Package bus implements a small, type-safe publish/subscribe message bus.
//
// Topics are keyed by any comparable type K and carry values of type V.
// Subscribers register either a channel (Subscribe) or a callback
// (SubscribeFunc) and receive every value published to their topic until
// they unsubscribe.
//
// The bus is optimised for a publish-heavy, subscription-stable workload: the
// subscriber table is held as an immutable snapshot behind an atomic pointer,
// so Publish reads it lock-free and allocation-free and scales linearly across
// goroutines. Subscribe/Unsubscribe copy the table (copy-on-write), so they
// cost O(topics + subscribers) — cheap when subscriptions change rarely, which
// is the intended use.
//
// Callbacks run synchronously in the publishing goroutine, so they must be
// fast and non-blocking. A slow callback stalls that Publish call; if you need
// blocking work, hand off to your own goroutine or use Subscribe's buffered
// channel.
package bus

import (
	"maps"
	"sync"
	"sync/atomic"
)

type subscriber[V any] struct {
	id uint64
	fn func(V)
}

// Controller is a concurrency-safe pub/sub bus. The zero value is not usable;
// create one with NewBus.
type Controller[K comparable, V any] struct {
	mu     sync.Mutex // serialises writers (Subscribe/Unsubscribe) only
	nextID uint64
	state  atomic.Pointer[map[K][]subscriber[V]]
}

// NewBus creates an empty bus for topics of type K carrying values of type V.
func NewBus[K comparable, V any]() *Controller[K, V] {
	c := &Controller[K, V]{}
	empty := make(map[K][]subscriber[V])
	c.state.Store(&empty)
	return c
}

// SubscribeFunc registers fn to be called for every value published to topic.
// It returns an unsubscribe function that removes the subscription; calling it
// more than once is safe and has no further effect.
//
// fn runs synchronously in the goroutine that calls Publish and must not block.
func (c *Controller[K, V]) SubscribeFunc(topic K, fn func(V)) (unsubscribe func()) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.replace(func(m map[K][]subscriber[V]) {
		m[topic] = append(cloneSlice(m[topic]), subscriber[V]{id: id, fn: fn})
	})
	c.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.unsubscribe(topic, id)
		})
	}
}

// Subscribe registers a buffered channel that receives every value published
// to topic. The returned unsubscribe function removes the subscription and
// closes the channel.
//
// buffer sets the channel's capacity. If the channel is full when a value is
// published, Publish skips this subscriber rather than blocking, so size the
// buffer for your expected burst rate or drain the channel promptly.
func (c *Controller[K, V]) Subscribe(topic K, buffer int) (ch <-chan V, unsubscribe func()) {
	out := make(chan V, buffer)
	stop := c.SubscribeFunc(topic, func(v V) {
		// Non-blocking send: a slow consumer must not stall the publisher.
		select {
		case out <- v:
		default:
		}
	})

	var once sync.Once
	return out, func() {
		once.Do(func() {
			stop()
			close(out)
		})
	}
}

// Publish delivers v to every current subscriber of topic. Callbacks run
// synchronously in the caller's goroutine in an unspecified order. This is the
// hot path: it takes no locks and allocates nothing.
func (c *Controller[K, V]) Publish(topic K, v V) {
	for _, s := range (*c.state.Load())[topic] {
		s.fn(v)
	}
}

func (c *Controller[K, V]) unsubscribe(topic K, id uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replace(func(m map[K][]subscriber[V]) {
		subs := m[topic]
		out := make([]subscriber[V], 0, len(subs))
		for _, s := range subs {
			if s.id != id {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			delete(m, topic)
		} else {
			m[topic] = out
		}
	})
}

// replace builds a shallow copy of the current subscriber table, applies mutate
// to the copy, and atomically swaps it in. Callers must hold c.mu so writers
// don't race each other; readers (Publish) never block. The previous table is
// never mutated, so any in-flight Publish keeps iterating a consistent view.
func (c *Controller[K, V]) replace(mutate func(map[K][]subscriber[V])) {
	old := *c.state.Load()
	next := make(map[K][]subscriber[V], len(old)+1)
	maps.Copy(next, old) // slices are immutable once published; share them
	mutate(next)
	c.state.Store(&next)
}

func cloneSlice[V any](s []subscriber[V]) []subscriber[V] {
	out := make([]subscriber[V], len(s), len(s)+1)
	copy(out, s)
	return out
}
