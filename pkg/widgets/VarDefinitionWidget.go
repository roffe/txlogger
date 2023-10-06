package widgets

import (
	"fmt"
	"log"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

type VarDefinitionWidget struct {
	widget.BaseWidget
	pos          int
	symbolName   *widget.Entry
	symbolValue  *widget.Label
	symbolMethod *widget.Select
	symbolNumber *widget.Entry
	//symbolType   *widget.Entry
	//	symbolSigned           *widget.Check
	symbolCorrectionfactor *widget.Entry
	symbolGroup            *widget.Entry
	symbolDeleteBTN        *widget.Button
	objects                []fyne.CanvasObject
}

func NewVarDefinitionWidget(ls *widget.List, definedVars *kwp2000.VarDefinitionList, saveSymbols func(), disabled bool) fyne.Widget {
	vd := &VarDefinitionWidget{}
	vd.symbolName = &widget.Entry{
		OnChanged: func(s string) {
			if definedVars.GetPos(vd.pos).Name != s {
				definedVars.SetName(vd.pos, s)
			}
		},
	}

	vd.symbolValue = &widget.Label{
		Alignment: fyne.TextAlignCenter,
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

	/*
		vd.symbolType = &widget.Entry{
			OnChanged: func(s string) {
				if s == "" {
					return
				}
				if len(s) == 1 {
					s = "0" + s
				}

				if len(s)%2 != 0 {
					return
				}

				decodedHex, err := hex.DecodeString(s)
				if err != nil {
					log.Println(err)
					return
				}
				if len(decodedHex) > 1 {
					if definedVars.GetPos(vd.pos).Type != uint8(decodedHex[0]) {
						definedVars.SetType(vd.pos, uint8(decodedHex[0]))
					}
				}
			},
		}
	*/

	//vd.symbolSigned = widget.NewCheck("", func(b bool) {
	//	//			definedVars[vd.pos].Signed = b
	//})
	//vd.symbolSigned.Disable()

	vd.symbolCorrectionfactor = &widget.Entry{
		OnChanged: func(s string) {
			cf, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return
			}
			if definedVars.GetPos(vd.pos).Correctionfactor != cf {
				definedVars.SetCorrectionfactor(vd.pos, cf)
			}
		},
	}

	vd.symbolGroup = &widget.Entry{
		OnChanged: func(s string) {
			if definedVars.GetPos(vd.pos).Group != s {
				definedVars.SetGroup(vd.pos, s)
			}
		},
	}

	vd.symbolDeleteBTN = widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		//definedVars = append(definedVars[:vd.pos], definedVars[vd.pos+1:]...)
		definedVars.Delete(vd.pos)
		ls.Refresh()
		saveSymbols()
	})

	vd.objects = []fyne.CanvasObject{
		vd.symbolName,
		vd.symbolValue,
		vd.symbolMethod,
		vd.symbolNumber,
		//vd.symbolType,
		//vd.symbolSigned,
		vd.symbolCorrectionfactor,
		vd.symbolDeleteBTN,
	}

	if disabled {
		vd.Disable()
	}

	return vd
}

func (wb *VarDefinitionWidget) Update(pos int, sym *kwp2000.VarDefinition) {
	wb.pos = pos
	wb.symbolName.SetText(sym.Name)
	wb.symbolMethod.SetSelected(sym.Method.String())
	wb.symbolNumber.SetText(strconv.Itoa(sym.Value))
	//wb.symbolType.SetText(fmt.Sprintf("%02X", sym.Type))
	//wb.symbolSigned.SetChecked(sym.Type&kwp2000.SIGNED != 0)
	wb.symbolGroup.SetText(sym.Group)
	switch {
	case sym.Correctionfactor == 1:
		wb.symbolCorrectionfactor.SetText(fmt.Sprintf("%.0f", sym.Correctionfactor))
	case sym.Correctionfactor >= 0.1:
		wb.symbolCorrectionfactor.SetText(fmt.Sprintf("%.01f", sym.Correctionfactor))
	case sym.Correctionfactor >= 0.01:
		wb.symbolCorrectionfactor.SetText(fmt.Sprintf("%.02f", sym.Correctionfactor))
	case sym.Correctionfactor >= 0.001:
		wb.symbolCorrectionfactor.SetText(fmt.Sprintf("%.03f", sym.Correctionfactor))
	default:
		wb.symbolCorrectionfactor.SetText(fmt.Sprintf("%.04f", sym.Correctionfactor))
	}
	sym.SetWidget(wb)
}

func (wb *VarDefinitionWidget) Disable() {
	wb.symbolName.Disable()
	wb.symbolMethod.Disable()
	wb.symbolNumber.Disable()
	//wb.symbolType.Disable()
	//wb.symbolSigned.Disable()
	wb.symbolGroup.Disable()
	wb.symbolCorrectionfactor.Disable()
	wb.symbolDeleteBTN.Disable()

}

func (wb *VarDefinitionWidget) Enable() {
	wb.symbolName.Enable()
	wb.symbolMethod.Enable()
	wb.symbolNumber.Enable()
	//wb.symbolType.Enable()
	//wb.symbolSigned.Enable()
	wb.symbolGroup.Enable()
	wb.symbolCorrectionfactor.Enable()
	wb.symbolDeleteBTN.Enable()

}

func (wb *VarDefinitionWidget) SetName(name string) {
	wb.symbolName.SetText(name)
}

func (wb *VarDefinitionWidget) SetValue(value string) {
	if wb.symbolValue.Text != value {
		wb.symbolValue.SetText(value)
	}
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
	//wb.symbolType.SetText(fmt.Sprintf("%0X", t))
	//wb.symbolSigned.SetChecked(t&kwp2000.SIGNED != 0)
}

func (wb *VarDefinitionWidget) Resize(size fyne.Size) {
	var sz = []float32{
		.35, // name
		.12, // value
		.14, // method
		.12, // number
		//.08, // type
		//.06, // signed
		.11, // correctionfactor
		.08, // deletebtn
	}
	var x float32
	var tw float32
	for _, o := range wb.objects {
		tw += o.MinSize().Width
	}
	for i, o := range wb.objects {
		az := size.Width * sz[i]
		o.Resize(fyne.NewSize(az, size.Height))
		o.Move(fyne.NewPos(x, 0))
		x += o.Size().Width + size.Width*.015
	}
}

func (wb *VarDefinitionWidget) MinSize() fyne.Size {
	wb.ExtendBaseWidget(wb)
	var w, h float32
	for _, o := range wb.objects {
		childSize := o.MinSize()
		if childSize.Height > h {
			h = childSize.Height
		}
		w += childSize.Width
	}
	return fyne.NewSize(w, h)
}

func (wb *VarDefinitionWidget) CreateRenderer() fyne.WidgetRenderer {
	wb.ExtendBaseWidget(wb)
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

// ----

func FixedWidth(width float32, obj fyne.CanvasObject) *fyne.Container {
	return container.New(&fixedWidthContainer{width: width}, obj)
}

type fixedWidthContainer struct {
	width float32
}

func (d *fixedWidthContainer) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objects {
		childSize := o.MinSize()
		if childSize.Height > h {
			h = childSize.Height
		}
	}
	return fyne.NewSize(d.width+theme.Padding()*2, h)
}

func (d *fixedWidthContainer) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	pos := fyne.NewPos(0, 0)
	for _, o := range objects {
		size := o.MinSize()
		o.Move(pos)
		o.Resize(fyne.NewSize(d.width, size.Height))
		pos = pos.Add(fyne.NewPos(d.width, size.Height))
	}
}

/*

type customLayout struct {
	ls *widget.List
}

func (d *customLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for _, o := range objects {
		childSize := o.MinSize()
		if childSize.Height > h {
			h = childSize.Height
		}
		w += childSize.Width
	}
	return fyne.NewSize(w, h)
}

func (d *customLayout) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {

	pp := d.ls.Size().Width / 8
	//pp := float32(80)
	log.Println(pp)
	for i, o := range objects {
		o.Resize(o.MinSize())
		o.Move(fyne.NewPos(float32(i)*pp, 0))
	}
}
*/
