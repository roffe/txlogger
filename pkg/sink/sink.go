package sink

import (
	"context"
	"errors"
	"log"
	"time"
)

type Message struct {
	Data []byte
}

type Manager struct {
	incoming    chan *Message
	subscribers []*Subscriber
	register    chan *Subscriber
	unregister  chan *Subscriber
}

func NewManager() *Manager {
	mgr := &Manager{
		incoming:    make(chan *Message, 100),
		subscribers: make([]*Subscriber, 0),
		register:    make(chan *Subscriber, 10),
		unregister:  make(chan *Subscriber, 10),
	}
	go mgr.run(context.TODO())
	return mgr
}

func (mgr *Manager) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case sub := <-mgr.register:
			mgr.subscribers = append(mgr.subscribers, sub)
		case sub := <-mgr.unregister:
			for i, s := range mgr.subscribers {
				if s == sub {
					mgr.subscribers = append(mgr.subscribers[:i], mgr.subscribers[i+1:]...)
					close(sub.incoming)
					break
				}
			}
		case msg := <-mgr.incoming:
			for _, sub := range mgr.subscribers {
				select {
				case sub.incoming <- msg:
				default:
					log.Println("failed to deliver message to subscriber")
					sub.failedDeliveries++
					if sub.failedDeliveries >= 10 {
						mgr.unregister <- sub
					}
				}
			}
		}
	}
}

var ErrPushTimeout = errors.New("timeout pushing message")

func (mgr *Manager) Push(msg *Message) error {
	t := time.NewTimer(1 * time.Second)
	defer t.Stop()
	select {
	case mgr.incoming <- msg:
		return nil
	case <-t.C:
		return ErrPushTimeout
	}
}

type Subscriber struct {
	mgr              *Manager
	incoming         chan *Message
	failedDeliveries int
}

func (mgr *Manager) NewSubscriber(onMessage func(*Message)) *Subscriber {
	sub := &Subscriber{
		mgr:      mgr,
		incoming: make(chan *Message, 100),
	}
	mgr.register <- sub
	if onMessage != nil {
		go func() {
			for msg := range sub.incoming {
				if len(msg.Data) == 0 {
					return
				}
				onMessage(msg)
			}
		}()
	}
	return sub
}

func (sub *Subscriber) Close() {
	sub.mgr.unregister <- sub
}

func (sub *Subscriber) Next(ctx context.Context) (*Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-sub.incoming:
		return msg, nil
	}
}
