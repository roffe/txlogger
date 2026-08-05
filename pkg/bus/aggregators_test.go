package bus

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestDIFFAggregatorPublishesDifference(t *testing.T) {
	b := NewBus[string, float64]()
	DIFFAggregator(b, "first", "second", "out")

	var got []float64
	b.SubscribeFunc("out", func(v float64) { got = append(got, v) })

	b.Publish("first", 5)
	b.Publish("second", 25)

	if len(got) != 1 {
		t.Fatalf("expected one diff, got %v", got)
	}
	if got[0] != 20 { // second - first
		t.Fatalf("diff = %v, want 20", got[0])
	}
}

// A diff is published only once both inputs have a fresh value; a single input
// updating must not emit anything.
func TestDIFFAggregatorWaitsForBothInputs(t *testing.T) {
	b := NewBus[string, float64]()
	DIFFAggregator(b, "first", "second", "out")

	var count int
	b.SubscribeFunc("out", func(float64) { count++ })

	b.Publish("first", 1)
	b.Publish("first", 2)
	b.Publish("first", 3)

	if count != 0 {
		t.Fatalf("expected no emission with only one input, got %d", count)
	}

	b.Publish("second", 10)
	if count != 1 {
		t.Fatalf("expected one emission once both inputs seen, got %d", count)
	}
}

// State resets after each emission: a new diff requires fresh values on both
// inputs again, not just one.
func TestDIFFAggregatorResetsAfterEmit(t *testing.T) {
	b := NewBus[string, float64]()
	DIFFAggregator(b, "first", "second", "out")

	var got []float64
	b.SubscribeFunc("out", func(v float64) { got = append(got, v) })

	b.Publish("first", 1)
	b.Publish("second", 4) // emits 3
	b.Publish("second", 9) // no second emit: first is stale
	b.Publish("first", 2)  // emits 9 - 2 = 7

	want := []float64{3, 7}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDIFFAggregatorUnsubscribeStops(t *testing.T) {
	b := NewBus[string, float64]()
	unsub := DIFFAggregator(b, "first", "second", "out")

	var count int
	b.SubscribeFunc("out", func(float64) { count++ })

	unsub()
	unsub() // idempotent

	b.Publish("first", 1)
	b.Publish("second", 2)

	if count != 0 {
		t.Fatalf("expected no emissions after unsubscribe, got %d", count)
	}
}

// Two aggregators sharing an input topic and output topic keep independent
// state, mirroring the AirDIFF wiring in package ebus.
func TestDIFFAggregatorIndependentStateOnSharedTopics(t *testing.T) {
	b := NewBus[string, float64]()
	DIFFAggregator(b, "shared", "a", "out")
	DIFFAggregator(b, "shared", "b", "out")

	var got []float64
	b.SubscribeFunc("out", func(v float64) { got = append(got, v) })

	b.Publish("shared", 10) // primes the shared input of both aggregators
	b.Publish("a", 30)      // first aggregator emits 20
	b.Publish("b", 100)     // second aggregator emits 90

	want := []float64{20, 90}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// The aggregator must stay race-free when its inputs are published from
// multiple goroutines; meaningful only under -race.
func TestDIFFAggregatorConcurrentPublish(t *testing.T) {
	b := NewBus[string, float64]()
	DIFFAggregator(b, "first", "second", "out")

	var emitted atomic.Int64
	b.SubscribeFunc("out", func(float64) { emitted.Add(1) })

	var wg sync.WaitGroup
	for _, topic := range []string{"first", "second"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 1000 {
				b.Publish(topic, float64(i))
			}
		}()
	}
	wg.Wait()

	// Exact count is non-deterministic under interleaving; just confirm the
	// aggregator made progress without racing.
	if emitted.Load() == 0 {
		t.Fatal("expected at least one diff emission")
	}
}
