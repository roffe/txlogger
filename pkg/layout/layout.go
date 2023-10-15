package layout

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

type Horizontal struct {
}

func (l *Horizontal) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for i, o := range objects {
		//w := size.Width / float32(len(objects))
		ww := (size.Width - 52) / float32(len(objects))
		o.Move(fyne.NewPos(51+ww*float32(i), 0))
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
	return fyne.NewSize(width+52, height)
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

type Grid struct {
	Xlines []*canvas.Line
	Ylines []*canvas.Line
}

func (g *Grid) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	cw := (size.Width) / float32(len(g.Xlines))
	ch := (size.Height) / float32(len(g.Ylines))
	for i, l := range g.Xlines {
		l.Position1 = fyne.NewPos(float32(i)*cw-1, 0)
		l.Position2 = fyne.NewPos(float32(i)*cw+1, size.Height)
		l.Refresh()
	}
	for i, l := range g.Ylines {
		l.Position1 = fyne.NewPos(0, float32(i)*ch-1)
		l.Position2 = fyne.NewPos(size.Width, float32(i)*ch+1)
		l.Refresh()
	}
}

func (g *Grid) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(100, 100)
}
