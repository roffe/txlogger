package datalogger

import "sync"

type ThreadSafeMap struct {
	values map[string]string
	sync.Mutex
}

func NewThreadSafeMap() *ThreadSafeMap {
	return &ThreadSafeMap{
		values: make(map[string]string),
	}
}

func (t *ThreadSafeMap) Keys() []string {
	t.Lock()
	defer t.Unlock()
	keys := make([]string, 0, len(t.values))
	for k := range t.values {
		keys = append(keys, k)
	}
	return keys
}

func (t *ThreadSafeMap) Exists(name string) bool {
	t.Lock()
	defer t.Unlock()
	_, ok := t.values[name]
	return ok
}

func (t *ThreadSafeMap) Set(name, value string) {
	t.Lock()
	defer t.Unlock()
	t.values[name] = value
}

func (t *ThreadSafeMap) Get(name string) string {
	t.Lock()
	defer t.Unlock()
	return t.values[name]
}

func (t *ThreadSafeMap) Delete(name string) {
	t.Lock()
	defer t.Unlock()
	delete(t.values, name)
}
