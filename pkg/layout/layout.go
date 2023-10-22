package layout

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
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
	offset := l.Offset.Size().Width
	width := (size.Width - offset) / float32(len(objects))
	for i, o := range objects {
		o.Resize(fyne.NewSize(o.MinSize().Width, o.MinSize().Height))
		o.Move(fyne.NewPos(offset+(float32(i)*width)+(width/2)-o.MinSize().Width/2, 0))
	}
}

func (l *Horizontal) MinSize(objects []fyne.CanvasObject) fyne.Size {
	offset := l.Offset.Size().Width
	var width float32
	var height float32
	for _, o := range objects {
		width += o.MinSize().Width
		if o.MinSize().Height > height {
			height = o.MinSize().Height
		}
	}
	return fyne.NewSize(width+offset, height)
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
	var width float32
	var height float32
	for _, o := range objects {
		if o.MinSize().Width > width {
			width = o.MinSize().Width
		}
		height += o.MinSize().Height

	}
	return fyne.NewSize(width, height)
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
			h = childSize.Height
		}
	}
	return fyne.NewSize(d.width+theme.Padding()*2, h)
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
