package eventbus

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/roffe/txlogger/pkg/debug"
)

type Config struct {
	IncomingBuffer    int
	SubscribeBuffer   int
	UnsubscribeBuffer int
	ChannelBuffer     int
	CacheTTL          time.Duration
}

var DefaultConfig = &Config{
	IncomingBuffer:    1000,
	SubscribeBuffer:   100,
	UnsubscribeBuffer: 100,
	ChannelBuffer:     50,
	CacheTTL:          time.Minute,
}

type EBusMessage struct {
	Topic string
	Data  float64
}

type Controller struct {
	subs     sync.Map
	incoming chan EBusMessage
	sub      chan newSub
	unsub    chan chan float64
	cache    *ttlcache.Cache[string, float64]

	// Optimized aggregator management
	aggregatorIndex map[string][]*EventAggregator
	aggregatorLock  sync.RWMutex

	closeOnce sync.Once
	quit      chan struct{}

	onMessage func(string, float64)
}

type newSub struct {
	topic string
	resp  chan float64
}

func New(cfg *Config) *Controller {
	if cfg == nil {
		cfg = DefaultConfig
	}

	c := &Controller{
		incoming:        make(chan EBusMessage, cfg.IncomingBuffer),
		sub:             make(chan newSub, cfg.SubscribeBuffer),
		unsub:           make(chan chan float64, cfg.UnsubscribeBuffer),
		cache:           ttlcache.New[string, float64](ttlcache.WithTTL[string, float64](cfg.CacheTTL)),
		quit:            make(chan struct{}),
		aggregatorIndex: make(map[string][]*EventAggregator),
	}

	// Register default aggregators
	c.RegisterAggregator(
		DIFFAggregator("MAF.m_AirInlet", "m_Request", "AirDIFF"),
		DIFFAggregator("MAF.m_AirInlet", "AirMassMast.m_Request", "AirDIFF"),
	)

	go c.run()

	return c
}

func (e *Controller) SetOnMessage(f func(string, float64)) {
	e.onMessage = f
}

func (e *Controller) run() {
	//var wg sync.WaitGroup
	//
	//for i := 0; i < 5; i++ {
	//	wg.Add(1)
	//	go e.run2(i, &wg)
	//}

	//outer:

	//t := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-e.quit:
			e.cleanup()
			//break outer
			return
		case msg := <-e.incoming:
			if f := e.onMessage; f != nil {
				f(msg.Topic, msg.Data)
			}
			e.handleMessage(msg)
		case sub := <-e.sub:
			e.handleSubscription(sub)
		case unsub := <-e.unsub:
			e.handleUnsubscription(unsub)
			/*
				case <-t.C:
					e.subs.Range(func(key, value interface{}) bool {
						topic := key.(string)
						subs := value.([]chan float64)
						log.Printf("Topic %s has %d subscribers", topic, len(subs))
						return true
					})
			*/
		}
	}
	//log.Println("Waiting for goroutines to finish")
	//wg.Wait()
	//log.Println("All goroutines finished")

}

func (e *Controller) run2(i int, wg *sync.WaitGroup) {
	log.Println("Starting ebus worker", i)
	defer wg.Done()
	for {
		select {
		case <-e.quit:
			return
		case msg := <-e.incoming:
			if f := e.onMessage; f != nil {
				f(msg.Topic, msg.Data)
			}
			e.handleMessage(msg)
		}
	}
}

func (e *Controller) handleMessage(msg EBusMessage) {
	e.cache.Set(msg.Topic, msg.Data, ttlcache.DefaultTTL)
	// Get subscribers
	if value, ok := e.subs.Load(msg.Topic); ok {
		if subs, ok := value.([]chan float64); ok {
			for _, sub := range subs {
				select {
				case sub <- msg.Data:
				default:
					log.Printf("Channel full for topic %s", msg.Topic)
				}
			}
		}
	}
	// Process aggregators
	e.aggregatorLock.RLock()
	if aggregators, exists := e.aggregatorIndex[msg.Topic]; exists {
		for _, agg := range aggregators {
			agg.fun(e, msg.Topic, msg.Data)
		}
	}
	e.aggregatorLock.RUnlock()
}

func (e *Controller) handleSubscription(sub newSub) {
	var subs []chan float64
	if value, ok := e.subs.Load(sub.topic); ok {
		subs = value.([]chan float64)
	}
	subs = append(subs, sub.resp)
	e.subs.Store(sub.topic, subs)

	// Send cached value if available
	if item := e.cache.Get(sub.topic); item != nil {
		select {
		case sub.resp <- item.Value():
		default:
			log.Printf("Cache hit but channel full for topic %s", sub.topic)
		}
	}
}

func (e *Controller) handleUnsubscription(unsub chan float64) {
	e.subs.Range(func(key, value interface{}) bool {
		topic := key.(string)
		subs := value.([]chan float64)
		for i, sub := range subs {
			if sub == unsub {
				newSubs := append(subs[:i], subs[i+1:]...)
				if len(newSubs) == 0 {
					e.subs.Delete(topic)
				} else {
					e.subs.Store(topic, newSubs)
				}
				close(unsub)
				return false
			}
		}
		return true
	})
}

func (e *Controller) Close() {
	e.closeOnce.Do(func() {
		close(e.quit)
	})
}

func (e *Controller) cleanup() {
	e.cache.DeleteAll()
	e.subs.Range(func(key, value interface{}) bool {
		subs := value.([]chan float64)
		for _, sub := range subs {
			close(sub)
		}
		return true
	})
}

func (e *Controller) RegisterAggregator(aggs ...*EventAggregator) {
	e.aggregatorLock.Lock()
	defer e.aggregatorLock.Unlock()
	for _, agg := range aggs {
		// Index aggregators by their monitored topics
		for _, topic := range agg.GetTopics() {
			e.aggregatorIndex[topic] = append(e.aggregatorIndex[topic], agg)
		}
	}
}

func (e *Controller) Publish(topic string, data float64) error {
	select {
	case e.incoming <- EBusMessage{Topic: topic, Data: data}:
		return nil
	default:
		return errors.New(topic + "publish channel full")
	}
}

// SubscribeFunc returns a function that can be used to unsubscribe the function
func (e *Controller) SubscribeFunc(topic string, fn func(float64)) (cancel func()) {
	// log.Println("SubscribeFunc", topic)
	respChan := e.Subscribe(topic)
	go func() {
		for v := range respChan {
			debug.Do(func() {
				fn(v)
			})
		}
	}()
	cancel = func() {
		//log.Println("UnsubscribeFunc", topic)
		e.Unsubscribe(respChan)
	}
	return
}

func (e *Controller) Subscribe(topic string) chan float64 {
	//log.Println("Subscribe", topic)
	respChan := make(chan float64, 10)
	e.sub <- newSub{topic: topic, resp: respChan}
	return respChan
}

func (e *Controller) Unsubscribe(channel chan float64) {
	e.unsub <- channel
}

func (e *Controller) Values() map[string]float64 {
	values := make(map[string]float64)
	for k, v := range e.cache.Items() {
		values[k] = v.Value()
	}
	return values
}
