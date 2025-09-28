package eventbus

import (
	"errors"
	"log"
	"sync"
)

type Config struct {
	IncomingBuffer    int
	SubscribeBuffer   int
	UnsubscribeBuffer int
	// CacheTTL          time.Duration
}

var DefaultConfig = &Config{
	IncomingBuffer:    1024,
	SubscribeBuffer:   20,
	UnsubscribeBuffer: 20,
	// CacheTTL:          time.Minute,
}

type EBusMessage struct {
	Topic string
	Data  float64
}

type Controller struct {
	subs     map[string][]chan float64
	incoming chan *EBusMessage
	sub      chan newSub
	unsub    chan chan float64
	//cache    *ttlcache.Cache[string, float64]

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
		incoming: make(chan *EBusMessage, cfg.IncomingBuffer),
		sub:      make(chan newSub, cfg.SubscribeBuffer),
		unsub:    make(chan chan float64, cfg.UnsubscribeBuffer),
		subs:     make(map[string][]chan float64),
		//cache:           ttlcache.New[string, float64](ttlcache.WithTTL[string, float64](cfg.CacheTTL)),
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
	for {
		select {
		case <-e.quit:
			e.cleanup()
			return
		case msg := <-e.incoming:
			e.handleMessage(msg)
		case sub := <-e.sub:
			e.handleSubscription(sub)
		case unsub := <-e.unsub:
			e.handleUnsubscription(unsub)
		}
	}
}

func (e *Controller) handleMessage(msg *EBusMessage) {
	if f := e.onMessage; f != nil {
		f(msg.Topic, msg.Data)
	}
	//e.cache.Set(msg.Topic, msg.Data, ttlcache.DefaultTTL)
	// Get subscribers

	for _, sub := range e.subs[msg.Topic] {
		select {
		case sub <- msg.Data:
		default:
			log.Printf("Channel full for topic %s", msg.Topic)
		}
	}

	// Process aggregators
	//e.aggregatorLock.RLock()
	if aggregators, exists := e.aggregatorIndex[msg.Topic]; exists {
		for _, agg := range aggregators {
			agg.fun(e, msg.Topic, msg.Data)
		}
	}
	//e.aggregatorLock.RUnlock()
}

func (e *Controller) handleSubscription(sub newSub) {
	e.subs[sub.topic] = append(e.subs[sub.topic], sub.resp)

	// Send cached value if available
	/*
		if item := e.cache.Get(sub.topic); item != nil {
			select {
			case sub.resp <- item.Value():
			default:
				log.Printf("Cache hit but channel full for topic %s", sub.topic)
			}
		}
	*/
}

func (e *Controller) handleUnsubscription(unsub chan float64) {
	for topic, subs := range e.subs {
		for i, sub := range subs {
			if sub == unsub {
				newSubs := append(subs[:i], subs[i+1:]...)
				if len(newSubs) == 0 {
					e.subs[topic] = nil
					delete(e.subs, topic)
				} else {
					e.subs[topic] = newSubs
				}
				close(unsub)
				return
			}
		}
	}
}

func (e *Controller) Close() {
	e.closeOnce.Do(func() {
		close(e.quit)
	})
}

func (e *Controller) cleanup() {
	//e.cache.DeleteAll()
	for topic, subs := range e.subs {
		for _, sub := range subs {
			close(sub)

		}
		e.subs[topic] = nil
		delete(e.subs, topic)
	}
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
	case e.incoming <- &EBusMessage{Topic: topic, Data: data}:
		return nil
	default:
		return errors.New(topic + "publish channel full")
	}
}

// SubscribeFunc returns a cancel function is used to unsubscribe the function
func (e *Controller) SubscribeFunc(topic string, fn func(float64)) func() {
	// log.Println("SubscribeFunc", topic)
	respChan := e.Subscribe(topic)
	go func() {
		for v := range respChan {
			fn(v)
		}
	}()
	return func() {
		// log.Println("UnsubscribeFunc", topic)
		e.Unsubscribe(respChan)
	}
}

func (e *Controller) Subscribe(topic string) chan float64 {
	//log.Println("Subscribe", topic)
	respChan := make(chan float64, 20)
	e.sub <- newSub{topic: topic, resp: respChan}
	return respChan
}

func (e *Controller) Unsubscribe(channel chan float64) {
	e.unsub <- channel
}

/*
func (e *Controller) Values() map[string]float64 {
	values := make(map[string]float64)
	for k, v := range e.cache.Items() {
		values[k] = v.Value()
	}
	return values
}
*/
