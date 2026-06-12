package plotter

import (
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func shaderPlotter(t testing.TB, numSeries, numPoints int) *Plotter {
	t.Helper()
	p := NewPlotter(benchValues(numSeries, numPoints))
	if p.backend != plotBackendShader {
		t.Fatal("shader backend not selected")
	}
	return p
}

// decode16 mirrors the shader's texel decode.
func decode16(hi, lo uint8) float64 {
	return (float64(hi)*256 + float64(lo)) / 65535
}

// Every sample must decode from the data texture to its display-normalized
// value: texel 0..1 spans [Min-range, Max+range].
func TestPlotShaderDataEncoding(t *testing.T) {
	p := shaderPlotter(t, 3, 5000)

	tex := p.shader.Textures["data_tex"].(*image.RGBA)
	texW := int(p.shader.Uniforms["tex_w"])
	rowsRaw := int(p.shader.Uniforms["rows_raw"])
	if texW != 4096 || rowsRaw != 2 {
		t.Fatalf("layout texW=%d rowsRaw=%d, want 4096/2", texW, rowsRaw)
	}

	for i, ts := range p.ts {
		data := p.values[ts.Name]
		for s, v := range data {
			c := tex.RGBAAt(s%texW, i*rowsRaw+s/texW)
			got := decode16(c.R, c.G)
			want := ((v-ts.Min)/ts.valueRange + 1) / 3
			if math.Abs(got-want) > 1.0/65535 {
				t.Fatalf("series %d sample %d: %v, want %v", i, s, got, want)
			}
		}
	}
}

// The min/max texture must hold the exact extremes of each 16-sample group.
func TestPlotShaderMinMax(t *testing.T) {
	p := shaderPlotter(t, 2, 5000)

	mm := p.shader.Textures["mm_tex"].(*image.RGBA)
	texW := int(p.shader.Uniforms["tex_w"])
	rowsMM := int(p.shader.Uniforms["rows_mm"])

	for i, ts := range p.ts {
		data := p.values[ts.Name]
		for g := 0; g*mmGroup < len(data); g++ {
			end := min((g+1)*mmGroup, len(data))
			lo, hi := data[g*mmGroup], data[g*mmGroup]
			for _, v := range data[g*mmGroup+1 : end] {
				lo = min(lo, v)
				hi = max(hi, v)
			}
			c := mm.RGBAAt(g%texW, i*rowsMM+g/texW)
			wantLo := ((lo-ts.Min)/ts.valueRange + 1) / 3
			wantHi := ((hi-ts.Min)/ts.valueRange + 1) / 3
			if math.Abs(decode16(c.R, c.G)-wantLo) > 1.0/65535 || math.Abs(decode16(c.B, c.A)-wantHi) > 1.0/65535 {
				t.Fatalf("series %d group %d: (%v,%v), want (%v,%v)",
					i, g, decode16(c.R, c.G), decode16(c.B, c.A), wantLo, wantHi)
			}
		}
	}
}

// Out-of-range samples must clamp a full display range off screen, not at
// the plot edge, so overshooting lines keep their slope like the Bresenham
// clipping does.
func TestPlotShaderEncodeHeadroom(t *testing.T) {
	ts := &TimeSeries{Min: 0, Max: 10, valueRange: 10}
	for _, tc := range []struct {
		v    float64
		want float64
	}{
		{0, 1.0 / 3},       // Min -> bottom of display band
		{10, 2.0 / 3},      // Max -> top of display band
		{-10, 0},           // one range below -> texel floor
		{20, 1},            // one range above -> texel ceil
		{-100, 0}, {99, 1}, // far overshoot clamps
	} {
		got := float64(encodeValue(ts, tc.v)) / 65535
		if math.Abs(got-tc.want) > 1.0/65535 {
			t.Fatalf("encode(%v) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

// The metadata texture carries color, enabled flag and series length, and a
// legend toggle must produce a fresh image holding the new state.
func TestPlotShaderMeta(t *testing.T) {
	p := shaderPlotter(t, 2, 100)

	meta := p.shader.Textures["meta_tex"].(*image.RGBA)
	for i, ts := range p.ts {
		if c := meta.RGBAAt(0, i); c != ts.Color {
			t.Fatalf("series %d color %v, want %v", i, c, ts.Color)
		}
		if c := meta.RGBAAt(1, i); c.R != 0xff {
			t.Fatalf("series %d not flagged enabled", i)
		}
		n := len(p.values[ts.Name])
		c := meta.RGBAAt(2, i)
		if got := int(c.R)<<16 | int(c.G)<<8 | int(c.B); got != n {
			t.Fatalf("series %d length %d, want %d", i, got, n)
		}
	}

	p.ts[1].Enabled = false
	p.updateShaderMeta()
	meta2 := p.shader.Textures["meta_tex"].(*image.RGBA)
	if meta2 == meta {
		t.Fatal("meta texture not replaced; painter would not re-upload")
	}
	if c := meta2.RGBAAt(1, 1); c.R != 0 {
		t.Fatal("disabled series still flagged enabled")
	}
}

// Logs that exceed the texture budget must fall back to the image backend.
func TestPlotShaderFallback(t *testing.T) {
	p := NewPlotter(benchValues(2, plotTexW*plotTexMaxH/2+1))
	if p.backend != plotBackendImage {
		t.Fatal("oversized log did not fall back to the image backend")
	}
	if p.plotObj != p.canvasImage {
		t.Fatal("fallback must keep drawing through canvasImage")
	}
}

// Both shader variants must at least compile; glslangValidator checks them
// against the GLSL 1.10 (desktop) and GLSL ES 1.00 specs.
func TestPlotShaderSourcesCompile(t *testing.T) {
	validator, err := exec.LookPath("glslangValidator")
	if err != nil {
		t.Skip("glslangValidator not installed")
	}
	for name, src := range map[string]string{
		"desktop.frag": plotShaderPreludeGL + plotShaderBody,
		"es.frag":      plotShaderPreludeES + plotShaderBody,
	} {
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(validator, p).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
	}
}

// CPU-side cost of a playback seek on the shader backend; compare against
// the BenchmarkPlot_* figures for the image backend (which additionally
// re-uploads the whole plot texture every frame).
func BenchmarkUpdateShaderView(b *testing.B) {
	p := shaderPlotter(b, 30, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.plotStartPos = i % 1000
		p.updateShaderView()
	}
}
