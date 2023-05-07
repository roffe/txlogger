package widgets

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/t7logger/pkg/kwp2000"
)

type VarDefinitionWidget struct {
	widget.BaseWidget
	pos                    int
	symbolName             *widget.Entry
	symbolMethod           *widget.Select
	symbolNumber           *widget.Entry
	symbolType             *widget.Entry
	symbolSigned           *widget.Check
	symbolCorrectionfactor *widget.Entry
	symbolDeleteBTN        *widget.Button
	objects                []fyne.CanvasObject
}

func NewVarDefinitionWidget(ls *widget.List, definedVars *kwp2000.VarDefinitionList) fyne.Widget {
	vd := &VarDefinitionWidget{}

	vd.symbolName = &widget.Entry{
		PlaceHolder: strings.Repeat(" ", 30),
		OnChanged: func(s string) {
			if definedVars.GetPos(vd.pos).Name != s {
				definedVars.SetName(vd.pos, s)
			}
		},
	}

	vd.symbolMethod = widget.NewSelect([]string{"Address", "Local ID", "Symbol"}, func(s string) {
		if definedVars.GetPos(vd.pos).Method.String() != s {
			switch s {
			case "Address":
				definedVars.SetMethod(vd.pos, kwp2000.VAR_METHOD_ADDRESS)
			case "Local ID":
				definedVars.SetMethod(vd.pos, kwp2000.VAR_METHOD_LOCID)
			case "Symbol":
				definedVars.SetMethod(vd.pos, kwp2000.VAR_METHOD_SYMBOL)
			}
		}
	})

	vd.symbolNumber = &widget.Entry{
		OnChanged: func(s string) {
			v, err := strconv.Atoi(s)
			if err != nil {
				log.Println(err)
				return
			}
			if definedVars.GetPos(vd.pos).Value != v {
				definedVars.SetValue(vd.pos, v)
			}

		},
	}

	vd.symbolType = widget.NewEntry()

	vd.symbolSigned = widget.NewCheck("Signed", func(b bool) {
		//			definedVars[vd.pos].Signed = b
	})
	vd.symbolSigned.Disable()

	vd.symbolCorrectionfactor = &widget.Entry{
		OnChanged: func(s string) {
			if definedVars.GetPos(vd.pos).Correctionfactor != s {
				definedVars.SetCorrectionfactor(vd.pos, s)
			}
		},
	}

	vd.symbolDeleteBTN = widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
		//definedVars = append(definedVars[:vd.pos], definedVars[vd.pos+1:]...)
		definedVars.Delete(vd.pos)
		ls.Refresh()
	})
	vd.objects = []fyne.CanvasObject{
		container.NewHBox(
			maxWidth(250, vd.symbolName),
			maxWidth(100, vd.symbolMethod),
			maxWidth(100, vd.symbolNumber),
			maxWidth(100, vd.symbolType),
			maxWidth(100, vd.symbolSigned),
			maxWidth(150, vd.symbolCorrectionfactor),
			maxWidth(100, vd.symbolDeleteBTN),
		),
	}

	return vd
}

func maxWidth(width float32, obj fyne.CanvasObject) *fyne.Container {
	return container.New(&diagonal{width: width}, obj)
}

type diagonal struct {
	width float32
}

func (d *diagonal) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objects {
		childSize := o.MinSize()
		//w += childSize.Width
		if childSize.Height > h {
			h = childSize.Height
		}
	}
	return fyne.NewSize(d.width+theme.Padding()*2, h)
}

func (d *diagonal) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	pos := fyne.NewPos(0, containerSize.Height-d.MinSize(objects).Height)
	for _, o := range objects {
		size := o.MinSize()
		o.Resize(fyne.NewSize(d.width, size.Height))
		o.Move(pos)
		pos = pos.Add(fyne.NewPos(d.width, size.Height))
	}
}

func (wb *VarDefinitionWidget) Update(pos int, sym *kwp2000.VarDefinition) {
	wb.pos = pos
	wb.symbolName.SetText(sym.Name)
	wb.symbolMethod.SetSelected(sym.Method.String())
	wb.symbolNumber.SetText(strconv.Itoa(sym.Value))
	wb.symbolType.SetText(fmt.Sprintf("%X", sym.Type))
	wb.symbolSigned.SetChecked(sym.Type&kwp2000.SIGNED != 0)
	wb.symbolCorrectionfactor.SetText(sym.Correctionfactor)
}

func (wb *VarDefinitionWidget) SetName(name string) {
	wb.symbolName.SetText(name)
}

func (wb *VarDefinitionWidget) SetMethod(method string) {
	wb.symbolMethod.SetSelected(method)
}

func (wb *VarDefinitionWidget) SetPos(pos int) {
	wb.pos = pos
}

func (wb *VarDefinitionWidget) SetNumber(number int) {
	wb.symbolNumber.SetText(strconv.Itoa(number))
}

func (wb *VarDefinitionWidget) SetType(t uint8) {
	wb.symbolType.SetText(fmt.Sprintf("%X", t))
	wb.symbolSigned.SetChecked(t&kwp2000.SIGNED != 0)
}

func (wb *VarDefinitionWidget) MinSize() fyne.Size {
	var w, h float32
	for _, o := range []fyne.CanvasObject{
		wb.symbolName,
		wb.symbolMethod,
		wb.symbolNumber,
		wb.symbolType,
		wb.symbolSigned,
		wb.symbolCorrectionfactor,
		wb.symbolDeleteBTN,
	} {
		ms := o.MinSize()
		w += o.MinSize().Width + theme.Padding()*2
		if ms.Height > h {
			h = ms.Height
		}
	}
	return fyne.NewSize(900+(theme.Padding()*20), h)
}

func (wb *VarDefinitionWidget) CreateRenderer() fyne.WidgetRenderer {
	return &varRenderer{
		obj: wb,
	}
}

type varRenderer struct {
	obj *VarDefinitionWidget
}

func (vr *varRenderer) Layout(size fyne.Size) {
}

func (vr *varRenderer) MinSize() fyne.Size {
	return vr.obj.MinSize()
}

func (vr *varRenderer) Refresh() {

}

func (vr *varRenderer) Destroy() {
}

func (vr *varRenderer) Objects() []fyne.CanvasObject {
	return vr.obj.objects
}
