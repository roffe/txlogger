package layout

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

type MinHeight struct {
	Height float32
}

func (l *MinHeight) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	var y float32
	for _, o := range objects {
		o.Move(fyne.NewPos(0, y))
		y += o.MinSize().Height
		o.Resize(size)
	}
}

func (l *MinHeight) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var height float32
	var width float32
	for _, o := range objects {
		if width < o.MinSize().Width {
			width = o.MinSize().Width
		}
		height += o.MinSize().Height
	}
	return fyne.NewSize(width, max(height, l.Height))
}

type Horizontal struct {
	Offset fyne.CanvasObject
}

func (l *Horizontal) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	var offset float32
	if l.Offset != nil {
		offset = l.Offset.Size().Width
	}
	width := (size.Width - offset) / float32(len(objects))
	for i, o := range objects {
		o.Resize(fyne.NewSize(o.MinSize().Width, o.MinSize().Height))
		o.Move(fyne.NewPos(offset+(float32(i)*width)+(width*.5)-o.MinSize().Width*.5, 0))
	}
}

func (l *Horizontal) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var offset float32
	if l.Offset != nil {
		offset = l.Offset.Size().Width
	}
	var width, height int
	for _, o := range objects {
		width += int(o.MinSize().Width)
		if int(o.MinSize().Height) > height {
			height = int(o.MinSize().Height)
		}
	}
	return fyne.NewSize(float32(width+int(offset)), float32(height))
}

type Vertical struct {
}

func (l *Vertical) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for i, o := range objects {
		height := size.Height / float32(len(objects))
		o.Resize(fyne.NewSize(o.MinSize().Width, o.MinSize().Height))
		o.Move(fyne.NewPos(0, (float32(i)*height)+(height/2)-o.MinSize().Height/2))
	}
}

func (l *Vertical) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var width int
	var height int
	for _, o := range objects {
		if int(o.MinSize().Width) > width {
			width = int(o.MinSize().Width)
		}
		height += int(o.MinSize().Height)

	}
	return fyne.NewSize(float32(width), float32(height))
}

func NewFixedWidth(width float32, obj fyne.CanvasObject) *fyne.Container {
	return container.New(&FixedWidthContainer{width: width}, obj)
}

type FixedWidthContainer struct {
	width float32
}

func (d *FixedWidthContainer) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objects {
		childSize := o.MinSize()
		if childSize.Height > h {
			h += childSize.Height
		}
	}
	return fyne.NewSize(d.width, h)
}

func (d *FixedWidthContainer) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	pos := fyne.NewPos(0, 0)
	for _, o := range objects {
		size := o.MinSize()
		o.Move(pos)
		o.Resize(fyne.NewSize(d.width, size.Height))
		pos = pos.Add(fyne.NewPos(d.width, size.Height))
	}
}

type RatioContainer struct {
	Widths []float32
}

func (d *RatioContainer) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(400, 35)
}

func (d *RatioContainer) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	var x float32
	padd := size.Width * ((1.0 - sumFloat32(d.Widths)) / float32(len(d.Widths)))
	for i, o := range objects {
		width := size.Width * d.Widths[i]
		o.Resize(fyne.NewSize(width, size.Height))
		o.Move(fyne.NewPos(x, 0))
		x += o.Size().Width + padd
	}
}

func sumFloat32(a []float32) float32 {
	var sum float32
	for _, v := range a {
		sum += v
	}
	return sum
}
