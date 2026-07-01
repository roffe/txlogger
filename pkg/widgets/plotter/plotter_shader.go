package plotter

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"sync/atomic"

	"fyne.io/fyne/v2/canvas"
)

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

const plotShaderBody = `
#define MAX_SERIES 64
#define MAX_SEG 4
#define MAX_RAW 16
#define MAX_GROUPS 24

uniform vec2 frame;
uniform vec4 bounds;

uniform sampler2D data_tex; // one texel per sample, 16-bit value in RG
uniform sampler2D mm_tex;   // min/max per 16 samples: RG=min, BA=max
uniform sampler2D meta_tex; // per series: x0 color, x1 enabled, x2 length

uniform float series_count;
uniform float highlight;    // hovered series index, -1 for none
uniform float plot_start;   // first visible sample
uniform float points_shown; // visible sample count
uniform float tex_w;        // texel columns in data_tex and mm_tex
uniform float rows_raw;     // data_tex rows per series
uniform float rows_mm;      // mm_tex rows per series
uniform float data_h;       // data_tex height in rows
uniform float mm_h;         // mm_tex height in rows
uniform float meta_h;       // meta_tex height = series count
uniform float size_w;       // widget size, logical px
uniform float size_h;

const float BIG = 100000.0;

float decode16(float hi, float lo) {
    return (hi * 65280.0 + lo * 255.0) / 65535.0;
}

vec4 meta_at(float x, float si) {
    return texture2D(meta_tex, vec2((x + 0.5) / 4.0, (si + 0.5) / meta_h));
}

// normalized sample value; idx is clamped to the series
float sample_val(float si, float idx, float len) {
    idx = clamp(idx, 0.0, len - 1.0);
    float row = floor(idx / tex_w);
    float colm = idx - row * tex_w;
    vec2 uv = vec2((colm + 0.5) / tex_w, (si * rows_raw + row + 0.5) / data_h);
    vec4 t = texture2D(data_tex, uv);
    return decode16(t.r, t.g);
}

// normalized (min, max) of sample group gidx
vec2 sample_mm(float si, float gidx, float glen) {
    gidx = clamp(gidx, 0.0, glen - 1.0);
    float row = floor(gidx / tex_w);
    float colm = gidx - row * tex_w;
    vec2 uv = vec2((colm + 0.5) / tex_w, (si * rows_mm + row + 0.5) / mm_h);
    vec4 t = texture2D(mm_tex, uv);
    return vec2(decode16(t.r, t.g), decode16(t.b, t.a));
}

// device y of a normalized value: texel range 0..1 spans one display range
// of headroom on each side of [Min, Max]
float val_y(float v, float h_dev) {
    float frac = v * 3.0 - 1.0;
    return (1.0 - frac) * (h_dev - 1.0);
}

float seg_dist(vec2 p, vec2 a, vec2 b) {
    vec2 e = b - a;
    float ee = dot(e, e);
    float h = ee > 0.000001 ? clamp(dot(p - a, e) / ee, 0.0, 1.0) : 0.0;
    return distance(p, a + e * h);
}

void main() {
    float pix_scale = (bounds.z - bounds.x) / max(size_w, 1.0);
    vec2 p_dev = vec2(gl_FragCoord.x, frame.y - gl_FragCoord.y) - bounds.xy;
    float w_dev = bounds.z - bounds.x;
    float h_dev = bounds.w - bounds.y;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > w_dev || p_dev.y > h_dev) {
        discard;
    }

    float ppd = points_shown / max(w_dev, 1.0);              // samples per device px
    float spos = plot_start + p_dev.x / w_dev * points_shown; // sample at this px
    float aa = 0.6;

    vec3 acc = vec3(0.0);
    float acc_a = 0.0;

    for (int i = 0; i < MAX_SERIES; i++) {
        if (i >= int(series_count + 0.5)) {
            break;
        }
        float si = float(i);
        if (meta_at(1.0, si).r < 0.5) {
            continue; // disabled via the legend
        }
        vec4 m2 = meta_at(2.0, si);
        float len = m2.r * 16711680.0 + m2.g * 65280.0 + m2.b * 255.0;
        if (len < 2.0) {
            continue;
        }
        // hovered series renders at 4 logical px like PlotImage thickness 4
        float half_w = (abs(si - highlight) < 0.5 ? 2.0 : 0.5) * pix_scale;

        float mask = 0.0;
        if (ppd <= 1.5) {
            // zoomed in: true polyline, distance to the segments around
            // this pixel's column
            float i0 = floor(spos);
            float dmin = BIG;
            for (int k = -MAX_SEG; k < MAX_SEG; k++) {
                float j = i0 + float(k);
                float x0 = (j - plot_start) / points_shown * w_dev;
                float x1 = (j + 1.0 - plot_start) / points_shown * w_dev;
                float y0 = val_y(sample_val(si, j, len), h_dev);
                float y1 = val_y(sample_val(si, j + 1.0, len), h_dev);
                dmin = min(dmin, seg_dist(p_dev, vec2(x0, y0), vec2(x1, y1)));
            }
            mask = 1.0 - smoothstep(half_w - aa, half_w + aa, dmin);
        } else {
            // zoomed out: vertical min/max run per column, like
            // plotImageDecimated (including the one-sample overlap into the
            // previous column that keeps runs connected)
            float s_a = spos - ppd * 0.5 - 1.0;
            float s_b = spos + ppd * 0.5;
            float lo = BIG;
            float hi = -BIG;
            if (ppd <= 14.0) {
                for (int k = 0; k < MAX_RAW; k++) {
                    float idx = s_a + float(k);
                    if (idx > s_b) {
                        break;
                    }
                    float v = sample_val(si, idx, len);
                    lo = min(lo, v);
                    hi = max(hi, v);
                }
            } else {
                float g0 = floor(s_a / 16.0);
                float glen = ceil(len / 16.0);
                for (int k = 0; k < MAX_GROUPS; k++) {
                    float g = g0 + float(k);
                    if (g * 16.0 > s_b) {
                        break;
                    }
                    vec2 mm = sample_mm(si, g, glen);
                    lo = min(lo, mm.x);
                    hi = max(hi, mm.y);
                }
            }
            float d = max(val_y(hi, h_dev) - p_dev.y, p_dev.y - val_y(lo, h_dev));
            mask = 1.0 - smoothstep(half_w - aa, half_w + aa, d);
        }

        // max blend, same as bresenhamCore, so overlap is order independent
        vec4 col = meta_at(0.0, si);
        acc = max(acc, col.rgb * mask * col.a);
        acc_a = max(acc_a, mask * col.a);
    }

    if (acc_a < 0.004) {
        discard;
    }
    gl_FragColor = vec4(acc / acc_a, acc_a);
}
`

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

// updateShaderMeta rebuilds the per-series metadata texture (color, enabled,
// length). Called when the legend toggles or recolors a series; a fresh image
// is allocated because the painter re-uploads only when the map entry points
// at a new image.
func (p *Plotter) updateShaderMeta() {
	if p.shader == nil {
		return
	}
	meta := image.NewRGBA(image.Rect(0, 0, 4, len(p.ts)))
	for i, ts := range p.ts {
		meta.SetRGBA(0, i, ts.Color)
		var enabled uint8
		if ts.Enabled {
			enabled = 0xff
		}
		meta.SetRGBA(1, i, color.RGBA{R: enabled, A: 0xff})
		n := len(p.values[ts.Name])
		meta.SetRGBA(2, i, color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 0xff})
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
