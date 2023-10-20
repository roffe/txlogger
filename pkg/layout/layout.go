package layout

import (
	"fyne.io/fyne/v2"
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
}

func (l *Horizontal) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for i, o := range objects {
		//w := size.Width / float32(len(objects))
		ww := (size.Width - 40) / float32(len(objects))
		o.Move(fyne.NewPos(40+(ww*float32(i)), 0))
		o.Resize(fyne.NewSize(ww, o.MinSize().Height))
	}
}

func (l *Horizontal) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var height float32
	var width float32
	for _, o := range objects {
		width += o.MinSize().Width
		if o.MinSize().Height > height {
			height = o.MinSize().Height
		}
	}
	return fyne.NewSize(width+40, height)
}

type Vertical struct {
}

func (l *Vertical) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for i, o := range objects {
		h := size.Height / float32(len(objects))
		o.Move(fyne.NewPos(0, h*float32(i)))
		o.Resize(fyne.NewSize(50, h))
	}
}

func (l *Vertical) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var height float32
	var width float32
	for _, o := range objects {
		height += o.MinSize().Height
		if o.MinSize().Width > width {
			width = o.MinSize().Width
		}
	}
	return fyne.NewSize(width, height)
}
