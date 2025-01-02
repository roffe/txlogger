package dashboard

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/roffe/txlogger/pkg/widgets/icon"
)

func knkDetSetter(icon *icon.Icon) func(float64) {
	var showTime time.Time
	knkStr2 := make([]byte, 4)

	return func(value float64) {
		if value <= 0 && time.Since(showTime) > 5*time.Second {
			icon.Hide()
			return
		}

		if value <= 0 {
			return
		}

		knockValue := uint32(value)
		// log.Printf("knkDetSetter: %08X\n", knockValue)

		knkCyl1 := uint8(knockValue & 0xFF000000 >> 24)
		knkCyl2 := uint8(knockValue & 0xFF0000 >> 16)
		knkCyl3 := uint8(knockValue & 0xFF00 >> 8)
		knkCyl4 := uint8(knockValue & 0xFF)

		if knkCyl1 > 0 {
			knkStr2[0] = '1'
		} else {
			knkStr2[0] = '-'
		}
		if knkCyl2 > 0 {
			knkStr2[1] = '2'

		} else {
			knkStr2[1] = '-'
		}
		if knkCyl3 > 0 {
			knkStr2[2] = '3'
		} else {
			knkStr2[2] = '-'
		}
		if knkCyl4 > 0 {
			knkStr2[3] = '4'
		} else {
			knkStr2[3] = '-'
		}
		icon.SetText(string(knkStr2))
		icon.Show()
		showTime = time.Now()
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
