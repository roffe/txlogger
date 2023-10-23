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

type VarDefinitionWidgetEntry struct {
	widget.BaseWidget
	pos          int
	symbolName   *widget.Entry
	symbolValue  *widget.Label
	symbolMethod *widget.Select
	symbolNumber *widget.Entry
	//symbolType   *widget.Entry
	//symbolSigned           *widget.Check
	symbolCorrectionfactor *widget.Entry
	symbolGroup            *widget.Entry
	symbolDeleteBTN        *widget.Button
	container              *fyne.Container
}

func NewVarDefinitionWidgetEntry(ls *widget.List, definedVars *kwp2000.VarDefinitionList, saveSymbols func(), disabled bool) *VarDefinitionWidgetEntry {
	vd := &VarDefinitionWidgetEntry{}
	vd.ExtendBaseWidget(vd)
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
	vd.container = container.NewWithoutLayout(
		vd.symbolName,
		vd.symbolValue,
		vd.symbolMethod,
		vd.symbolNumber,
		//vd.symbolType,
		//vd.symbolSigned,
		vd.symbolCorrectionfactor,
		vd.symbolDeleteBTN,
	)
	if disabled {
		vd.Disable()
	}
	return vd
}

func (wb *VarDefinitionWidgetEntry) Update(pos int, sym *kwp2000.VarDefinition) {
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

func (wb *VarDefinitionWidgetEntry) Disable() {
	wb.symbolName.Disable()
	wb.symbolMethod.Disable()
	wb.symbolNumber.Disable()
	//wb.symbolType.Disable()
	//wb.symbolSigned.Disable()
	wb.symbolGroup.Disable()
	wb.symbolCorrectionfactor.Disable()
	wb.symbolDeleteBTN.Disable()

}

func (wb *VarDefinitionWidgetEntry) Enable() {
	wb.symbolName.Enable()
	wb.symbolMethod.Enable()
	wb.symbolNumber.Enable()
	//wb.symbolType.Enable()
	//wb.symbolSigned.Enable()
	wb.symbolGroup.Enable()
	wb.symbolCorrectionfactor.Enable()
	wb.symbolDeleteBTN.Enable()

}

func (wb *VarDefinitionWidgetEntry) SetName(name string) {
	wb.symbolName.SetText(name)
}

func (wb *VarDefinitionWidgetEntry) SetValue(value string) {
	if wb.symbolValue.Text != value {
		wb.symbolValue.SetText(value)
	}
}

func (wb *VarDefinitionWidgetEntry) SetMethod(method string) {
	wb.symbolMethod.SetSelected(method)
}

func (wb *VarDefinitionWidgetEntry) SetPos(pos int) {
	wb.pos = pos
}

func (wb *VarDefinitionWidgetEntry) SetNumber(number int) {
	wb.symbolNumber.SetText(strconv.Itoa(number))
}

func (wb *VarDefinitionWidgetEntry) SetType(t uint8) {
	//wb.symbolType.SetText(fmt.Sprintf("%0X", t))
	//wb.symbolSigned.SetChecked(t&kwp2000.SIGNED != 0)
}

func (wb *VarDefinitionWidgetEntry) CreateRenderer() fyne.WidgetRenderer {
	return &VarDefinitionWidgetEntryRenderer{
		obj: wb,
	}
}

type VarDefinitionWidgetEntryRenderer struct {
	obj *VarDefinitionWidgetEntry
}

func (vr *VarDefinitionWidgetEntryRenderer) Layout(size fyne.Size) {
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
	for i, o := range vr.obj.container.Objects {
		tw += o.MinSize().Width
		az := size.Width * sz[i]
		o.Resize(fyne.NewSize(az, size.Height))
		o.Move(fyne.NewPos(x, 0))
		x += o.Size().Width + size.Width*.015
	}
}

func (vr *VarDefinitionWidgetEntryRenderer) MinSize() fyne.Size {
	var w, h float32
	for _, o := range vr.obj.container.Objects {
		childSize := o.MinSize()
		if childSize.Height > h {
			h = childSize.Height
		}
		w += childSize.Width
	}
	return fyne.NewSize(w, h)
}

func (vr *VarDefinitionWidgetEntryRenderer) Refresh() {
}

func (vr *VarDefinitionWidgetEntryRenderer) Destroy() {
}

func (vr *VarDefinitionWidgetEntryRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{vr.obj.container}
}
