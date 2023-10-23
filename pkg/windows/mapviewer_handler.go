package windows

import (
	"log"
	"sync"
)

type MapViewerEvent struct {
	SymbolName string
	Value      float64
}

type MapViewerHandler struct {
	incoming    chan MapViewerEvent
	subs        map[string][]MapViewerWindowWidget
	aggregators []*MapAggregator

	quit            chan struct{}
	subsLock        sync.Mutex
	aggregatorsLock sync.Mutex
}

func NewMapViewerHandler() *MapViewerHandler {
	mvh := &MapViewerHandler{
		subs:        make(map[string][]MapViewerWindowWidget),
		incoming:    make(chan MapViewerEvent, 110),
		quit:        make(chan struct{}),
		aggregators: make([]*MapAggregator, 0),
	}

	mvh.AddAggregator(
		NewDIFFAggregator("MAF.m_AirInlet", "m_Request", "AirDIFF"),
	)

	go mvh.run()
	return mvh
}

func (mvh *MapViewerHandler) Close() {
	close(mvh.quit)
}

func (mvh *MapViewerHandler) Subscribe(symbolName string, mv MapViewerWindowWidget) {
	//	log.Printf("MapViewerHandler: Subscribe: %s", symbolName)
	mvh.subsLock.Lock()
	defer mvh.subsLock.Unlock()
	mvh.subs[symbolName] = append(mvh.subs[symbolName], mv)
}

func (mvh *MapViewerHandler) Unsubscribe(symbolName string, mv MapViewerWindowWidget) {
	//	log.Printf("MapViewerHandler: Unsubscribe: %s", symbolName)
	mvh.subsLock.Lock()
	defer mvh.subsLock.Unlock()
	for i, m := range mvh.subs[symbolName] {
		if m == mv {
			mvh.subs[symbolName] = append(mvh.subs[symbolName][:i], mvh.subs[symbolName][i+1:]...)
			break
		}
	}
}

func (mvh *MapViewerHandler) SetValue(symbolName string, value float64) {
	select {
	case mvh.incoming <- MapViewerEvent{SymbolName: symbolName, Value: value}:
		return
	default:
		//log.Panic("MapViewerHandler: incoming channel full")
		log.Println("dropped update")
		return
	}
}

func (mvh *MapViewerHandler) run() {
	for {
		select {
		case <-mvh.quit:
			return
		case event := <-mvh.incoming:
			for _, agg := range mvh.aggregators {
				agg.Func(mvh, event.SymbolName, event.Value)
			}
			mvh.subsLock.Lock()
			for _, mv := range mvh.subs[event.SymbolName] {
				mv.SetValue(event.SymbolName, event.Value)
			}
			mvh.subsLock.Unlock()
		}
	}
}

func (mvh *MapViewerHandler) AddAggregator(aggregators ...*MapAggregator) {
	mvh.aggregatorsLock.Lock()
	defer mvh.aggregatorsLock.Unlock()
outer:
	for _, agg := range aggregators {
		for _, existing := range mvh.aggregators {
			if existing == agg {
				continue outer
			}
		}
		mvh.aggregators = append(mvh.aggregators, agg)
	}
}

type MapAggregatorHandlerFunc func(mvh *MapViewerHandler, name string, value float64)

type MapAggregator struct {
	Func MapAggregatorHandlerFunc
}

/*
func NewAirDIFFAggregator() MapAggregatorHandlerFunc {
	var airUpdate uint8
	var tmpAir, tmpmReq float64
	return func(mvh *MapViewerHandler, name string, value float64) {
		if name == "MAF.m_AirInlet" {
			tmpAir = value
			airUpdate++
		}
		if name == "m_Request" {
			tmpmReq = value
			airUpdate++
		}
		if airUpdate >= 2 {
			mvh.Pump("AirDIFF", tmpmReq-tmpAir)
			airUpdate = 0
		}
	}
}
*/

func NewDIFFAggregator(first, second, outputName string) *MapAggregator {
	var firstUpdated, secondUpdated bool
	var firstValue, secondValue float64
	return &MapAggregator{
		Func: func(mvh *MapViewerHandler, name string, value float64) {
			if name == first {
				firstValue = value
				firstUpdated = true
			}
			if name == second {
				secondValue = value
				secondUpdated = true
			}
			if firstUpdated && secondUpdated {
				mvh.SetValue(outputName, secondValue-firstValue)
				firstUpdated, secondUpdated = false, false
			}
		},
	}
}
