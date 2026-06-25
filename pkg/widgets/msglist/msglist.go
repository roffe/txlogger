package msglist

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

var _ fyne.Widget = (*MsgList)(nil)

type MsgList struct {
	widget.BaseWidget
	msgs     binding.StringList
	list     *widget.List
	listener binding.DataListener
}

func New(data binding.StringList) *MsgList {
	m := &MsgList{msgs: data}
	m.ExtendBaseWidget(m)

	m.list = widget.NewListWithData(
		data,
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.Selectable = true
			return l
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			txt, err := item.(binding.String).Get()
			if err != nil {
				fyne.LogError("msglist: get string", err)
				return
			}
			obj.(*widget.Label).SetText(txt)
		},
	)

	m.listener = binding.NewDataListener(m.list.ScrollToBottom)

	return m
}

func (m *MsgList) CreateRenderer() fyne.WidgetRenderer {
	m.msgs.AddListener(m.listener)
	return &msgListRenderer{m: m, objects: []fyne.CanvasObject{m.list}}
}

var _ fyne.WidgetRenderer = (*msgListRenderer)(nil)

type msgListRenderer struct {
	m       *MsgList
	objects []fyne.CanvasObject
}

func (r *msgListRenderer) MinSize() fyne.Size           { return fyne.NewSize(300, 200) }
func (r *msgListRenderer) Layout(size fyne.Size)        { r.m.list.Resize(size) }
func (r *msgListRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *msgListRenderer) Refresh()                     { r.m.list.Refresh() }
func (r *msgListRenderer) Destroy()                     { r.m.msgs.RemoveListener(r.m.listener) }
