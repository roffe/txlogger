package eventbus

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

type EBusMessage struct {
	Topic string
	Data  float64
}

type Controller struct {
	subs     map[string][]chan float64
	incoming chan EBusMessage
	sub      chan newSub
	unsub    chan chan float64
	cache    *ttlcache.Cache[string, float64]

	aggregators []*EventAggregator

	aggregatorsLock sync.Mutex

	closeOnce sync.Once
	quit      chan struct{}
}

type newSub struct {
	topic string
	resp  chan float64
}

func New() *Controller {
	c := &Controller{
		subs:     make(map[string][]chan float64),
		incoming: make(chan EBusMessage, 100),
		sub:      make(chan newSub, 10),
		unsub:    make(chan chan float64, 10),
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
			// Cache check disabled as dedupping causes problems with mapviewer in some cases
			//if v := e.cache.Get(msg.Topic); v != nil {
			//	if v.Value() == msg.Data {
			//		continue
			//	}
			//}
			e.cache.Set(msg.Topic, msg.Data, ttlcache.DefaultTTL)
			for _, sub := range e.subs[msg.Topic] {
				select {
				case sub <- msg.Data:
				default:
				}
			}
			for _, agg := range e.aggregators {
				agg.fun(e, msg.Topic, msg.Data)
			}
		case sub := <-e.sub:
			e.subs[sub.topic] = append(e.subs[sub.topic], sub.resp)
			if itm := e.cache.Get(sub.topic); itm != nil {
				//				log.Println("Cache hit", sub.topic, itm.Value())
				select {
				case sub.resp <- itm.Value():
				default:
					log.Println("Cache hit but channel full", sub.topic, itm.Value())
				}
			}
		case unsub := <-e.unsub:
		outer:
			for topic, subs := range e.subs {
				for i, sub := range subs {
					if sub == unsub {
						//log.Println("Unsubscribe", topic)
						e.subs[topic] = append(subs[:i], subs[i+1:]...)
						close(unsub)
						if len(e.subs[topic]) == 0 {
							delete(e.subs, topic)
						}
						break outer
					}
				}
			}
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
	case e.incoming <- EBusMessage{Topic: topic, Data: data}:
		return nil
	default:
		return errors.New("publish channel full")
	}
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
	// log.Println("Subscribe", topic)
	respChan := make(chan float64, 10)
	e.sub <- newSub{topic: topic, resp: respChan}
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

func (e *Controller) Values() map[string]float64 {
	values := make(map[string]float64)
	for k, v := range e.cache.Items() {
		values[k] = v.Value()
	}
	return values
}
