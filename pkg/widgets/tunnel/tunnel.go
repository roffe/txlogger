// Package tunnel renders an endless psychedelic 3D tunnel ride entirely on the
// GPU with a single canvas.Shader, modelled on the meshgrid shader backend.
// The viewer rides a retrowave wiremesh vortex like a rollercoaster; all motion
// comes from the "time" uniform that canvas.NewShaderAnimation advances, so the
// CPU does no per-frame work.
package tunnel

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

var _ fyne.Widget = (*Tunnel)(nil)

// defaultSize is the requested 500x500 viewport; the widget keeps it as its
// minimum and fills whatever a container hands it, staying circular via the
// short-axis normalisation in the shader.
const defaultSize = 700

type Tunnel struct {
	widget.BaseWidget

	shader *canvas.Shader
	anim   *fyne.Animation

	running bool
}

// New builds a tunnel widget. The animation starts automatically once the
// widget is shown (CreateRenderer) and stops when it is removed (Destroy).
func New() *Tunnel {
	t := &Tunnel{}
	t.ExtendBaseWidget(t)
	t.initShader()
	t.anim = canvas.NewShaderAnimation(t.shader)
	return t
}

// Start begins (or resumes) the ride. Safe to call repeatedly.
func (t *Tunnel) Start() {
	if t.running || t.anim == nil {
		return
	}
	t.running = true
	t.anim.Start()
}

// Stop freezes the tunnel on its current frame.
func (t *Tunnel) Stop() {
	if !t.running || t.anim == nil {
		return
	}
	t.running = false
	t.anim.Stop()
}

// SetSpeed scales how fast the viewer rides into the tunnel.
func (t *Tunnel) SetSpeed(speed float32) {
	if t.shader == nil {
		return
	}
	t.shader.Uniforms["speed"] = speed
	t.shader.Refresh()
}

// SetLogoScale sets the bouncing logo's half-size in short-axis units (the
// short axis spans -1..1), e.g. 0.30 makes the logo ~30% of half the viewport.
func (t *Tunnel) SetLogoScale(scale float32) {
	if t.shader == nil {
		return
	}
	t.shader.Uniforms["logo_scale"] = scale
	t.shader.Refresh()
}

func (t *Tunnel) CreateRenderer() fyne.WidgetRenderer {
	t.Start()
	return &tunnelRenderer{t: t}
}

type tunnelRenderer struct {
	t *Tunnel
}

func (r *tunnelRenderer) Layout(size fyne.Size) {
	r.t.shader.Resize(size)
}

func (r *tunnelRenderer) MinSize() fyne.Size {
	return fyne.NewSize(defaultSize, defaultSize)
}

func (r *tunnelRenderer) Refresh() {
	r.t.shader.Refresh()
}

func (r *tunnelRenderer) Destroy() {
	r.t.Stop()
}

func (r *tunnelRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.t.shader}
}
