package kwp2000

import (
	"sync"

	"github.com/roffe/txlogger/pkg/symbol"
)

type VarDefinitionList struct {
	data []*VarDefinition
	//updateChan chan struct{}
	mu sync.Mutex
}

func NewVarDefinitionList() *VarDefinitionList {
	return &VarDefinitionList{
		data: make([]*VarDefinition, 0),
		//updateChan: make(chan struct{}, 1),
	}
}

//func (v *VarDefinitionList) Update() chan struct{} {
//	return v.updateChan
//}

func (v *VarDefinitionList) Len() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.data)
}

func (v *VarDefinitionList) Add(def *VarDefinition) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data = append(v.data, def)
	//v.updated()
}

func (v *VarDefinitionList) GetPos(i int) *VarDefinition {
	return v.data[i]
}

func (v *VarDefinitionList) Get() []*VarDefinition {
	return v.data
}

func (v *VarDefinitionList) Set(content []*VarDefinition) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data = content
	//v.updated()
}

func (v *VarDefinitionList) SetName(pos int, name string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data[pos].Name = name
	//v.updated()
}

func (v *VarDefinitionList) SetMethod(pos int, method Method) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data[pos].Method = method
	//v.updated()
}

func (v *VarDefinitionList) SetType(pos int, value uint8) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data[pos].Type = value
	//v.updated()
}

func (v *VarDefinitionList) SetValue(pos, value int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data[pos].Value = value
	//v.updated()
}

func (v *VarDefinitionList) SetGroup(pos int, value string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data[pos].Group = value
	//v.updated()
}

func (v *VarDefinitionList) SetCorrectionfactor(pos int, correctionfactor float64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data[pos].Correctionfactor = correctionfactor
	//v.updated()
}

func (v *VarDefinitionList) Delete(pos int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data = append(v.data[:pos], v.data[pos+1:]...)
	//v.updated()
}

func (v *VarDefinitionList) UpdatePos(i int, sym *VarDefinition) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data[i].Name = sym.Name
	v.data[i].Method = sym.Method
	v.data[i].Value = sym.Value
	v.data[i].Type = sym.Type
	v.data[i].Length = sym.Length
	v.data[i].Unit = sym.Unit
	v.data[i].Correctionfactor = sym.Correctionfactor
	v.data[i].Unit = symbol.GetUnit(sym.Name)
	//v.updated()
}

//func (v *VarDefinitionList) updated() {
//	//	_, file, no, ok := runtime.Caller(1)
//	//	if ok {
//	//		log.Printf("called from %s#%d\n", file, no)
//	//	}
//	//log.Println("VarDefinitionList updated")
//	select {
//	case v.updateChan <- struct{}{}:
//	default:
//	}
//}
