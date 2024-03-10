package eventbus

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

type EBusMessage struct {
	Topic *string
	Data  *float64
}

type Controller struct {
	subs    map[string][]chan float64
	subsAll []chan *EBusMessage

	subsLock sync.Mutex

	incoming chan *EBusMessage
	unsubAll chan chan *EBusMessage
	unsub    chan chan float64
	cache    *ttlcache.Cache[string, float64]

	aggregators []*EventAggregator

	aggregatorsLock sync.Mutex

	closeOnce sync.Once
	quit      chan struct{}
}

func New() *Controller {
	c := &Controller{
		subs:     make(map[string][]chan float64),
		subsAll:  make([]chan *EBusMessage, 0),
		incoming: make(chan *EBusMessage, 100),
		unsubAll: make(chan chan *EBusMessage, 100),
		unsub:    make(chan chan float64, 100),
		cache:    ttlcache.New[string, float64](ttlcache.WithTTL[string, float64](1 * time.Minute)),
		quit:     make(chan struct{}),
	}
	c.RegisterAggregator(
		DIFFAggregator("MAF.m_AirInlet", "m_Request", "AirDIFF"),
		DIFFAggregator("MAF.m_AirInlet", "AirMassMast.m_Request", "AirDIFF"),
	)

	go c.run()
	return c
}

func (e *Controller) run() {
	for {
		select {
		case <-e.quit:
			e.cache.DeleteAll()
			return
		case msg := <-e.incoming:
			if v := e.cache.Get(*msg.Topic); v != nil {
				if v.Value() == *msg.Data {
					continue
				}
			}
			e.cache.Set(*msg.Topic, *msg.Data, ttlcache.DefaultTTL)
			for _, sub := range e.subsAll {
				select {
				case sub <- msg:
				default:
					e.UnsubscribeAll(sub)
				}
			}
			for _, sub := range e.subs[*msg.Topic] {
				select {
				case sub <- *msg.Data:
				default:
				}
			}
			for _, agg := range e.aggregators {
				agg.fun(e, *msg.Topic, *msg.Data)
			}

		case unsub := <-e.unsubAll:
			e.subsLock.Lock()
			for i, sub := range e.subsAll {
				if sub == unsub {
					log.Println("unsubAll", unsub)
					e.subsAll = append(e.subsAll[:i], e.subsAll[i+1:]...)
					close(sub)
					break
				}
			}
			e.subsLock.Unlock()
		case unsub := <-e.unsub:
			e.subsLock.Lock()
		outer:
			for topic, subz := range e.subs {
				for i, sub := range subz {
					if sub == unsub {
						log.Println("Unsubscribe", topic)
						e.subs[topic] = append(subz[:i], subz[i+1:]...)
						close(unsub)
						if len(e.subs[topic]) == 0 {
							delete(e.subs, topic)
						}
						break outer
					}
				}
			}
			e.subsLock.Unlock()
		}
	}
}

func (e *Controller) Close() {
	e.closeOnce.Do(func() {
		close(e.quit)
	})
}

func (e *Controller) Publish(topic string, data float64) error {
	select {
	case e.incoming <- &EBusMessage{Topic: &topic, Data: &data}:
		return nil
	default:
		return errors.New("publish channel full")
	}
}

func (e *Controller) SubscribeAll() chan *EBusMessage {
	respChan := make(chan *EBusMessage, 100)
	e.subsLock.Lock()
	e.subsAll = append(e.subsAll, respChan)
	e.subsLock.Unlock()

	e.cache.Range(func(item *ttlcache.Item[string, float64]) bool {
		k := item.Key()
		v := item.Value()
		respChan <- &EBusMessage{Topic: &k, Data: &v}
		return true
	})
	return respChan
}

func (e *Controller) SubscribeAllFunc(f func(topic string, value float64)) (cancel func()) {
	respChan := e.SubscribeAll()
	go func() {
		for v := range respChan {
			f(*v.Topic, *v.Data)
		}
	}()
	// return a function that can be used to unsubscribe
	cancel = func() { // unsubscribe
		e.UnsubscribeAll(respChan)
	}
	return
}

func (e *Controller) UnsubscribeAll(channel chan *EBusMessage) {
	e.unsubAll <- channel
}

// SubscribeFunc returns a function that can be used to unsubscribe the function
func (e *Controller) SubscribeFunc(topic string, f func(float64)) (cancel func()) {
	respChan := e.Subscribe(topic)
	go func() {
		for v := range respChan {
			f(v)
		}
	}()
	cancel = func() {
		e.Unsubscribe(respChan)
	}
	return
}

func (e *Controller) Subscribe(topic string) chan float64 {
	log.Println("Subscribe", topic)
	respChan := make(chan float64, 100)
	e.subsLock.Lock()
	e.subs[topic] = append(e.subs[topic], respChan)
	e.subsLock.Unlock()
	if itm := e.cache.Get(topic); itm != nil {
		respChan <- itm.Value()
	}
	return respChan
}

func (e *Controller) Unsubscribe(channel chan float64) {
	e.unsub <- channel
}

func (e *Controller) RegisterAggregator(aggs ...*EventAggregator) {
	e.aggregatorsLock.Lock()
	defer e.aggregatorsLock.Unlock()
outer:

	for _, agg := range aggs {
		for _, existing := range e.aggregators {
			if existing == agg {
				continue outer
			}
		}
		e.aggregators = append(e.aggregators, agg)
	}
}
