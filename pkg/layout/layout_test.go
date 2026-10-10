package layout

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/stretchr/testify/assert"
)

// box is a canvas object with a fixed minimum size.
func box(w, h float32) *canvas.Rectangle {
	r := canvas.NewRectangle(color.Black)
	r.SetMinSize(fyne.NewSize(w, h))
	return r
}

// MinHeight stacks objects by their minimum heights, sizes every object to the
// full container, and is at least Height tall.
func TestMinHeight(t *testing.T) {
	a, b := box(30, 10), box(50, 20)
	l := &MinHeight{Height: 100}

	assert.Equal(t, fyne.NewSize(50, 100), l.MinSize([]fyne.CanvasObject{a, b}))
	l.Height = 5
	assert.Equal(t, fyne.NewSize(50, 30), l.MinSize([]fyne.CanvasObject{a, b}))

	l.Layout([]fyne.CanvasObject{a, b}, fyne.NewSize(200, 300))
	assert.Equal(t, fyne.NewPos(0, 0), a.Position())
	assert.Equal(t, fyne.NewPos(0, 10), b.Position())
	assert.Equal(t, fyne.NewSize(200, 300), a.Size())
	assert.Equal(t, fyne.NewSize(200, 300), b.Size())

	assert.Equal(t, fyne.NewSize(0, 5), l.MinSize(nil))
}

// Horizontal centres each object, at its minimum size, in an equal-width
// column of the space to the right of Offset.
func TestHorizontal(t *testing.T) {
	test.NewTempApp(t)
	a, b := box(20, 10), box(40, 16)
	l := &Horizontal{}
	objs := []fyne.CanvasObject{a, b}

	assert.Equal(t, fyne.NewSize(60, 16), l.MinSize(objs))
	l.Layout(objs, fyne.NewSize(200, 50))
	// Columns are 100 wide: a centred at 50, b centred at 150.
	assert.Equal(t, fyne.NewPos(40, 0), a.Position())
	assert.Equal(t, fyne.NewPos(130, 0), b.Position())
	assert.Equal(t, fyne.NewSize(20, 10), a.Size())
	assert.Equal(t, fyne.NewSize(40, 16), b.Size())

	// With an Offset the columns start after it and MinSize includes it.
	offset := box(0, 0)
	offset.Resize(fyne.NewSize(40, 1))
	l.Offset = offset
	assert.Equal(t, fyne.NewSize(100, 16), l.MinSize(objs))
	l.Layout(objs, fyne.NewSize(240, 50))
	assert.Equal(t, fyne.NewPos(40+50-10, 0), a.Position())
	assert.Equal(t, fyne.NewPos(40+150-20, 0), b.Position())
}

// Vertical centres each object, at its minimum size, in an equal-height row,
// aligned to the left edge.
func TestVertical(t *testing.T) {
	a, b, c := box(20, 10), box(40, 16), box(8, 4)
	l := &Vertical{}
	objs := []fyne.CanvasObject{a, b, c}

	assert.Equal(t, fyne.NewSize(40, 30), l.MinSize(objs))
	l.Layout(objs, fyne.NewSize(100, 300))
	// Rows are 100 tall, centred at 50, 150 and 250.
	assert.Equal(t, fyne.NewPos(0, 45), a.Position())
	assert.Equal(t, fyne.NewPos(0, 142), b.Position())
	assert.Equal(t, fyne.NewPos(0, 248), c.Position())
	assert.Equal(t, fyne.NewSize(40, 16), b.Size())

	assert.Equal(t, fyne.NewSize(0, 0), l.MinSize(nil))
}

// NewFixedWidth gives its object the fixed width at the object's own minimum
// height, whatever the container's size, and reports that as its MinSize.
func TestFixedWidth(t *testing.T) {
	test.NewTempApp(t)
	obj := box(300, 24)
	c := NewFixedWidth(120, obj)

	assert.Equal(t, fyne.NewSize(120, 24), c.MinSize())
	c.Resize(fyne.NewSize(400, 100))
	assert.Equal(t, fyne.NewPos(0, 0), obj.Position())
	assert.Equal(t, fyne.NewSize(120, 24), obj.Size())

	// It holds the width inside an HBox, where it gets exactly its MinSize.
	row := container.NewHBox(c, box(10, 10))
	row.Resize(fyne.NewSize(1000, 50))
	assert.Equal(t, float32(120), c.Size().Width)
}

// VPortion splits the container's height between the objects by portion,
// minus the padding between them.
func TestVPortionUsesHeight(t *testing.T) {
	test.NewTempApp(t)
	a, b := box(10, 10), box(10, 10)
	cont := container.New(NewVPortion([]float64{1, 3}), a, b)
	cont.Resize(fyne.NewSize(100, 400))

	avail := 400 - theme.Padding()
	assert.Equal(t, avail/4, a.Size().Height)
	assert.Equal(t, avail*3/4, b.Size().Height)
	assert.Equal(t, avail/4+theme.Padding(), b.Position().Y)
}

// HPortion and VPortion report a MinSize of just the padding when no object
// has a minimum extent along their axis.
func TestPortionMinSizeZeroObjects(t *testing.T) {
	test.NewTempApp(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MinSize panicked: %v", r)
		}
	}()
	pad := theme.Padding()
	objs := []fyne.CanvasObject{box(0, 8), box(0, 12)}
	assert.Equal(t, fyne.NewSize(pad, 12), NewHPortion([]float64{1, 1}).MinSize(objs))
	objs = []fyne.CanvasObject{box(8, 0), box(12, 0)}
	assert.Equal(t, fyne.NewSize(12, pad), NewVPortion([]float64{1, 1}).MinSize(objs))
}

// HPortion's (VPortion's) MinSize is the widest (tallest) object scaled up by
// the inverse of its portion, plus padding, so that object gets at least its
// minimum extent along the axis.
func TestPortionMinSizeScales(t *testing.T) {
	test.NewTempApp(t)
	objs := []fyne.CanvasObject{box(30, 5), box(10, 9)}
	got := NewHPortion([]float64{1, 2}).MinSize(objs)
	assert.Equal(t, fyne.NewSize(30*3+theme.Padding(), 9), got)

	objs = []fyne.CanvasObject{box(5, 30), box(9, 10)}
	got = NewVPortion([]float64{1, 2}).MinSize(objs)
	assert.Equal(t, fyne.NewSize(9, 30*3+theme.Padding()), got)
}
