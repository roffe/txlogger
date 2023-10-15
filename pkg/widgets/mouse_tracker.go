package widgets

import (
	"fmt"
	"log"

	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type MouseTracker struct {
	widget.BaseWidget
	//Cursor *canvas.Rectangle
}

func (m *MouseTracker) MouseIn(_ *desktop.MouseEvent) {
	fmt.Println("Mouse entered!")
}

// MouseMoved is called when the mouse is moved over the widget.
func (m *MouseTracker) MouseMoved(event *desktop.MouseEvent) {
	fmt.Printf("Mouse position: (%.2f, %.2f)\n", event.Position.X, event.Position.Y)
	//m.Cursor.Move(fyne.NewPos(event.Position.X-m.Cursor.Size().Width/2, event.Position.Y-m.Cursor.Size().Height/2))

}

func (m *MouseTracker) MouseOut() {
	fmt.Println("Mouse out!")
}

func (m *MouseTracker) MouseDown(event *desktop.MouseEvent) {
	log.Printf("Mouse position: (%.2f, %.2f) Mouse down: %d", event.Position.X, event.Position.Y, event.Button)

}

func (m *MouseTracker) MouseUp(event *desktop.MouseEvent) {
	log.Printf("Mouse position: (%.2f, %.2f) Mouse down: %d", event.Position.X, event.Position.Y, event.Button)
}
