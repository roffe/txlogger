package tunnel

import (
	"image"
	"image/color"
	"log"

	"fyne.io/fyne/v2/theme"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Credits-strip texture layout. Each line gets a fixed-height band; the shader
// samples the strip with a perspective crawl and loops it with fract.
const (
	crawlTexWidth = 640
	crawlLineH    = 56
	crawlFontSize = 40
)

// SetCredits sets the lines shown as a Star Wars-style crawl that rises from
// the bottom into the tunnel centre and loops forever. An empty string renders
// as a blank line, useful for spacing and section breaks - including a blank
// first/last line, which hides the loop seam. Passing an empty slice removes
// the crawl.
func (t *Tunnel) SetCredits(lines []string) {
	if t.shader == nil {
		return
	}
	img := renderCrawl(lines)
	if img == nil {
		delete(t.shader.Textures, "crawl_tex")
		t.shader.Uniforms["crawl_lines"] = 0
		t.shader.Refresh()
		return
	}
	if t.shader.Textures == nil {
		t.shader.Textures = make(map[string]image.Image, 2)
	}
	t.shader.Textures["crawl_tex"] = img
	t.shader.Uniforms["crawl_lines"] = float32(len(lines))
	t.shader.Refresh()
}

// renderCrawl rasterises the credit lines into a tall, centred, transparent
// strip: band i holds line i, with band 0 (image row 0, texture t = 0) the
// first line. The painter uploads image rows verbatim, so the first line sits
// at the far end of the crawl. Returns nil for an empty list.
func renderCrawl(lines []string) *image.RGBA {
	if len(lines) == 0 {
		return nil
	}
	face, err := crawlFace()
	if err != nil {
		log.Printf("tunnel: crawl font: %v", err)
		return nil
	}
	defer face.Close()

	img := image.NewRGBA(image.Rect(0, 0, crawlTexWidth, len(lines)*crawlLineH))

	m := face.Metrics()
	textH := (m.Ascent + m.Descent).Ceil()
	topPad := (crawlLineH - textH) / 2

	d := &font.Drawer{Dst: img, Src: image.NewUniform(color.White), Face: face}
	for i, line := range lines {
		if line == "" {
			continue // blank spacer line
		}
		adv := font.MeasureString(face, line)
		d.Dot = fixed.Point26_6{
			X: (fixed.I(crawlTexWidth) - adv) / 2, // centre horizontally
			Y: fixed.I(i*crawlLineH+topPad) + m.Ascent,
		}
		d.DrawString(line)
	}
	return img
}

// crawlFace builds a font face from the app's bundled text font so the crawl
// matches the rest of the UI.
func crawlFace() (font.Face, error) {
	ft, err := opentype.Parse(theme.DefaultTextFont().Content())
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(ft, &opentype.FaceOptions{
		Size:    crawlFontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}
