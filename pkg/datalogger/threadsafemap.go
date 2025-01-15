package datalogger

import "sync"

type ThreadSafeMap struct {
	values map[string]float64
	sync.RWMutex
}

func NewThreadSafeMap() *ThreadSafeMap {
	return &ThreadSafeMap{
		values: make(map[string]float64),
	}
}

func (t *ThreadSafeMap) Keys() []string {
	t.RLock()
	defer t.RUnlock()
	keys := make([]string, 0, len(t.values))
	for k := range t.values {
		keys = append(keys, k)
	}
	return keys
}

func (t *ThreadSafeMap) Exists(name string) bool {
	t.RLock()
	defer t.RUnlock()
	_, ok := t.values[name]
	return ok
}

func (t *ThreadSafeMap) Set(name string, value float64) {
	t.Lock()
	defer t.Unlock()
	t.values[name] = value
}

func (t *ThreadSafeMap) Get(name string) float64 {
	t.RLock()
	defer t.RUnlock()
	return t.values[name]
}

func (t *ThreadSafeMap) Delete(name string) {
	t.Lock()
	defer t.Unlock()
	delete(t.values, name)
}
