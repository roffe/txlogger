package datalogger

import "sync"

type ThreadSafeMap struct {
	values map[string]string
	sync.Mutex
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
