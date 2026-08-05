package eventbus_test

import (
	"strconv"
	"testing"

	"github.com/roffe/txlogger/experiments/eventbus"
)

// BenchmarkPublishNoSubscribers measures the bare cost of enqueueing a message
// when nobody is listening (the run loop still drains and processes it).
func BenchmarkPublishNoSubscribers(b *testing.B) {
	c := eventbus.New(nil)
	defer c.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Publish("topic", float64(i))
	}
}

// BenchmarkPublishOneSubscriber measures publish throughput with a single
// active subscriber that immediately drains its channel.
func BenchmarkPublishOneSubscriber(b *testing.B) {
	c := eventbus.New(nil)
	defer c.Close()

	ch := c.Subscribe("topic")
	go func() {
		for range ch {
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Publish("topic", float64(i))
	}
}

// BenchmarkPublishManySubscribers measures fan-out cost across N subscribers.
func BenchmarkPublishManySubscribers(b *testing.B) {
	for _, subs := range []int{1, 4, 16, 64} {
		b.Run(strconv.Itoa(subs), func(b *testing.B) {
			c := eventbus.New(nil)
			defer c.Close()

			for i := 0; i < subs; i++ {
				ch := c.Subscribe("topic")
				go func() {
					for range ch {
					}
				}()
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.Publish("topic", float64(i))
			}
		})
	}
}

// BenchmarkPublishParallel measures publish throughput under contention from
// multiple concurrent publishers.
func BenchmarkPublishParallel(b *testing.B) {
	c := eventbus.New(nil)
	defer c.Close()

	ch := c.Subscribe("topic")
	go func() {
		for range ch {
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Publish("topic", 1)
		}
	})
}

// BenchmarkSubscribeUnsubscribe measures the cost of the subscribe/unsubscribe
// round trip through the run loop.
func BenchmarkSubscribeUnsubscribe(b *testing.B) {
	c := eventbus.New(nil)
	defer c.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch := c.Subscribe("topic")
		c.Unsubscribe(ch)
	}
}

// BenchmarkAggregatorPublish measures publishing to topics watched by the
// default DIFF aggregators, exercising the aggregator index path.
func BenchmarkAggregatorPublish(b *testing.B) {
	c := eventbus.New(nil)
	defer c.Close()

	out := c.Subscribe("VDIFFL")
	go func() {
		for range out {
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Publish("ActualIn.v_Vehicle", float64(i))
		c.Publish("ActualIn.v_Vehicle2", float64(i)+1)
	}
}

// BenchmarkUnboundedChan measures round-trip throughput of the unbounded
// channel with a concurrent consumer.
func BenchmarkUnboundedChan(b *testing.B) {
	ch := eventbus.NewUnboundedChan[int]()
	done := make(chan struct{})
	go func() {
		for range ch.Out() {
		}
		close(done)
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.In() <- i
	}
	b.StopTimer()
	ch.Close()
	<-done
}
