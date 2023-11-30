package symbol

import (
	"fmt"
	"strings"
	"sync"
)

type SymbolCollection interface {
	GetByName(name string) *Symbol
	GetByNumber(number int) *Symbol
	GetXYZ(xAxis, yAxis, zAxis string) ([]int, []int, []int, float64, float64, float64, error)
	Symbols() []*Symbol
	Dump() string
	Count() int
	Add(symbols ...*Symbol)
	Save(filename string) error
}

type Collection struct {
	symbols   []*Symbol
	nameMap   map[string]*Symbol
	numberMap map[int]*Symbol
	count     int
	mu        sync.Mutex
}

func NewCollection(symbols ...*Symbol) *Collection {
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

func (c *Collection) Save(filename string) error {
	return nil
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
	//out := make([]*Symbol, len(c.symbols))
	//copy(out, c.symbols)
	//return out
	return c.symbols
}

func (c *Collection) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

func (c *Collection) Dump() string {
	var out strings.Builder
	for _, s := range c.symbols {
		out.WriteString(s.String())
		out.WriteString("\n")
	}
	return out.String()
}

func (c *Collection) GetXYZ(xAxis, yAxis, zAxis string) ([]int, []int, []int, float64, float64, float64, error) {
	symx, symy, symz := c.GetByName(xAxis), c.GetByName(yAxis), c.GetByName(zAxis)
	if symz == nil {
		return nil, nil, nil, 0, 0, 0, fmt.Errorf("%s not found", zAxis)
	}

	//	log.Println("symz", symz.Bytes())

	zOut := symz.Ints()

	var xOut, yOut []int
	xFac, yFac := 1.0, 1.0
	if symx == nil {
		xOut = []int{0}
	} else {
		xOut = symx.Ints()
		xFac = symx.Correctionfactor
	}

	if symy == nil {
		if symx == nil {
			yOut = make([]int, len(zOut))
		} else {
			yOut = []int{0}
		}
	} else {
		yOut = symy.Ints()
		yFac = symy.Correctionfactor
	}

	if len(xOut) >= 1 || len(yOut) >= 1 {
		return xOut, yOut, zOut, xFac, yFac, symz.Correctionfactor, nil
	}
	checks := map[string]*Symbol{
		xAxis: symx,
		yAxis: symy,
		zAxis: symz,
	}
	for k, v := range checks {
		if v == nil {
			return nil, nil, nil, 0, 0, 0, fmt.Errorf("failed to find %s", k)
		}
	}

	return nil, nil, nil, 0, 0, 0, fmt.Errorf("failed to convert x:%s y:%s z:%s", xAxis, yAxis, zAxis)
}

/*
	var x, y, z []int
	if symx.Type&SIGNED == 1 {
		x = symx.DataToInt16()
	} else {
		x = symx.DataToUint16()
	}
	if symy.Type&SIGNED == 1 {
		y = symy.DataToInt16()
	} else {
		y = symy.DataToUint16()
	}

	if len(x)*len(y) == len(symz.Bytes()) {
		if symz.Type&SIGNED == 1 {
			for _, v := range symz.DataToInt8() {
				z = append(z, int(v))
			}
		} else {
			for _, v := range symz.DataToUint8() {
				z = append(z, int(v))
			}
		}
		log.Println("3x")
		return x, y, z, symx.Correctionfactor, symy.Correctionfactor, symz.Correctionfactor, nil
	}

	if len(x)*len(y) == int(symz.Length/2) {
		data := make([]int16, symz.Length/2)
		reader := bytes.NewReader(symz.Bytes())
		if err := binary.Read(reader, binary.BigEndian, &data); err != nil {
			log.Fatalf("Failed to convert zData to int16 slice: %v", err)
		}
		for _, v := range data {
			z = append(z, int(v))
		}
		log.Println("4x")
		return x, y, z, symx.Correctionfactor, symy.Correctionfactor, symz.Correctionfactor, nil
	}
	log.Println("189x")
	return nil, nil, nil, 0, 0, 0, fmt.Errorf("failed to convert %s %s %s", xAxis, yAxis, zAxis)
*/
