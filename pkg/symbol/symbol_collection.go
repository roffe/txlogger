package symbol

import "sync"

type SymbolCollection interface {
	GetByName(name string) *Symbol
	GetByNumber(number int) *Symbol
	Symbols() []*Symbol
	Count() int
	Add(symbols ...*Symbol)
}

type Collection struct {
	symbols   []*Symbol
	nameMap   map[string]*Symbol
	numberMap map[int]*Symbol

	count int
	mu    sync.Mutex
}

func NewCollection(symbols ...*Symbol) SymbolCollection {
	c := &Collection{
		symbols:   symbols,
		nameMap:   make(map[string]*Symbol),
		numberMap: make(map[int]*Symbol),
	}
	for _, s := range symbols {
		c.nameMap[s.Name] = s
		c.numberMap[s.Number] = s
		c.count++
	}
	return c
}

func (c *Collection) GetByName(name string) *Symbol {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nameMap[name]
}

func (c *Collection) GetByNumber(number int) *Symbol {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.numberMap[number]
}

func (c *Collection) Add(symbols ...*Symbol) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.symbols = append(c.symbols, symbols...)
	for _, s := range symbols {
		c.nameMap[s.Name] = s
		c.numberMap[s.Number] = s
		c.count++
	}
}

func (c *Collection) Symbols() []*Symbol {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*Symbol, len(c.symbols))
	copy(out, c.symbols)
	return out
}

func (c *Collection) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}
