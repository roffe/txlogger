package dashboard

import (
	"fmt"
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/roffe/txlogger/pkg/widgets"
)

func knkDetSetter(obj *widgets.Icon) func(float64) {
	var lastVal float64
	return func(value float64) {
		if value == lastVal {
			return
		}
		if value > 0 {
			kn := int(value)
			knockValue := 0
			if kn&1<<24 == 1<<24 {
				knockValue += 1000
			}
			if kn&1<<16 == 1<<16 {
				knockValue += 200
			}
			if kn&1<<8 == 1<<8 {
				knockValue += 30
			}
			if kn&1 == 1 {
				knockValue += 4
			}
			obj.SetText(strconv.Itoa(knockValue))
			obj.Show()
		} else {
			obj.Hide()
		}
	}
}

func showHider(obj fyne.CanvasObject) func(float64) {
	var oldValue float64
	return func(value float64) {
		if value == oldValue {
			return
		}
		if value == 1 {
			obj.Show()
		} else {
			obj.Hide()
		}
	}
}

func ioffSetter(obj *canvas.Text) func(float64) {
	var buf []byte
	var lastVal float64
	return func(value float64) {
		if value == lastVal {
			return
		}
		buf = buf[:0] // clear buffer
		buf = append(buf, "Ioff: "...)
		buf = strconv.AppendFloat(buf, value, 'f', 1, 64)
		buf = append(buf, "°"...)
		obj.Text = string(buf)
		switch {
		case value >= 0:
			obj.Color = color.RGBA{R: 0, G: 0xFF, B: 0, A: 0xFF}
		case value < 0 && value >= -3:
			obj.Color = color.RGBA{R: 0xFF, G: 0xA5, B: 0, A: 0xFF}
		case value < -3:
			obj.Color = color.RGBA{R: 0xFF, G: 0, B: 0, A: 0xFF}
		}
		obj.Refresh()
		lastVal = value
	}
}

func activeAirDemSetter(obj *canvas.Text, f func(float64) string) func(float64) {
	var buf []byte
	var lastVal float64
	return func(value float64) {
		if value == lastVal {
			return
		}
		buf = buf[:0]
		buf = append(buf, f(value)...)
		buf = append(buf, "("...)
		buf = strconv.AppendFloat(buf, value, 'f', 0, 64)
		buf = append(buf, ")"...)
		obj.Text = string(buf)
		obj.Text = string(buf)
		obj.Refresh()
		lastVal = value
	}
}

func textSetter(obj *canvas.Text, text, unit string, precision int) func(float64) {
	var buf []byte
	var lastVal float64
	return func(value float64) {
		if value == lastVal {
			return
		}
		buf = buf[:0]
		buf = append(buf, text...)
		buf = append(buf, ": "...)
		buf = strconv.AppendFloat(buf, value, 'f', precision, 64)
		buf = append(buf, unit...)
		obj.Text = string(buf)
		obj.Refresh()
		lastVal = value
	}
}

func idcSetter(obj *canvas.Text, text string) func(float64) {
	return func(value float64) {
		//		log.Println(value)
		obj.Text = fmt.Sprintf(text+": %02.0f%%", value)
		switch {
		case value > 60 && value < 85:
			obj.Color = color.RGBA{R: 0xFF, G: 0xA5, B: 0, A: 0xFF}
		case value >= 85:
			obj.Color = color.RGBA{R: 0xFF, G: 0, B: 0, A: 0xFF}
		default:
			obj.Color = color.RGBA{R: 0, G: 0xFF, B: 0, A: 0xFF}
		}
		obj.Refresh()
	}
}
