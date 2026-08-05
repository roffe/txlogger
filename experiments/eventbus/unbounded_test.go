package eventbus_test

import (
	"sync"
	"testing"
	"time"

	"github.com/roffe/txlogger/experiments/eventbus"
)

func TestUnboundedChanPreservesOrder(t *testing.T) {
	ch := eventbus.NewUnboundedChan[int]()

	const n = 10000
	go func() {
		for i := 0; i < n; i++ {
			ch.In() <- i
		}
	}()

	for i := 0; i < n; i++ {
		select {
		case got := <-ch.Out():
			if got != i {
				t.Fatalf("out of order: got %d, want %d", got, i)
			}
		case <-time.After(testTimeout):
			t.Fatalf("timed out waiting for value %d", i)
		}
	}
	ch.Close()
}

func TestUnboundedChanBuffersBeyondCapacity(t *testing.T) {
	ch := eventbus.NewUnboundedChan[int]()

	// Send many more values than the underlying in/out buffers (16 each)
	// without anyone reading. The unbounded buffer must absorb them without
	// blocking the sender.
	const n = 5000
	done := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			ch.In() <- i
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("sender blocked: unbounded channel did not buffer")
	}

	for i := 0; i < n; i++ {
		if got := <-ch.Out(); got != i {
			t.Fatalf("out of order: got %d, want %d", got, i)
		}
	}
	ch.Close()
}

func TestUnboundedChanCloseClosesOut(t *testing.T) {
	ch := eventbus.NewUnboundedChan[int]()
	ch.In() <- 1
	ch.In() <- 2

	// Read the buffered values, then close and confirm Out() is closed.
	if got := <-ch.Out(); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
	if got := <-ch.Out(); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}

	ch.Close()

	select {
	case _, ok := <-ch.Out():
		if ok {
			t.Fatal("expected Out() to be drained/closed after Close")
		}
	case <-time.After(testTimeout):
		t.Fatal("Out() was not closed after Close")
	}
}

func TestUnboundedChanConcurrentProducerConsumer(t *testing.T) {
	// Primarily intended to be run with -race.
	ch := eventbus.NewUnboundedChan[int]()

	const n = 20000
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			ch.In() <- i
		}
	}()

	for i := 0; i < n; i++ {
		if got := <-ch.Out(); got != i {
			t.Fatalf("out of order: got %d, want %d", got, i)
		}
	}
	wg.Wait()
	ch.Close()
}
