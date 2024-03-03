package ebus

import (
	"errors"
	"log"
	"sync"
)

type EBusMessage struct {
	Topic *string
	Data  *float64
}

var (
	initOnce  sync.Once
	subs      = make(map[string][]chan float64)
	subsMutex sync.Mutex

	subsAll      = make([]chan *EBusMessage, 0)
	subsAllMutex sync.Mutex

	inChan       = make(chan *EBusMessage, 100)
	unsubChan    = make(chan chan float64, 100)
	unsubAllChan = make(chan chan *EBusMessage, 100)
)

func init() {
	initOnce.Do(func() {
		go run()
	})
}

func run() {
	for {
		select {
		case msg := <-inChan:
			for _, sub := range subsAll {
				select {
				case sub <- msg:
				default:
					UnsubscribeAll(sub)
				}
			}
			for _, sub := range subs[*msg.Topic] {
				select {
				case sub <- *msg.Data:
				default:
				}
			}

		case unsub := <-unsubAllChan:
			subsAllMutex.Lock()
			for i, sub := range subsAll {
				if sub == unsub {
					log.Println("unsubAll", unsub)
					subsAll = append(subsAll[:i], subsAll[i+1:]...)
					close(sub)
					break
				}
			}
			subsAllMutex.Unlock()
		case unsub := <-unsubChan:
			subsMutex.Lock()
		outer:
			for topic, subz := range subs {
				for i, sub := range subz {
					if sub == unsub {
						log.Println("unsub", unsub)
						subs[topic] = append(subz[:i], subz[i+1:]...)
						close(unsub)
						if len(subs[topic]) == 0 {
							delete(subs, topic)
						}
						break outer
					}
				}
			}
			subsMutex.Unlock()
		}
	}
}

func Publish(topic string, data float64) error {
	//log.Println("Publish", topic, data)
	select {
	case inChan <- &EBusMessage{Topic: &topic, Data: &data}:
		return nil
	default:
		return errors.New("publish channel full")
	}
}

func SubscribeAll() chan *EBusMessage {
	respChan := make(chan *EBusMessage, 100)
	subsAllMutex.Lock()
	subsAll = append(subsAll, respChan)
	subsAllMutex.Unlock()
	return respChan
}

func SubscribeAllFunc(f func(topic string, value float64)) func() /*unsubscribe*/ {
	respChan := SubscribeAll()
	go func() {
		for v := range respChan {
			f(*v.Topic, *v.Data)
		}
	}()
	// return a function that can be used to unsubscribe
	return func() { // unsubscribe
		UnsubscribeAll(respChan)
	}
}

func UnsubscribeAll(channel chan *EBusMessage) {
	unsubAllChan <- channel
}

// SubscribeFunc returns a function that can be used to unsubscribe the function
func SubscribeFunc(topic string, f func(float64)) func() {
	respChan := Subscribe(topic)
	go func() {
		for v := range respChan {
			f(v)
		}
	}()
	return func() {
		Unsubscribe(respChan)
	}
}

func Subscribe(topic string) chan float64 {
	log.Println("Subscribe", topic)
	respChan := make(chan float64, 100)
	subsMutex.Lock()
	subs[topic] = append(subs[topic], respChan)
	subsMutex.Unlock()
	return respChan
}

func Unsubscribe(channel chan float64) {
	unsubChan <- channel
}
