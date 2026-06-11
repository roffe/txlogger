package meshgrid

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
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
	m, err := NewMeshgrid("RPM", "Load", "Fuel", values, cols, rows, colors.ModeNormal)
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
		m, err := NewMeshgrid("RPM", "Load", "Fuel", values, cols, rows, colors.ModeNormal)
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

func BenchmarkDrawSurface(b *testing.B) {
	m := testGrid(b)
	m.renderMode = RenderModeSolidWireframe
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.drawMeshgridLines()
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
