package ebus

import (
	"context"
	"sync"

	"github.com/roffe/txlogger/pkg/eventbus"
)

var once sync.Once
var CONTROLLER *eventbus.Controller

func init() {
	once.Do(func() {
		CONTROLLER = eventbus.New(eventbus.DefaultConfig)
	})
}

func Publish(topic string, data float64) error {
	return CONTROLLER.Publish(topic, data)
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
	return CONTROLLER.SubscribeFunc(topic, f)
}

func Subscribe(topic string) chan float64 {
	return CONTROLLER.Subscribe(topic)
}

func SubscribeWithContext(ctx context.Context, topic string) (chan float64, error) {
	ch := CONTROLLER.Subscribe(topic)
	go func() {
		<-ctx.Done()
		CONTROLLER.Unsubscribe(ch)
	}()
	return ch, nil
}

func Unsubscribe(channel chan float64) {
	CONTROLLER.Unsubscribe(channel)
}

func SetOnMessage(f func(string, float64)) {
	CONTROLLER.SetOnMessage(f)
}
