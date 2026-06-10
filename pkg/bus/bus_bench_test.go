package bus

import (
	"sync/atomic"
	"testing"
)

func benchPublish(b *testing.B, nSubs int) {
	bus := NewBus[string, int]()
	var sink atomic.Int64
	for range nSubs {
		bus.SubscribeFunc("t", func(v int) { sink.Add(int64(v)) })
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		bus.Publish("t", 1)
	}
}

func BenchmarkPublish1(b *testing.B)    { benchPublish(b, 1) }
func BenchmarkPublish10(b *testing.B)   { benchPublish(b, 10) }
func BenchmarkPublish100(b *testing.B)  { benchPublish(b, 100) }
func BenchmarkPublish1000(b *testing.B) { benchPublish(b, 1000) }

// Contended: many goroutines publishing to the same topic concurrently.
func BenchmarkPublishParallel(b *testing.B) {
	bus := NewBus[string, int]()
	var sink atomic.Int64
	for range 100 {
		bus.SubscribeFunc("t", func(v int) { sink.Add(int64(v)) })
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bus.Publish("t", 1)
		}
	})
}
