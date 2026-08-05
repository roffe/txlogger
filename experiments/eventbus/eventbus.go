package eventbus

import (
	"log"
	"sync"
	"sync/atomic"
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

type onMessageFunc func(string, float64)

type Controller struct {
	subs     map[string][]chan float64
	subTopic map[chan float64]string
	incoming chan EBusMessage
	sub      chan newSub
	unsub    chan chan float64
	// cache    *ttlcache.Cache[string, float64]

	aggregatorIndex map[string][]*EventAggregator

	closeOnce sync.Once
	quit      chan struct{}

	onMessage atomic.Pointer[onMessageFunc]
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
		subs:            make(map[string][]chan float64),
		subTopic:        make(map[chan float64]string),
		quit:            make(chan struct{}),
		aggregatorIndex: make(map[string][]*EventAggregator),
	}

	// Register default aggregators before starting the run loop so no
	// synchronization is needed on aggregatorIndex.
	c.registerAggregator(
		DIFFAggregator("ActualIn.v_Vehicle", "ActualIn.v_Vehicle2", "VDIFFL"),
		DIFFAggregator("ActualIn.v_Vehicle3", "ActualIn.v_Vehicle2", "VDIFFR"),
		DIFFAggregator("MAF.m_AirInlet", "m_Request", "AirDIFF"),
		DIFFAggregator("MAF.m_AirInlet", "AirMassMast.m_Request", "AirDIFF"),
	)

	go c.run()

	return c
}

func (e *Controller) SetOnMessage(f func(string, float64)) {
	if f == nil {
		e.onMessage.Store(nil)
		return
	}
	fn := onMessageFunc(f)
	e.onMessage.Store(&fn)
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

func (e *Controller) handleMessage(msg EBusMessage) {
	if f := e.onMessage.Load(); f != nil {
		(*f)(msg.Topic, msg.Data)
	}

	for _, sub := range e.subs[msg.Topic] {
		select {
		case sub <- msg.Data:
		default:
			log.Printf("Channel full for topic %s", msg.Topic)
		}
	}

	if aggregators, exists := e.aggregatorIndex[msg.Topic]; exists {
		for _, agg := range aggregators {
			agg.fun(e, msg.Topic, msg.Data)
		}
	}
}

func (e *Controller) handleSubscription(sub newSub) {
	e.subs[sub.topic] = append(e.subs[sub.topic], sub.resp)
	e.subTopic[sub.resp] = sub.topic
}

func (e *Controller) handleUnsubscription(unsub chan float64) {
	topic, ok := e.subTopic[unsub]
	if !ok {
		return
	}
	delete(e.subTopic, unsub)

	subs := e.subs[topic]
	for i, sub := range subs {
		if sub != unsub {
			continue
		}
		last := len(subs) - 1
		subs[i] = subs[last]
		subs[last] = nil
		subs = subs[:last]
		if len(subs) == 0 {
			delete(e.subs, topic)
		} else {
			e.subs[topic] = subs
		}
		close(unsub)
		return
	}
}

func (e *Controller) Close() {
	e.closeOnce.Do(func() {
		close(e.quit)
	})
}

func (e *Controller) cleanup() {
	for topic, subs := range e.subs {
		for _, sub := range subs {
			close(sub)
		}
		e.subs[topic] = nil
		delete(e.subs, topic)
	}
	for ch := range e.subTopic {
		delete(e.subTopic, ch)
	}
}

// registerAggregator indexes aggregators by their monitored topics. Must only
// be called before run() starts; aggregatorIndex is read without locking on
// the run goroutine.
func (e *Controller) registerAggregator(aggs ...*EventAggregator) {
	for _, agg := range aggs {
		for _, topic := range agg.GetTopics() {
			e.aggregatorIndex[topic] = append(e.aggregatorIndex[topic], agg)
		}
	}
}

func (e *Controller) Publish(topic string, data float64) {
	select {
	case e.incoming <- EBusMessage{Topic: topic, Data: data}:
	default:
		log.Println(topic + "publish channel full")
	}
}

// SubscribeFunc returns a cancel function is used to unsubscribe the function
func (e *Controller) SubscribeFunc(topic string, fn func(float64)) func() {
	respChan := e.Subscribe(topic)
	go func() {
		for v := range respChan {
			fn(v)
		}
	}()
	return func() {
		e.Unsubscribe(respChan)
	}
}

func (e *Controller) Subscribe(topic string) chan float64 {
	respChan := make(chan float64, 20)
	e.sub <- newSub{topic: topic, resp: respChan}
	return respChan
}

func (e *Controller) Unsubscribe(channel chan float64) {
	e.unsub <- channel
}
