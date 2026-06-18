// Package liveplotter provides a scope-style widget that plots live EBUS values
// over a sliding time window (default 120s). It auto-follows the newest data and
// lets you scrub back into the retained history of the current pull.
//
// Unlike the logplayer's plotter, which renders an immutable log, the live
// plotter appends one sample per ECU frame: it subscribes to each selected
// symbol (storing the latest value) and to ebus.TOPIC_FRAME, which fires once per
// completed frame carrying that frame's real timestamp. On each frame it snapshots
// every symbol's latest value with the shared timestamp, so all series stay time
// aligned. It reuses the plotter package's primitives (TimeSeries + PlotImage
// rasterizer and the TappableText legend) but owns its own data model and layout.
package liveplotter

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/widgets/plotter"
)

var (
	_ fyne.Widget     = (*Widget)(nil)
	_ fyne.Draggable  = (*Widget)(nil)
	_ fyne.Scrollable = (*Widget)(nil)
)

const defaultWindow = 120 * time.Second

// Config configures a live plotter.
type Config struct {
	// Order is the set of symbol names to plot, in legend order.
	Order []string
	// Window is the sliding time window length. Defaults to 120s when zero.
	Window time.Duration
}

type Widget struct {
	widget.BaseWidget

	order []string
	ts    []*plotter.TimeSeries

	// mu guards the raw sample store and latest values, which are written from
	// the logging goroutine (subscriptions) and read on the UI goroutine (refresh).
	mu     sync.Mutex
	latest map[string]float64
	times  []int64              // frame timestamps, Unix millis, ascending
	values map[string][]float64 // per-series samples, parallel to times
	view   map[string][]float64 // reusable per-render windowed copy (UI goroutine)

	windowMillis int64

	// UI-goroutine-only view state.
	follow       bool
	anchorMillis int64 // right edge timestamp when paused
	lastNewest   int64
	viewCount    int
	hilightLine  int

	// mouse cursor state (UI goroutine only). When mouseInPlot the vertical
	// cursor and the legend track cursorX (plot-relative pixels); otherwise the
	// cursor sits at the right edge and the legend shows the newest/frozen value.
	mouseInPlot bool
	cursorX     float32

	// canvas objects
	canvasImage *canvas.Image
	cursor      *canvas.Line
	overlayText *canvas.Text
	plotObj     fyne.CanvasObject
	zoom        *widget.Slider
	legend      *fyne.Container
	legendTexts []*plotter.TappableText
	legendVals  []float64
	windowSel   *widget.Select
	followBtn   *widget.Button
	split       *container.Split
	root        *fyne.Container

	plotResolution fyne.Size
	size           fyne.Size

	imgBuffers [2]*image.RGBA
	imgIndex   int

	refreshPending atomic.Bool
	// paused mirrors !follow for the logging goroutine (which can't touch the
	// UI-only follow field): while set, onFrame keeps appending but skips
	// compaction so the frozen history is never trimmed until playback resumes.
	paused atomic.Bool

	cancels   []func()
	closeOnce sync.Once
	closed    atomic.Bool
}

func New(cfg *Config) *Widget {
	window := cfg.Window
	if window <= 0 {
		window = defaultWindow
	}

	p := &Widget{
		order:        append([]string(nil), cfg.Order...),
		latest:       make(map[string]float64, len(cfg.Order)),
		values:       make(map[string][]float64, len(cfg.Order)),
		view:         make(map[string][]float64, len(cfg.Order)),
		windowMillis: window.Milliseconds(),
		follow:       true,
		hilightLine:  -1,
		legend:       container.NewVBox(),
		zoom:         widget.NewSlider(1, 100),
		canvasImage:  canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 400, 200))),
		cursor:       canvas.NewLine(color.White),
	}
	p.ExtendBaseWidget(p)

	p.canvasImage.FillMode = canvas.ImageFillContain
	p.canvasImage.ScaleMode = canvas.ImageScaleFastest

	p.zoom.Orientation = widget.Vertical
	p.zoom.Value = 100 // show the whole window by default
	p.zoom.OnChanged = func(float64) { p.scheduleRefresh() }

	p.overlayText = canvas.NewText("", color.White)
	p.overlayText.TextSize = 25

	p.ts = make([]*plotter.TimeSeries, len(p.order))
	p.legendVals = make([]float64, len(p.order))
	for n, name := range p.order {
		p.values[name] = make([]float64, 0, 4096)
		p.ts[n] = plotter.NewSeries(name)
		p.legendTexts = append(p.legendTexts, p.newLegendText(n, name))
		p.legend.Add(p.legendTexts[n])
	}

	p.plotObj = p.canvasImage
	p.subscribe()

	return p
}

func (p *Widget) newLegendText(n int, name string) *plotter.TappableText {
	onTapped := func(enabled bool) {
		p.ts[n].Enabled = enabled
		p.scheduleRefresh()
	}
	onColorUpdate := func(col color.Color) {
		r, g, b, a := col.RGBA()
		p.ts[n].Color = color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
		p.scheduleRefresh()
	}
	onHover := func(hover bool) {
		if hover {
			p.overlayText.Text = name
			p.overlayText.Color = p.ts[n].Color
			p.hilightLine = n
		} else {
			p.overlayText.Text = ""
			p.hilightLine = -1
		}
		p.scheduleRefresh()
	}
	return plotter.NewTappableText(name, p.ts[n].Color, onTapped, onColorUpdate, onHover)
}

// subscribe wires the per-symbol latest-value updates and the frame tick. The
// callbacks run on the logging goroutine and must stay fast, so they only touch
// the guarded store; the redraw is coalesced onto the UI goroutine.
func (p *Widget) subscribe() {
	for _, name := range p.order {
		name := name
		p.cancels = append(p.cancels, ebus.CONTROLLER.SubscribeFunc(name, func(v float64) {
			p.mu.Lock()
			p.latest[name] = v
			p.mu.Unlock()
		}))
	}
	p.cancels = append(p.cancels, ebus.CONTROLLER.SubscribeFunc(ebus.TOPIC_FRAME, p.onFrame))
}

func (p *Widget) onFrame(tMillis float64) {
	if p.closed.Load() {
		return
	}
	p.ingest(int64(tMillis), p.paused.Load())
	p.scheduleRefresh()
}

// ingest appends one frame (the latest value of every series at timestamp t) and
// compacts unless paused. Split out from onFrame so the windowing/retention can
// be tested without a running UI.
func (p *Widget) ingest(t int64, paused bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Guard against out-of-order/duplicate timestamps so times stays ascending.
	if n := len(p.times); n > 0 && t <= p.times[n-1] {
		t = p.times[n-1] + 1
	}
	p.times = append(p.times, t)
	for _, name := range p.order {
		p.values[name] = append(p.values[name], p.latest[name])
	}
	if !paused {
		p.compactLocked()
	}
}

// viewRange returns the [start,end) sample indices to display for the visible
// span. In follow mode the right edge tracks the newest sample; when paused it
// is frozen at anchor, clamped only to the data we still hold (never pushed
// forward). The returned rightT is the clamped right-edge timestamp.
func viewRange(times []int64, follow bool, anchor, span int64) (start, end int, rightT int64) {
	n := len(times)
	newest, oldest := times[n-1], times[0]
	if follow {
		rightT = newest
	} else {
		rightT = anchor
		if rightT > newest {
			rightT = newest
		}
		if rightT < oldest {
			rightT = oldest
		}
	}
	leftT := rightT - span
	if leftT < oldest {
		leftT = oldest
	}
	start = sort.Search(n, func(i int) bool { return times[i] >= leftT })
	end = sort.Search(n, func(i int) bool { return times[i] > rightT })
	if end <= start {
		end = start + 1
	}
	return start, end, rightT
}

// compactLocked drops samples older than the window once the retained span grows
// past 2x the window, so memory stays bounded and the cost is amortized O(1) per
// frame. Caller holds p.mu.
func (p *Widget) compactLocked() {
	n := len(p.times)
	if n < 2 || p.times[n-1]-p.times[0] <= 2*p.windowMillis {
		return
	}
	cutoff := p.times[n-1] - p.windowMillis
	cut := sort.Search(n, func(i int) bool { return p.times[i] >= cutoff })
	if cut <= 0 {
		return
	}
	p.times = append(p.times[:0], p.times[cut:]...)
	for _, name := range p.order {
		s := p.values[name]
		p.values[name] = append(s[:0], s[cut:]...)
	}
}

// scheduleRefresh coalesces redraws onto the UI goroutine: while one is pending,
// further calls only return, so the draw rate is bounded by what the UI can keep
// up with rather than the frame rate (mirrors plotter.Plotter.Seek).
func (p *Widget) scheduleRefresh() {
	if p.closed.Load() {
		return
	}
	if !p.refreshPending.CompareAndSwap(false, true) {
		return
	}
	fyne.Do(func() {
		p.refreshPending.Store(false)
		p.refresh()
	})
}

// spanMillis is the visible time span derived from the zoom slider, clamped to
// the window. Zoom at max shows the whole window; scrolling in shrinks it.
func (p *Widget) spanMillis() int64 {
	span := int64(float64(p.windowMillis) * p.zoom.Value / p.zoom.Max)
	if span > p.windowMillis {
		span = p.windowMillis
	}
	if span < 1000 {
		span = 1000
	}
	return span
}

// refresh recomputes the visible window, copies it out under the lock, and
// redraws. Runs on the UI goroutine.
func (p *Widget) refresh() {
	span := p.spanMillis()

	p.mu.Lock()
	n := len(p.times)
	if n < 2 {
		p.mu.Unlock()
		p.viewCount = 0
		p.drawImage()
		p.canvasImage.Refresh()
		return
	}
	p.lastNewest = p.times[n-1]

	startIdx, endIdx, rightT := viewRange(p.times, p.follow, p.anchorMillis, span)
	if !p.follow {
		p.anchorMillis = rightT
	}
	for _, name := range p.order {
		src := p.values[name][startIdx:endIdx]
		p.view[name] = append(p.view[name][:0], src...)
	}
	p.mu.Unlock()

	p.viewCount = endIdx - startIdx

	// Auto-range unknown signals from the visible window so they stay on screen.
	for _, ts := range p.ts {
		if !ts.Auto || !ts.Enabled {
			continue
		}
		data := p.view[ts.Name]
		if len(data) == 0 {
			continue
		}
		mn, mx := data[0], data[0]
		for _, v := range data {
			if v < mn {
				mn = v
			}
			if v > mx {
				mx = v
			}
		}
		if mn == mx {
			mn -= 1
			mx += 1
		} else {
			pad := (mx - mn) * 0.05
			mn -= pad
			mx += pad
		}
		ts.SetRange(mn, mx)
	}

	p.computeLegendVals()
	p.drawImage()
	p.updateLegendValues()
	p.layoutCursor()
	p.cursor.Refresh()
	p.canvasImage.Refresh()
}

// computeLegendVals fills legendVals from the visible window: the sample under
// the mouse cursor when hovering, otherwise the right edge (newest when live,
// frozen value when paused).
func (p *Widget) computeLegendVals() {
	if p.viewCount <= 0 {
		return
	}
	idx := p.viewCount - 1
	if p.mouseInPlot {
		if plotW := p.plotObj.Size().Width; plotW > 0 {
			frac := float64(p.cursorX) / float64(plotW)
			idx = int(frac*float64(p.viewCount-1) + 0.5)
			if idx < 0 {
				idx = 0
			}
			if idx > p.viewCount-1 {
				idx = p.viewCount - 1
			}
		}
	}
	for i, name := range p.order {
		if v := p.view[name]; idx < len(v) {
			p.legendVals[i] = v[idx]
		}
	}
}

func (p *Widget) updateLegendValues() {
	for i := range p.order {
		newValue := fmt.Sprintf("%.4g", p.legendVals[i])
		obj := p.legendTexts[i]
		if obj.Value() == newValue {
			continue
		}
		obj.SetValue(newValue)
	}
}

// drawImage rasterizes the visible window into a reused buffer via the plotter's
// TimeSeries.PlotImage, exactly like the plotter image backend.
func (p *Widget) drawImage() {
	w, h := int(p.plotResolution.Width), int(p.plotResolution.Height)
	if w <= 0 || h <= 0 {
		return
	}
	p.imgIndex ^= 1
	img := p.imgBuffers[p.imgIndex]
	if img == nil || img.Rect.Dx() != w || img.Rect.Dy() != h {
		img = image.NewRGBA(image.Rect(0, 0, w, h))
		p.imgBuffers[p.imgIndex] = img
	} else {
		clear(img.Pix)
	}

	if p.viewCount >= 2 {
		for n := range p.ts {
			if !p.ts[n].Enabled || p.hilightLine == n {
				continue
			}
			p.ts[n].PlotImage(img, p.view, 0, p.viewCount, 1)
		}
		if p.hilightLine >= 0 && p.ts[p.hilightLine].Enabled {
			p.ts[p.hilightLine].PlotImage(img, p.view, 0, p.viewCount, 4)
		}
	}

	p.canvasImage.Image = img
}

// layoutCursor positions the vertical cursor: under the mouse while hovering the
// plot, otherwise at the right edge as a "now" line.
func (p *Widget) layoutCursor() {
	plotSize := p.plotObj.Size()
	zw := p.zoom.Size().Width
	var x float32
	if p.mouseInPlot {
		x = zw + p.cursorX
	} else {
		x = zw + plotSize.Width - 1
	}
	p.cursor.Position1 = fyne.NewPos(x, 0)
	p.cursor.Position2 = fyne.NewPos(x+1, plotSize.Height)
}

func (p *Widget) setFollow(follow bool) {
	p.follow = follow
	p.paused.Store(!follow)
	// The button is labelled with the action it performs, not the current state:
	// while live it offers Pause; while paused it offers Go Live.
	if follow {
		p.followBtn.SetIcon(theme.MediaPauseIcon())
		p.followBtn.SetText("Pause")
	} else {
		p.anchorMillis = p.lastNewest
		p.followBtn.SetIcon(theme.MediaPlayIcon())
		p.followBtn.SetText("Go Live")
	}
	p.scheduleRefresh()
}

func (p *Widget) Close() {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		for _, cancel := range p.cancels {
			cancel()
		}
		p.cancels = nil
	})
}

func (p *Widget) CreateRenderer() fyne.WidgetRenderer {
	// Initial state is live (following), so the button offers the Pause action.
	p.followBtn = widget.NewButtonWithIcon("Pause", theme.MediaPauseIcon(), func() {
		p.setFollow(!p.follow)
	})

	p.windowSel = widget.NewSelect([]string{"30s", "60s", "120s", "300s"}, func(s string) {
		var d time.Duration
		switch s {
		case "30s":
			d = 30 * time.Second
		case "60s":
			d = 60 * time.Second
		case "120s":
			d = 120 * time.Second
		case "300s":
			d = 300 * time.Second
		}
		if d > 0 {
			p.mu.Lock()
			p.windowMillis = d.Milliseconds()
			p.mu.Unlock()
			p.scheduleRefresh()
		}
	})
	p.windowSel.Selected = fmt.Sprintf("%ds", p.windowMillis/1000)

	toggleVisibleBtn := widget.NewButton("Toggle visible", func() {
		for _, ts := range p.legendTexts {
			ts.Tapped(&fyne.PointEvent{})
		}
	})

	leading := container.NewBorder(
		nil, nil, p.zoom, nil,
		container.New(&livePlotLayout{p: p}, p.plotObj),
	)
	p.split = container.NewHSplit(
		leading,
		container.NewBorder(
			nil, toggleVisibleBtn, nil, nil,
			container.NewVScroll(p.legend),
		),
	)
	p.split.Offset = 0.90

	controls := container.NewHBox(
		p.followBtn,
		widget.NewLabel("Window"),
		p.windowSel,
	)

	p.root = container.NewBorder(nil, controls, nil, nil, p.split)

	return &liveRenderer{p: p}
}

type liveRenderer struct {
	p       *Widget
	objects []fyne.CanvasObject
}

func (r *liveRenderer) MinSize() fyne.Size { return r.p.root.MinSize() }

func (r *liveRenderer) Layout(size fyne.Size) {
	if r.p.size == size {
		return
	}
	r.p.size = size
	r.p.root.Resize(size)
}

func (r *liveRenderer) Refresh() {}

func (r *liveRenderer) Destroy() { r.p.Close() }

func (r *liveRenderer) Objects() []fyne.CanvasObject {
	if r.objects == nil {
		r.objects = []fyne.CanvasObject{r.p.root, r.p.overlayText, r.p.cursor}
	}
	return r.objects
}

// livePlotLayout sizes the plot canvas and recomputes the render resolution and
// cursor whenever the plot area changes (mirrors plotter.plotLayout).
type livePlotLayout struct {
	p       *Widget
	oldSize fyne.Size
}

func (t *livePlotLayout) Layout(_ []fyne.CanvasObject, plotSize fyne.Size) {
	if t.oldSize == plotSize {
		return
	}
	t.oldSize = plotSize

	t.p.overlayText.Move(fyne.NewPos(t.p.zoom.Size().Width, 20))
	t.p.plotObj.Resize(plotSize)
	t.p.plotResolution = fyne.NewSize(plotSize.Width, plotSize.Height)
	t.p.refresh()
	t.p.layoutCursor()
	t.p.cursor.Refresh()
}

func (t *livePlotLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(400, 100)
}
