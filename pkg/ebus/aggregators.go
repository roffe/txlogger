package ebus

type EventAggregatorFunc func(name string, value float64)

type EventAggregator struct {
	fun EventAggregatorFunc
}

func RegisterAggregator(aggs ...*EventAggregator) {
	aggregatorsLock.Lock()
	defer aggregatorsLock.Unlock()
outer:

	for _, agg := range aggs {
		for _, existing := range aggregators {
			if existing == agg {
				continue outer
			}
		}
		aggregators = append(aggregators, agg)
	}
}

func DIFFAggregator(first, second, outputName string) *EventAggregator {
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
				Publish(outputName, secondValue-firstValue)
				firstUpdated, secondUpdated = false, false
			}
		},
	}
}
