package eventbus_test

import (
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/roffe/txlogger/experiments/eventbus"
)

const testTimeout = 2 * time.Second

// TestMain silences the package's drop logging ("publish channel full" etc.),
// which is expected under load and would otherwise flood benchmark output.
func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	code := m.Run()
	log.SetOutput(os.Stderr)
	os.Exit(code)
}

// recvWithin returns the next value from ch, failing if nothing arrives within d.
func recvWithin(t *testing.T, ch <-chan float64, d time.Duration) float64 {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatal("channel closed while waiting for a value")
		}
		return v
	case <-time.After(d):
		t.Fatal("timed out waiting for a value")
		return 0
	}
}

// publishUntilRecv repeatedly invokes pub until a value shows up on ch. Because
// Subscribe and Publish are processed asynchronously on separate channels, a
// single Publish right after Subscribe may race ahead of the subscription being
// registered. Re-publishing until delivery makes the test deterministic. pub
// must only ever publish the same value to the topic ch is subscribed to so the
// returned value is unambiguous.
func publishUntilRecv(t *testing.T, pub func(), ch <-chan float64) float64 {
	t.Helper()
	deadline := time.After(testTimeout)
	for {
		pub()
		select {
		case v, ok := <-ch:
			if !ok {
				t.Fatal("channel closed while waiting for delivery")
			}
			return v
		case <-time.After(2 * time.Millisecond):
		}
		select {
		case <-deadline:
			t.Fatal("subscription never received a published value")
		default:
		}
	}
}

// drain discards any buffered values currently sitting on ch.
func drain(ch <-chan float64) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func TestNewNilConfigUsesDefaults(t *testing.T) {
	c := eventbus.New(nil)
	defer c.Close()

	ch := c.Subscribe("topic")
	got := publishUntilRecv(t, func() { c.Publish("topic", 1.5) }, ch)
	if got != 1.5 {
		t.Fatalf("got %v, want 1.5", got)
	}
}

func TestNewCustomConfig(t *testing.T) {
	cfg := &eventbus.Config{IncomingBuffer: 4, SubscribeBuffer: 2, UnsubscribeBuffer: 2}
	c := eventbus.New(cfg)
	defer c.Close()

	ch := c.Subscribe("topic")
	got := publishUntilRecv(t, func() { c.Publish("topic", 42) }, ch)
	if got != 42 {
		t.Fatalf("got %v, want 42", got)
	}
}

func TestPublishDeliversToSubscriber(t *testing.T) {
	c := eventbus.New(nil)
	defer c.Close()

	ch := c.Subscribe("rpm")
	got := publishUntilRecv(t, func() { c.Publish("rpm", 3000) }, ch)
	if got != 3000 {
		t.Fatalf("got %v, want 3000", got)
	}
}

func TestPublishToOtherTopicNotDelivered(t *testing.T) {
	c := eventbus.New(nil)
	defer c.Close()

	ch := c.Subscribe("a")
	// Make sure the subscription is live first.
	publishUntilRecv(t, func() { c.Publish("a", 1) }, ch)
	drain(ch)

	c.Publish("b", 99)
	select {
	case v := <-ch:
		t.Fatalf("received %v on topic a from a publish to topic b", v)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestMultipleSubscribersSameTopic(t *testing.T) {
	c := eventbus.New(nil)
	defer c.Close()

	ch1 := c.Subscribe("temp")
	ch2 := c.Subscribe("temp")

	// Keep publishing until both subscriptions are registered and have each
	// received at least one value.
	var got1, got2 bool
	deadline := time.After(testTimeout)
	for !(got1 && got2) {
		c.Publish("temp", 7)
		select {
		case <-ch1:
			got1 = true
		case <-ch2:
			got2 = true
		case <-time.After(2 * time.Millisecond):
		}
		select {
		case <-deadline:
			t.Fatalf("both subscribers did not receive: got1=%v got2=%v", got1, got2)
		default:
		}
	}
}

func TestSubscribeFuncReceivesAndCancel(t *testing.T) {
	c := eventbus.New(nil)
	defer c.Close()

	var count int64
	cancel := c.SubscribeFunc("boost", func(v float64) {
		atomic.AddInt64(&count, 1)
	})

	// Publish until the callback fires at least once.
	deadline := time.After(testTimeout)
	for atomic.LoadInt64(&count) == 0 {
		c.Publish("boost", 1)
		time.Sleep(2 * time.Millisecond)
		select {
		case <-deadline:
			t.Fatal("SubscribeFunc callback never fired")
		default:
		}
	}

	cancel()
	// Give the unsubscribe time to propagate, then confirm no further calls.
	time.Sleep(20 * time.Millisecond)
	before := atomic.LoadInt64(&count)
	for i := 0; i < 50; i++ {
		c.Publish("boost", 1)
	}
	time.Sleep(50 * time.Millisecond)
	if after := atomic.LoadInt64(&count); after != before {
		t.Fatalf("callback fired after cancel: before=%d after=%d", before, after)
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	c := eventbus.New(nil)
	defer c.Close()

	ch := c.Subscribe("x")
	publishUntilRecv(t, func() { c.Publish("x", 1) }, ch)
	drain(ch)

	c.Unsubscribe(ch)

	// The run loop closes the channel on unsubscribe; reading should eventually
	// observe a closed channel.
	deadline := time.After(testTimeout)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // closed as expected
			}
		case <-deadline:
			t.Fatal("channel was not closed after Unsubscribe")
		}
	}
}

func TestUnsubscribeUnknownChannelNoPanic(t *testing.T) {
	c := eventbus.New(nil)
	defer c.Close()

	// A channel that was never registered should be ignored without panicking.
	ch := make(chan float64, 1)
	c.Unsubscribe(ch)
	// Confirm the controller is still functional afterwards.
	sub := c.Subscribe("ok")
	got := publishUntilRecv(t, func() { c.Publish("ok", 5) }, sub)
	if got != 5 {
		t.Fatalf("got %v, want 5", got)
	}
}

func TestSetOnMessageInvoked(t *testing.T) {
	c := eventbus.New(nil)
	defer c.Close()

	var mu sync.Mutex
	var lastTopic string
	var lastData float64
	var calls int

	c.SetOnMessage(func(topic string, data float64) {
		mu.Lock()
		lastTopic = topic
		lastData = data
		calls++
		mu.Unlock()
	})

	deadline := time.After(testTimeout)
	for {
		c.Publish("hook", 12.5)
		time.Sleep(2 * time.Millisecond)
		mu.Lock()
		got := calls
		mu.Unlock()
		if got > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("onMessage callback never fired")
		default:
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if lastTopic != "hook" || lastData != 12.5 {
		t.Fatalf("got topic=%q data=%v, want hook/12.5", lastTopic, lastData)
	}
}

func TestSetOnMessageNilClears(t *testing.T) {
	c := eventbus.New(nil)
	defer c.Close()

	var calls int64
	c.SetOnMessage(func(string, float64) { atomic.AddInt64(&calls, 1) })

	deadline := time.After(testTimeout)
	for atomic.LoadInt64(&calls) == 0 {
		c.Publish("hook", 1)
		time.Sleep(2 * time.Millisecond)
		select {
		case <-deadline:
			t.Fatal("callback never fired before clearing")
		default:
		}
	}

	c.SetOnMessage(nil)
	time.Sleep(20 * time.Millisecond)
	before := atomic.LoadInt64(&calls)
	for i := 0; i < 50; i++ {
		c.Publish("hook", 1)
	}
	time.Sleep(50 * time.Millisecond)
	if after := atomic.LoadInt64(&calls); after != before {
		t.Fatalf("callback fired after SetOnMessage(nil): before=%d after=%d", before, after)
	}
}

func TestCloseClosesSubscriberChannels(t *testing.T) {
	c := eventbus.New(nil)

	ch := c.Subscribe("y")
	publishUntilRecv(t, func() { c.Publish("y", 1) }, ch)
	drain(ch)

	c.Close()

	deadline := time.After(testTimeout)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // closed by cleanup
			}
		case <-deadline:
			t.Fatal("subscriber channel not closed after Close")
		}
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	c := eventbus.New(nil)
	c.Close()
	// A second Close must not panic (guarded by sync.Once).
	c.Close()
}

func TestDIFFAggregatorPublishesDifference(t *testing.T) {
	c := eventbus.New(nil)
	defer c.Close()

	// Default aggregators include DIFFAggregator(v_Vehicle, v_Vehicle2, VDIFFL).
	out := c.Subscribe("VDIFFL")

	// Publish both inputs repeatedly until the aggregated diff is delivered.
	// Each completed pair yields (second - first) = 30 - 10 = 20.
	got := publishUntilRecv(t, func() {
		c.Publish("ActualIn.v_Vehicle", 10)
		c.Publish("ActualIn.v_Vehicle2", 30)
	}, out)
	if got != 20 {
		t.Fatalf("VDIFFL = %v, want 20", got)
	}
}

func TestConcurrentPublishersNoRace(t *testing.T) {
	// Primarily intended to be run with -race.
	c := eventbus.New(nil)
	defer c.Close()

	ch := c.Subscribe("hot")
	go func() {
		for range ch {
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.Publish("hot", float64(n))
			}
		}(i)
	}
	wg.Wait()
}
