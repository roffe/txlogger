package eventbus

type EventAggregatorFunc func(name string, value float64)

type EventAggregator struct {
	fun EventAggregatorFunc
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

func DIFFAggregator(c *Controller, first, second, outputName string) *EventAggregator {
	var firstUpdated, secondUpdated bool
	var firstValue, secondValue float64
	return &EventAggregator{
		fun: func(name string, value float64) {
			if name == first {
				firstValue = value
				firstUpdated = true
			}
			if name == second {
				secondValue = value
				secondUpdated = true
			}
			if firstUpdated && secondUpdated {
				c.Publish(outputName, secondValue-firstValue)
				firstUpdated, secondUpdated = false, false
			}
		},
	}
}
