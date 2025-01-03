package ebus

import (
	"context"
	"sync"

	"github.com/roffe/txlogger/pkg/eventbus"
)

var once sync.Once
var eb *eventbus.Controller

func init() {
	once.Do(func() {
		eb = eventbus.New(&eventbus.DefaultConfig)
	})
}

func Publish(topic string, data float64) error {
	return eb.Publish(topic, data)
}

/*
	 func SubscribeAll() chan eventbus.EBusMessage {
		return eb.SubscribeAll()
	}

	func SubscribeAllFunc(f func(topic string, value float64)) func() {
		return eb.SubscribeAllFunc(f)
	}

	func UnsubscribeAll(channel chan eventbus.EBusMessage) {
		eb.UnsubscribeAll(channel)
	}
*/
func SubscribeFunc(topic string, f func(float64)) func() {
	return eb.SubscribeFunc(topic, f)
}

func Subscribe(topic string) chan float64 {
	return eb.Subscribe(topic)
}

func SubscribeWithContext(ctx context.Context, topic string) (chan float64, error) {
	ch := eb.Subscribe(topic)
	go func() {
		<-ctx.Done()
		eb.Unsubscribe(ch)
	}()
	return ch, nil
}

func Unsubscribe(channel chan float64) {
	eb.Unsubscribe(channel)
}

func SetOnMessage(f func(string, float64)) {
	eb.SetOnMessage(f)
}
