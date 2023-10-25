package windows

import (
	"log"
	"sync"
)

type MapViewerEvent struct {
	SymbolName string
	Value      float64
}

type MapViewerSubscriber struct {
	SymbolName string
	Widget     MapViewerWindowWidget
}

type MapViewerHandler struct {
	subs map[string][]MapViewerWindowWidget

	subChan   chan MapViewerSubscriber
	unsubChan chan MapViewerSubscriber

	incoming chan MapViewerEvent

	quit chan struct{}

	aggregators     []*MapAggregator
	aggregatorsLock sync.Mutex
}

func NewMapViewerHandler() *MapViewerHandler {
	mvh := &MapViewerHandler{
		subChan:     make(chan MapViewerSubscriber, 10),
		unsubChan:   make(chan MapViewerSubscriber, 10),
		subs:        make(map[string][]MapViewerWindowWidget),
		incoming:    make(chan MapViewerEvent, 100),
		quit:        make(chan struct{}),
		aggregators: make([]*MapAggregator, 0),
	}

	mvh.AddAggregator(
		NewDIFFAggregator("MAF.m_AirInlet", "m_Request", "AirDIFF"),
		NewDIFFAggregator("MAF.m_AirInlet", "AirMassMast.m_Request", "AirDIFF"),
	)

	go mvh.run()
	return mvh
}

func (mvh *MapViewerHandler) Close() {
	close(mvh.quit)
}

func (mvh *MapViewerHandler) Subscribe(symbolName string, mv MapViewerWindowWidget) {
	mvh.subChan <- MapViewerSubscriber{SymbolName: symbolName, Widget: mv}
}

func (mvh *MapViewerHandler) Unsubscribe(symbolName string, mv MapViewerWindowWidget) {
	mvh.unsubChan <- MapViewerSubscriber{SymbolName: symbolName, Widget: mv}
}

func (mvh *MapViewerHandler) SetValue(symbolName string, value float64) {
	select {
	case mvh.incoming <- MapViewerEvent{SymbolName: symbolName, Value: value}:
		return
	default:
		log.Println("dropped update")
		return
	}
}

func (mvh *MapViewerHandler) run() {
	for {
		select {
		case <-mvh.quit:
			return
		case sub := <-mvh.subChan:
			mvh.subs[sub.SymbolName] = append(mvh.subs[sub.SymbolName], sub.Widget)
		case unsub := <-mvh.unsubChan:
			for i, m := range mvh.subs[unsub.SymbolName] {
				if m == unsub.Widget {
					mvh.subs[unsub.SymbolName] = append(mvh.subs[unsub.SymbolName][:i], mvh.subs[unsub.SymbolName][i+1:]...)
					break
				}
			}
		case event := <-mvh.incoming:
			for _, mv := range mvh.subs[event.SymbolName] {
				mv.SetValue(event.SymbolName, event.Value)
			}
			for _, agg := range mvh.aggregators {
				agg.Func(mvh, event.SymbolName, event.Value)
			}
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
