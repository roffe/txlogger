package ebus

import (
	"sync"

	"github.com/roffe/txlogger/pkg/eventbus"
)

var once sync.Once
var eb *eventbus.Controller

func init() {
	once.Do(func() {
		eb = eventbus.New()
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

func Unsubscribe(channel chan float64) {
	eb.Unsubscribe(channel)
}
