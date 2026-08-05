package meshgrid

import (
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/roffe/txlogger/pkg/colors"
)

// In dataVertexMode the data texture plus z_off/z_gain must reproduce every
// cell's flat-top height (V - zmin)/zrange * depth in grid units. Data cell
// (r, c) is stored at texel (c, rows-1-r) so grid Y runs with the data rows.
func TestShaderDataEncoding(t *testing.T) {
	m := testGrid(t)
	if !m.dataVertexMode() {
		t.Fatalf("testGrid is %dx%d, expected dataVertexMode", m.cols, m.rows)
	}
	tex, ok := m.shader.Textures["mesh_tex"].(*image.RGBA)
	if !ok {
		t.Fatal("mesh_tex missing or not RGBA")
	}
	if tex.Bounds().Dx() != m.cols || tex.Bounds().Dy() != m.rows {
		t.Fatalf("mesh_tex is %v, want %dx%d", tex.Bounds(), m.cols, m.rows)
	}

	zOff := float64(m.shader.Uniforms["z_off"])
	zGain := float64(m.shader.Uniforms["z_gain"])
	hmax := m.depth / float64(m.cellWidth)

	for r := 0; r < m.rows; r++ {
		for c := 0; c < m.cols; c++ {
			px := tex.RGBAAt(c, m.rows-1-r)
			norm := (float64(px.R)*256 + float64(px.G)) / 65535
			gotH := zOff + zGain*norm
			wantH := (m.values[r*m.cols+c] - m.zmin) / m.zrange * hmax
			// 16-bit quantization of the encoded value range
			if math.Abs(gotH-wantH) > zGain/65535+1e-6 {
				t.Fatalf("cell (%d,%d): height %v, want %v", r, c, gotH, wantH)
			}
		}
	}
}

func TestShaderColormap(t *testing.T) {
	m := testGrid(t)
	lut, ok := m.shader.Textures["colormap_tex"].(*image.RGBA)
	if !ok {
		t.Fatal("colormap_tex missing or not RGBA")
	}
	if lut.Bounds().Dx() != 256 || lut.Bounds().Dy() != 1 {
		t.Fatalf("colormap_tex is %v, want 256x1", lut.Bounds())
	}
	for k := 0; k < 256; k++ {
		want := colors.GetColorInterpolation(0, 255, float64(k), m.colorMode)
		if got := lut.RGBAAt(k, 0); got != want {
			t.Fatalf("lut[%d] = %v, want %v", k, got, want)
		}
	}
}

// The shader projects grid points with
//
//	view = R*((g - center)*scale_px) - cam; screen = view.xy + size/2
//
// which must land on the same screen position as the CPU-transformed
// vertices, for any camera. Otherwise the mesh and the cursor/axis overlays
// drift apart.
func TestShaderProjectionMatchesVertices(t *testing.T) {
	m := testGrid(t)
	m.orbit(23, -41)
	m.panMeshgrid(15, -7)
	m.scaleMeshgrid(1.3)
	m.updateShaderUniforms()

	u := m.shader.Uniforms
	r := [3][3]float64{
		{float64(u["r0"]), float64(u["r1"]), float64(u["r2"])},
		{float64(u["r3"]), float64(u["r4"]), float64(u["r5"])},
		{float64(u["r6"]), float64(u["r7"]), float64(u["r8"])},
	}
	cg := [3]float64{float64(u["center_gx"]), float64(u["center_gy"]), float64(u["center_gz"])}
	scalePx := float64(u["scale_px"])
	cw := float64(m.cellWidth)

	for i := 0; i <= m.rows; i += 4 {
		for j := 0; j <= m.cols; j += 4 {
			v := m.vertices[i][j]
			// dataVertexMode shifts the grid half a cell: the averaged corner
			// (i, j) maps to shader grid (j-0.5, rows-i-0.5), which projects to
			// the same screen point as the CPU corner transform.
			g := [3]float64{float64(j) - 0.5, float64(m.rows-i) - 0.5, v.Oz / cw}

			var view [3]float64
			for a := 0; a < 3; a++ {
				view[a] = r[a][0]*(g[0]-cg[0])*scalePx +
					r[a][1]*(g[1]-cg[1])*scalePx +
					r[a][2]*(g[2]-cg[2])*scalePx
			}
			view[0] -= float64(u["cam_x"])
			view[1] -= float64(u["cam_y"])

			if math.Abs(view[0]-v.X) > 1e-3 || math.Abs(view[1]-v.Y) > 1e-3 || math.Abs(view[2]-v.Z) > 1e-3 {
				t.Fatalf("vertex (%d,%d): shader view (%v,%v,%v), CPU view (%v,%v,%v)",
					i, j, view[0], view[1], view[2], v.X, v.Y, v.Z)
			}
		}
	}
}

// CPU-side cost of a shader-backend frame: this plus the axis/cursor overlay
// moves is everything the CPU does per rotation/zoom frame. Compare against
// BenchmarkDrawSurface (image backend) and BenchmarkUpdatePolygons.
func BenchmarkUpdateShaderUniforms(b *testing.B) {
	m := testGrid(b)
	m.renderMode = RenderModeSolidWireframe
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.updateShaderUniforms()
	}
}

// Both shader variants must at least compile; glslangValidator checks them
// against the GLSL 1.10 (desktop) and GLSL ES 1.00 specs.
func TestShaderSourcesCompile(t *testing.T) {
	validator, err := exec.LookPath("glslangValidator")
	if err != nil {
		t.Skip("glslangValidator not installed")
	}
	for name, src := range map[string]string{
		"desktop.frag": meshShaderPreludeGL + meshShaderBody,
		"es.frag":      meshShaderPreludeES + meshShaderBody,
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
