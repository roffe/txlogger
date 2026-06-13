package meshgrid

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/colors"
)

func testGrid(t testing.TB) *Meshgrid {
	t.Helper()
	cols, rows := 16, 16
	values := make([]float64, cols*rows)
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			x := float64(j-cols/2) / 3
			y := float64(i-rows/2) / 3
			values[i*cols+j] = 100 / (1 + x*x + y*y) // central hump
		}
	}
	m, err := NewMeshgrid("RPM", "Load", "Fuel", values, cols, rows, colors.ModeNormal, backendFromEnv())
	if err != nil {
		t.Fatal(err)
	}
	m.size = fyne.NewSize(800, 500)
	return m
}

// TestRenderRotated renders an asymmetric surface (tall corner spike) from
// four yaw angles so painter's-order mistakes show up as the spike being
// overdrawn by cells that are behind it.
func TestRenderRotated(t *testing.T) {
	if os.Getenv("MESHGRID_DUMP") == "" {
		t.Skip("set MESHGRID_DUMP=1 to dump rotation PNGs")
	}
	cols, rows := 16, 16
	for n, yaw := range []float64{0, 90, 180, 270} {
		values := make([]float64, cols*rows)
		for i := 0; i < rows; i++ {
			for j := 0; j < cols; j++ {
				x := float64(j-2) / 1.5
				y := float64(i-2) / 1.5
				values[i*cols+j] = 10 + 100/(1+x*x+y*y) // spike near one corner
			}
		}
		m, err := NewMeshgrid("RPM", "Load", "Fuel", values, cols, rows, colors.ModeNormal, backendFromEnv())
		if err != nil {
			t.Fatal(err)
		}
		m.size = fyne.NewSize(800, 500)
		m.rotateMeshgrid(0, yaw, 0)
		img := m.drawMeshgridLines()
		f, err := os.Create("/tmp/meshgrid_rot_" + string(rune('0'+n)) + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

// the tracking marker overlay must land on the surface: a cursor centered on
// a vertex (cell 7.5 + the 0.5 corner offset = vertex 8) must project to
// exactly that vertex's screen position
func TestCursorScreenPosition(t *testing.T) {
	m := testGrid(t)
	m.cursorX, m.cursorY, m.showCursor = 7.5, 7.5, true
	px, py := m.cursorScreenPosition()
	v := m.vertices[8][8]
	wantX := float32(float64(m.size.Width)*0.5 + v.X)
	wantY := float32(float64(m.size.Height)*0.5 + v.Y)
	if px != wantX || py != wantY {
		t.Fatalf("cursor at (%v, %v), want vertex projection (%v, %v)", px, py, wantX, wantY)
	}
	if px < 0 || px > m.size.Width || py < 0 || py > m.size.Height {
		t.Fatalf("cursor (%v, %v) outside widget %v", px, py, m.size)
	}
}

func BenchmarkDrawSurface(b *testing.B) {
	m := testGrid(b)
	m.renderMode = RenderModeSolidWireframe
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.drawMeshgridLines()
	}
}

// usePolyBackend switches a test grid to the polygon renderer (the default
// is the shader backend, whose per-frame work happens on the GPU).
func usePolyBackend(m *Meshgrid) {
	m.backend = BackendPolygons
	m.initPolygons()
}

// CPU-side cost of a polygon-renderer frame (projection, colors, painter's
// sort, object updates). The GPU draw calls aren't measurable here; compare
// against BenchmarkDrawSurface, which also excludes that path's per-frame
// texture upload.
func BenchmarkUpdatePolygons(b *testing.B) {
	m := testGrid(b)
	usePolyBackend(m)
	m.renderMode = RenderModeSolidWireframe
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.updatePolygons()
	}
}

// TestPolygonDegenerateQuads sweeps the camera across many angles over a
// surface with flat plateaus (rows of identical values, like real boost maps)
// and asserts every visible polygon is well-formed: at least three corners,
// no near-zero edges (those become NaN in the GL shader's normalize() and
// flood the bounding box with color) and no self-intersecting outlines.
func TestPolygonDegenerateQuads(t *testing.T) {
	cols, rows := 16, 16
	values := make([]float64, cols*rows)
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			// Stepped plateaus: blocks of identical values produce coplanar
			// cells that collapse to slivers when viewed edge-on.
			values[i*cols+j] = float64((i / 4) * 100)
		}
	}
	m, err := NewMeshgrid("RPM", "Load", "Fuel", values, cols, rows, colors.ModeNormal, backendFromEnv())
	if err != nil {
		t.Fatal(err)
	}
	m.size = fyne.NewSize(800, 500)
	usePolyBackend(m)
	m.renderMode = RenderModeSolidWireframe

	for pitch := 0; pitch < 180; pitch += 15 {
		for yaw := 0; yaw < 360; yaw += 15 {
			m.cameraRotation = RotationMatrixX(float64(pitch)).Multiply(NewMatrix3x3()).Multiply(RotationMatrixZ(float64(yaw)))
			m.updateVertexPositions()
			m.updatePolygons()

			for idx, p := range m.polys {
				if p.Hidden {
					continue
				}
				pts := p.Points
				if len(pts) < 3 {
					t.Fatalf("pitch=%d yaw=%d cell %d: visible polygon with %d points", pitch, yaw, idx, len(pts))
				}
				for k := range pts {
					a, b := pts[k], pts[(k+1)%len(pts)]
					dx, dy := b.X-a.X, b.Y-a.Y
					if dx*dx+dy*dy < minEdgeSq {
						t.Fatalf("pitch=%d yaw=%d cell %d: collapsed edge %d: %v", pitch, yaw, idx, k, pts)
					}
				}
				if len(pts) == 4 {
					if segmentsCross(pts[0].X, pts[0].Y, pts[1].X, pts[1].Y, pts[2].X, pts[2].Y, pts[3].X, pts[3].Y) ||
						segmentsCross(pts[1].X, pts[1].Y, pts[2].X, pts[2].Y, pts[3].X, pts[3].Y, pts[0].X, pts[0].Y) {
						t.Fatalf("pitch=%d yaw=%d cell %d: self-intersecting polygon: %v", pitch, yaw, idx, pts)
					}
				}
			}
		}
	}
}

func TestAppendDistinct(t *testing.T) {
	p := func(x, y float32) fyne.Position { return fyne.NewPos(x, y) }
	for _, tc := range []struct {
		name    string
		corners [4]fyne.Position
		want    int
	}{
		{"all distinct", [4]fyne.Position{p(0, 0), p(10, 0), p(10, 10), p(0, 10)}, 4},
		{"one collapsed edge", [4]fyne.Position{p(0, 0), p(10, 0), p(10.1, 0.1), p(0, 10)}, 3},
		{"wrap-around collapse", [4]fyne.Position{p(0, 0), p(10, 0), p(10, 10), p(0.1, 0.1)}, 3},
		{"line", [4]fyne.Position{p(0, 0), p(10, 0), p(10.1, 0), p(0.2, 0.1)}, 2},
		{"point", [4]fyne.Position{p(5, 5), p(5.1, 5), p(5, 5.1), p(5.1, 5.1)}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := appendDistinct(make([]fyne.Position, 0, 4), tc.corners)
			if len(got) != tc.want {
				t.Fatalf("got %d points %v, want %d", len(got), got, tc.want)
			}
			for k := 0; len(got) >= 2 && k < len(got); k++ {
				a, b := got[k], got[(k+1)%len(got)]
				dx, dy := b.X-a.X, b.Y-a.Y
				if dx*dx+dy*dy < minEdgeSq {
					t.Fatalf("result has collapsed edge %d: %v", k, got)
				}
			}
		})
	}
}

// TestPolygonRender captures the polygon renderer through the software
// painter so the output can be eyeballed without a GL window.
func TestPolygonRender(t *testing.T) {
	if os.Getenv("MESHGRID_DUMP") == "" {
		t.Skip("set MESHGRID_DUMP=1 to dump polygon render PNGs")
	}
	test.NewApp()
	for _, tc := range []struct {
		name string
		mode RenderMode
	}{
		{"solidwire", RenderModeSolidWireframe},
		{"solid", RenderModeSolid},
		{"wireframe", RenderModeWireframe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testGrid(t)
			usePolyBackend(m)
			m.renderMode = tc.mode
			w := test.NewWindow(m)
			defer w.Close()
			w.Resize(fyne.NewSize(820, 520))
			m.refresh()
			img := w.Canvas().Capture()
			f, err := os.Create("/tmp/meshgrid_poly_" + tc.name + ".png")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRenderModes(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode RenderMode
	}{
		{"solidwire", RenderModeSolidWireframe},
		{"solid", RenderModeSolid},
		{"wireframe", RenderModeWireframe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testGrid(t)
			m.renderMode = tc.mode
			img := m.drawMeshgridLines()
			if img.Bounds().Dx() != 800 || img.Bounds().Dy() != 500 {
				t.Fatalf("unexpected bounds %v", img.Bounds())
			}
			if os.Getenv("MESHGRID_DUMP") != "" {
				f, err := os.Create("/tmp/meshgrid_" + tc.name + ".png")
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				if err := png.Encode(f, img); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
