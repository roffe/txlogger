package msglist

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

var _ fyne.Widget = (*MsgList)(nil)

type MsgList struct {
	widget.BaseWidget
	msgs     binding.StringList
	output   *widget.List
	listener binding.DataListener
}

func New(data binding.StringList) *MsgList {
	m := &MsgList{
		msgs: data,
	}
	m.ExtendBaseWidget(m)

	m.output = widget.NewListWithData(
		m.msgs,
		func() fyne.CanvasObject {
			w := widget.NewLabel("")
			w.Alignment = fyne.TextAlignLeading
			w.Truncation = fyne.TextTruncateEllipsis
			return w
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			i := item.(binding.String)
			txt, err := i.Get()
			if err != nil {
				fyne.LogError("Failed to get string", err)
				return
			}
			obj.(*widget.Label).SetText(txt)
		},
	)

	m.listener = binding.NewDataListener(func() {
		m.output.ScrollToBottom()
	})

	return m
}

func (m *MsgList) CreateRenderer() fyne.WidgetRenderer {
	m.msgs.AddListener(m.listener)
	return &msgListRenderer{
		m:         m,
		container: container.NewVScroll(m.output),
	}
}

var _ fyne.WidgetRenderer = (*msgListRenderer)(nil)

type msgListRenderer struct {
	m         *MsgList
	container *container.Scroll
}

func (r *msgListRenderer) MinSize() fyne.Size {
	return fyne.NewSize(300, 200)
}

func (r *msgListRenderer) Layout(size fyne.Size) {
	r.container.Resize(size)
}

func (r *msgListRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.container}
}

func (r *msgListRenderer) Refresh() {

}

func (r *msgListRenderer) Destroy() {
	r.m.msgs.RemoveListener(r.m.listener)
}

func (r *msgListRenderer) FocusGained() {
	log.Println("FocusGained")
}

func (r *msgListRenderer) FocusLost() {
	log.Println("FocusLost")

}

func (r *msgListRenderer) TypedKey(key *fyne.KeyEvent) {

}

func (r *msgListRenderer) TypedRune(ru rune) {

}
