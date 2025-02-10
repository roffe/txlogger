package eventbus

import "log"

type EventAggregatorFunc func(c DiffPublisher, name string, value float64)

type DiffPublisher interface {
	Publish(name string, value float64) error
}

// Modified EventAggregator to track its topics
type EventAggregator struct {
	fun    EventAggregatorFunc
	topics []string
}

func (e *EventAggregator) GetTopics() []string {
	return e.topics
}

func DIFFAggregator(first, second, output string) *EventAggregator {
	var firstUpdated, secondUpdated bool
	var firstValue, secondValue float64

	return &EventAggregator{
		topics: []string{first, second},
		fun: func(c DiffPublisher, name string, value float64) {
			switch name {
			case first:
				firstValue = value
				firstUpdated = true
			case second:
				secondValue = value
				secondUpdated = true
			default:
				return
			}
			if firstUpdated && secondUpdated {
				diff := secondValue - firstValue
				if err := c.Publish(output, diff); err != nil {
					log.Printf("failed to publish diff %s: %v", output, err)
				}
				firstUpdated, secondUpdated = false, false
			}
		},
	}
}
