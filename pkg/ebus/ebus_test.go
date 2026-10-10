package ebus

import (
	"sync"
	"testing"
	"time"
)

// recorder collects the values delivered to a subscription.
type recorder struct {
	mu   sync.Mutex
	vals []float64
}

func (r *recorder) add(v float64) {
	r.mu.Lock()
	r.vals = append(r.vals, v)
	r.mu.Unlock()
}

func (r *recorder) get() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]float64(nil), r.vals...)
}

func subscribe(t *testing.T, topic string) *recorder {
	t.Helper()
	r := &recorder{}
	t.Cleanup(SubscribeFunc(topic, r.add))
	return r
}

func equal(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Publish delivers each value, in order, to every subscriber of that topic and
// to no other topic.
func TestPublishDeliversToTopicSubscribers(t *testing.T) {
	a1 := subscribe(t, "test.topicA")
	a2 := subscribe(t, "test.topicA")
	b := subscribe(t, "test.topicB")

	Publish("test.topicA", 1.5)
	Publish("test.topicA", -2)

	want := []float64{1.5, -2}
	if got := a1.get(); !equal(got, want) {
		t.Fatalf("subscriber 1 got %v, want %v", got, want)
	}
	if got := a2.get(); !equal(got, want) {
		t.Fatalf("subscriber 2 got %v, want %v", got, want)
	}
	if got := b.get(); len(got) != 0 {
		t.Fatalf("other topic received %v", got)
	}
}

// SubscribeFunc callbacks run synchronously on the publishing goroutine, so
// the value is visible as soon as Publish returns, without extra
// synchronisation (the race detector enforces this).
func TestSubscribeFuncRunsSynchronously(t *testing.T) {
	var got float64
	unsub := SubscribeFunc("test.sync", func(v float64) { got = v })
	defer unsub()

	Publish("test.sync", 42)
	if got != 42 {
		t.Fatalf("value not delivered before Publish returned: got %v", got)
	}
}

// The func returned by SubscribeFunc stops delivery to that subscriber only.
func TestSubscribeFuncUnsubscribe(t *testing.T) {
	var stopped []float64
	unsub := SubscribeFunc("test.unsub", func(v float64) { stopped = append(stopped, v) })
	kept := subscribe(t, "test.unsub")

	Publish("test.unsub", 1)
	unsub()
	Publish("test.unsub", 2)

	if !equal(stopped, []float64{1}) {
		t.Fatalf("unsubscribed callback got %v, want [1]", stopped)
	}
	if got := kept.get(); !equal(got, []float64{1, 2}) {
		t.Fatalf("remaining subscriber got %v, want [1 2]", got)
	}
}

// PublishFrame announces a completed log frame on TOPIC_FRAME, carrying the
// frame time as exact Unix milliseconds.
func TestPublishFrameCarriesUnixMilli(t *testing.T) {
	r := subscribe(t, TOPIC_FRAME)

	ts := time.Date(2026, 10, 10, 12, 34, 56, 789_654_321, time.UTC)
	PublishFrame(ts)

	got := r.get()
	if len(got) != 1 {
		t.Fatalf("got %d frame events, want 1", len(got))
	}
	if got[0] != float64(ts.UnixMilli()) {
		t.Fatalf("frame value = %v, want %d", got[0], ts.UnixMilli())
	}
	if back := time.UnixMilli(int64(got[0])); !back.Equal(ts.Truncate(time.Millisecond)) {
		t.Fatalf("round trip = %v, want %v", back, ts.Truncate(time.Millisecond))
	}
}

// The package wires AirDIFF as requested air mass minus measured inlet air,
// for both request topic names used by the different ECU types.
func TestAirDIFFAggregation(t *testing.T) {
	diff := subscribe(t, "AirDIFF")

	// T7 style request topic.
	Publish("MAF.m_AirInlet", 400)
	Publish("m_Request", 520)

	// T8 style request topic.
	Publish("MAF.m_AirInlet", 300)
	Publish("AirMassMast.m_Request", 250)

	want := []float64{120, -50}
	if got := diff.get(); !equal(got, want) {
		t.Fatalf("AirDIFF = %v, want %v", got, want)
	}
}
