package widgets

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Sample struct {
	widget.BaseWidget

	minsize   fyne.Size
	container *fyne.Container

	size fyne.Size

	mu sync.Mutex
}

func NewSampleWidget(minSize fyne.Size) *Sample {
	s := &Sample{
		container: container.NewVBox(),
		minsize:   minSize,
	}
	s.ExtendBaseWidget(s)

	text := widget.NewLabel("Sample Widget")
	s.container.Add(text)

	return s
}

func (s *Sample) MinSize() fyne.Size {
	return s.minsize
}

func (s *Sample) Size() fyne.Size {
	return s.container.Size()
}

func (s *Sample) Resize(size fyne.Size) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.size == size {
		return
	}
	s.size = size
	s.container.Resize(size)
}

func (s *Sample) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(s.container)
}
