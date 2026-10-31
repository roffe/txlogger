package tunnel

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"log"
	"sync/atomic"

	"fyne.io/fyne/v2/canvas"
)

//go:embed logo.png
var logoPNG []byte

// tunnelWGSL is the same shader ported to WGSL for Fyne's WebGPU painter; see
// its file header for the porting notes. The OpenGL painter ignores it.
//
//go:embed tunnel.wgsl
var tunnelWGSL []byte

// The whole effect is a single procedural fragment shader: no geometry, no
// textures, no per-frame CPU work beyond the "time" uniform that
// canvas.NewShaderAnimation advances. Each pixel reconstructs an infinite
// tube in polar coordinates - angle around the tube, 1/radius into it - and
// draws a glowing retrowave wireframe of rings and ribs that streams toward
// the viewer. A swaying vanishing point plus a depth-dependent twist give the
// rollercoaster ride; hue cycling over depth and time keeps it psychedelic.

// tunnelSeq makes each widget's Shader.Name unique so the painter caches one
// compiled program per live widget instead of evicting a shared one.
var tunnelSeq atomic.Int64

const tunnelPreludeGL = "#version 110\n"

const tunnelPreludeES = `#version 100
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif
`

//go:embed tunnel.frag
var tunnelBody string

func (t *Tunnel) initShader() {
	t.shader = canvas.NewShader(
		fmt.Sprintf("tunnel-%d", tunnelSeq.Add(1)),
		[]byte(tunnelPreludeGL+tunnelBody),
		[]byte(tunnelPreludeES+tunnelBody),
	)
	t.shader.SourceWGSL = tunnelWGSL
	t.shader.Uniforms = map[string]float32{
		"time":         0,
		"speed":        1.1,
		"ring_freq":    5.0,
		"rib_freq":     16.0,
		"logo_scale":   0.30,
		"crawl_lines":  0, // no credits until SetCredits is called
		"crawl_persp":  0.45,
		"crawl_width":  0.9,
		"crawl_zlines": 3.0,
		"crawl_speed":  0.8,
	}
	t.shader.Textures = map[string]image.Image{}
	if logo := loadLogo(); logo != nil {
		t.shader.Textures["logo_tex"] = logo
	}
}

// loadLogo decodes the embedded txlogger logo into a premultiplied *image.RGBA,
// the form the GL painter uploads verbatim (image row 0 -> texture t=0, so the
// logo samples upright). Returns nil if decoding fails, in which case the
// shader simply samples an unbound texture and draws no logo.
func loadLogo() image.Image {
	img, _, err := image.Decode(bytes.NewReader(logoPNG))
	if err != nil {
		log.Printf("tunnel: decoding logo.png: %v", err)
		return nil
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	return rgba
}
