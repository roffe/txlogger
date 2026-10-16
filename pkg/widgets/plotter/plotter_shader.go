package plotter

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"log"
	"sync/atomic"

	"fyne.io/fyne/v2/canvas"
)

// plotShaderHLSL is the same shader ported to HLSL for Fyne's Direct3D 11
// driver, which is only built with the `directx` tag. Carrying it
// unconditionally costs an unreferenced blob on OpenGL targets and keeps the
// widget free of build tags; see the file header for the porting notes.
//
//go:embed plotter.hlsl
var plotShaderHLSL []byte

// GPU renderer: the whole plot is drawn by a single canvas.Shader. The log is
// immutable once loaded, so every sample is uploaded to the GPU exactly once
// (16-bit packed, one texel per sample) together with a small min/max
// decimation level; after that a playback seek, zoom or drag only updates a
// couple of float uniforms and the fragment shader re-renders the view. The
// CPU never rasterizes a plot frame again - compare drawImage, which redraws
// and re-uploads the full plot image on every coalesced Seek.
//
// Per pixel the shader walks the enabled series; when zoomed in it computes
// the anti-aliased distance to the polyline segments crossing the pixel's
// column, when zoomed out it min/maxes the samples covering the column (via
// the 16:1 min/max texture when even that is too many fetches) and draws the
// vertical run, mirroring plotImageDecimated. Series are combined with the
// same order-independent max-blend as bresenhamCore.
//
// Sample encoding: values are normalized to the series' display range
// [Min, Max] with one full range of headroom on each side - texel 0..1 spans
// [Min-range, Max+range] - so overshooting samples keep their slope off
// screen like the Bresenham clipping does instead of flattening at the plot
// edge. 16 bits give the on-screen third ~21800 steps, far below a pixel.

// plotShaderSeq makes each widget's Shader.Name unique: the painter caches
// the compiled program and its textures per name, so two plotters sharing a
// name would evict each other's data textures every frame.
var plotShaderSeq atomic.Int64

const (
	// plotTexW caps the data-texture width; sample index s of series i lives
	// at texel (s mod plotTexW, i*rowsPerSeries + s/plotTexW).
	plotTexW = 4096
	// plotTexMaxH bounds texture heights to what every desktop GPU handles.
	plotTexMaxH = 4096
	// plotMaxSeries matches MAX_SERIES in the shader source.
	plotMaxSeries = 64
	// mmGroup is the min/max decimation factor, matching the shader's
	// group-16 lookups.
	mmGroup = 16
)

const plotShaderPreludeGL = "#version 110\n"

const plotShaderPreludeES = `#version 100
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif
`

//go:embed plotter.frag
var plotShaderBody string

// initShader builds the shader object and uploads the immutable log data.
// It reports false when the data does not fit the GPU layout (no series, too
// many series, or a log too long for the texture budget); the caller then
// stays on the image backend.
func (p *Plotter) initShader() bool {
	log.Println("Init plotter shaders")

	if len(p.ts) == 0 || len(p.ts) > plotMaxSeries {
		return false
	}

	maxLen := 0
	for _, ts := range p.ts {
		if ts == nil {
			return false
		}
		maxLen = max(maxLen, len(p.values[ts.Name]))
	}
	if maxLen < 2 {
		return false
	}

	texW := min(maxLen, plotTexW)
	rowsRaw := (maxLen + texW - 1) / texW
	groups := (maxLen + mmGroup - 1) / mmGroup
	rowsMM := (groups + texW - 1) / texW
	if len(p.ts)*rowsRaw > plotTexMaxH || len(p.ts)*rowsMM > plotTexMaxH {
		return false
	}

	p.shader = canvas.NewShader(
		fmt.Sprintf("plotter-%d", plotShaderSeq.Add(1)),
		[]byte(plotShaderPreludeGL+plotShaderBody),
		[]byte(plotShaderPreludeES+plotShaderBody),
	)
	p.shader.SourceHLSL = plotShaderHLSL
	p.shader.Textures = make(map[string]image.Image, 3)
	p.shader.Uniforms = make(map[string]float32, 16)

	data := image.NewRGBA(image.Rect(0, 0, texW, len(p.ts)*rowsRaw))
	mm := image.NewRGBA(image.Rect(0, 0, texW, len(p.ts)*rowsMM))
	for i, ts := range p.ts {
		encodeSeries(data, mm, texW, i*rowsRaw, i*rowsMM, ts, p.values[ts.Name])
	}
	p.shader.Textures["data_tex"] = data
	p.shader.Textures["mm_tex"] = mm

	u := p.shader.Uniforms
	u["series_count"] = float32(len(p.ts))
	u["tex_w"] = float32(texW)
	u["rows_raw"] = float32(rowsRaw)
	u["rows_mm"] = float32(rowsMM)
	u["data_h"] = float32(len(p.ts) * rowsRaw)
	u["mm_h"] = float32(len(p.ts) * rowsMM)
	u["meta_h"] = float32(len(p.ts))

	p.updateShaderMeta()
	p.updateShaderView()
	return true
}

// encodeValue maps a sample to the 16-bit texel value: 0..1 spans
// [Min-range, Max+range] so overshoot keeps its slope (clamped a full plot
// height off screen).
func encodeValue(ts *TimeSeries, v float64) uint16 {
	r := ts.valueRange
	if r <= 0 {
		r = 1
	}
	norm := ((v-ts.Min)/r + 1) / 3
	norm = min(1, max(0, norm))
	return uint16(norm*65535 + 0.5)
}

// encodeSeries packs one series' samples (and its 16:1 min/max groups) into
// the texture rows starting at rawRow/mmRow.
func encodeSeries(data, mm *image.RGBA, texW, rawRow, mmRow int, ts *TimeSeries, values []float64) {
	for s, v := range values {
		q := encodeValue(ts, v)
		data.SetRGBA(s%texW, rawRow+s/texW, color.RGBA{R: uint8(q >> 8), G: uint8(q), A: 0xff})
	}
	for g := 0; g*mmGroup < len(values); g++ {
		end := min((g+1)*mmGroup, len(values))
		lo, hi := values[g*mmGroup], values[g*mmGroup]
		for _, v := range values[g*mmGroup+1 : end] {
			lo = min(lo, v)
			hi = max(hi, v)
		}
		ql, qh := encodeValue(ts, lo), encodeValue(ts, hi)
		mm.SetRGBA(g%texW, mmRow+g/texW, color.RGBA{
			R: uint8(ql >> 8), G: uint8(ql),
			B: uint8(qh >> 8), A: uint8(qh),
		})
	}
}

// updateShaderMeta rebuilds the per-series metadata texture: color in column
// 0, then length (0 for a disabled series) and lane packed into column 1 so the
// shader reads all three with one fetch. Called when the legend toggles or
// recolors a series; a fresh image is allocated because the painter re-uploads
// only when the map entry points at a new image.
func (p *Plotter) updateShaderMeta() {
	if p.shader == nil {
		return
	}
	p.shader.Uniforms["lane_count"] = float32(p.updateLanes())
	meta := image.NewRGBA(image.Rect(0, 0, 2, len(p.ts)))
	for i, ts := range p.ts {
		meta.SetRGBA(0, i, ts.Color)
		n := 0
		if ts.Enabled {
			n = len(p.values[ts.Name])
		}
		meta.SetRGBA(1, i, color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: uint8(p.laneOf[i])})
	}
	p.shader.Textures["meta_tex"] = meta
}

// updateShaderView pushes the view window and hover state; this is the whole
// per-frame CPU cost of a playback seek on the shader backend.
func (p *Plotter) updateShaderView() {
	if p.shader == nil {
		return
	}
	size := p.plotObj.Size()
	u := p.shader.Uniforms
	u["plot_start"] = float32(p.plotStartPos)
	u["points_shown"] = float32(p.dataPointsToShow)
	u["highlight"] = float32(p.hilightLine)
	u["size_w"] = size.Width
	u["size_h"] = size.Height
}
