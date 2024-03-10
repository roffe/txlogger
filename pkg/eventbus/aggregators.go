package eventbus

import "log"

type EventAggregator struct {
	fun EventAggregatorFunc
}

type EventAggregatorFunc func(c *Controller, name string, value float64)

func DIFFAggregator(first, second, output string) *EventAggregator {
	var firstUpdated, secondUpdated bool
	var firstValue, secondValue float64
	var diff float64
	return &EventAggregator{
		fun: func(c *Controller, name string, value float64) {
			if name == first {
				firstValue = value
				firstUpdated = true
			}
			if name == second {
				secondValue = value
				secondUpdated = true
			}
			if firstUpdated && secondUpdated {
				diff = secondValue - firstValue
				if err := c.Publish(output, diff); err != nil {
					log.Printf("failed to publish diff %s: %v", output, err)
				}
				firstUpdated, secondUpdated = false, false
			}
		},
	}
}
