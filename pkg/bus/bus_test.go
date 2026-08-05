package bus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubscribeFuncReceivesPublishes(t *testing.T) {
	b := NewBus[string, int]()
	var got []int
	b.SubscribeFunc("t", func(v int) { got = append(got, v) })

	for _, v := range []int{1, 2, 3} {
		b.Publish("t", v)
	}

	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestPublishToUnknownTopicIsNoop(t *testing.T) {
	b := NewBus[string, int]()
	b.SubscribeFunc("a", func(int) { t.Fatal("subscriber on topic a should not fire") })
	b.Publish("b", 1) // different topic — must not panic or deliver
}

func TestMultipleSubscribersEachReceive(t *testing.T) {
	b := NewBus[string, int]()
	var c1, c2 int
	b.SubscribeFunc("t", func(int) { c1++ })
	b.SubscribeFunc("t", func(int) { c2++ })

	b.Publish("t", 1)
	b.Publish("t", 1)

	if c1 != 2 || c2 != 2 {
		t.Fatalf("each subscriber should see 2 messages, got c1=%d c2=%d", c1, c2)
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := NewBus[string, int]()
	var count int
	unsub := b.SubscribeFunc("t", func(int) { count++ })

	b.Publish("t", 1)
	unsub()
	b.Publish("t", 1)

	if count != 1 {
		t.Fatalf("expected 1 delivery before unsubscribe, got %d", count)
	}
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	b := NewBus[string, int]()
	var count int
	unsub := b.SubscribeFunc("t", func(int) { count++ })

	unsub()
	unsub() // second call must be a safe no-op
	b.Publish("t", 1)

	if count != 0 {
		t.Fatalf("expected no deliveries after unsubscribe, got %d", count)
	}
}

// Unsubscribing one subscriber must not affect the others on the same topic.
func TestUnsubscribeLeavesOthersIntact(t *testing.T) {
	b := NewBus[string, int]()
	var a, c int
	unsubA := b.SubscribeFunc("t", func(int) { a++ })
	b.SubscribeFunc("t", func(int) { c++ })

	unsubA()
	b.Publish("t", 1)

	if a != 0 {
		t.Fatalf("unsubscribed handler fired %d times", a)
	}
	if c != 1 {
		t.Fatalf("remaining handler should fire once, got %d", c)
	}
}

// A Publish already iterating its snapshot must complete against a consistent
// view even if a handler unsubscribes mid-dispatch.
func TestUnsubscribeFromWithinCallback(t *testing.T) {
	b := NewBus[string, int]()
	var unsub func()
	var calls int
	unsub = b.SubscribeFunc("t", func(int) {
		calls++
		unsub() // remove self during dispatch
	})
	b.SubscribeFunc("t", func(int) {}) // a second sub keeps the topic alive

	b.Publish("t", 1)
	b.Publish("t", 1)

	if calls != 1 {
		t.Fatalf("self-unsubscribing handler should fire exactly once, got %d", calls)
	}
}

func TestSubscribeChannelDelivers(t *testing.T) {
	b := NewBus[string, string]()
	ch, unsub := b.Subscribe("t", 4)
	defer unsub()

	b.Publish("t", "hello")
	select {
	case got := <-ch:
		if got != "hello" {
			t.Fatalf("got %q, want %q", got, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel delivery")
	}
}

func TestSubscribeChannelClosesOnUnsubscribe(t *testing.T) {
	b := NewBus[string, int]()
	ch, unsub := b.Subscribe("t", 1)
	unsub()

	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after unsubscribe")
	}
}

// A full channel must not block the publisher; excess messages are dropped.
func TestSubscribeChannelDropsWhenFull(t *testing.T) {
	b := NewBus[string, int]()
	ch, unsub := b.Subscribe("t", 1)
	defer unsub()

	b.Publish("t", 1) // fills the buffer
	b.Publish("t", 2) // dropped, must not block

	if got := <-ch; got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
	select {
	case v := <-ch:
		t.Fatalf("expected no second message, got %d", v)
	default:
	}
}

// Hammer subscribe/unsubscribe/publish concurrently; meaningful only under -race.
func TestConcurrentChurn(t *testing.T) {
	b := NewBus[int, int]()
	var wg sync.WaitGroup
	var delivered atomic.Int64

	const topics = 8
	stop := make(chan struct{})

	// Publishers.
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					for tpc := range topics {
						b.Publish(tpc, 1)
					}
				}
			}
		}()
	}

	// Subscribers churning in and out.
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					for tpc := range topics {
						unsub := b.SubscribeFunc(tpc, func(int) { delivered.Add(1) })
						unsub()
					}
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}
