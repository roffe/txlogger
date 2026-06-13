package tunnel

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
)

// New must produce a widget whose shader carries the animation-driven "time"
// uniform and tuning knobs, plus a renderer that exposes the shader.
func TestNewTunnel(t *testing.T) {
	test.NewTempApp(t) // CreateRenderer starts the animation, which needs a driver
	tn := New()
	if tn.shader == nil {
		t.Fatal("shader not initialised")
	}
	if _, ok := tn.shader.Uniforms["time"]; !ok {
		t.Fatal("time uniform missing")
	}
	if tn.anim == nil {
		t.Fatal("animation not created")
	}
	if _, ok := tn.shader.Textures["logo_tex"]; !ok {
		t.Fatal("embedded logo texture not loaded")
	}

	r := tn.CreateRenderer()
	objs := r.Objects()
	if len(objs) != 1 || objs[0] != tn.shader {
		t.Fatalf("renderer objects = %v, want [shader]", objs)
	}
	if min := r.MinSize(); min.Width != defaultSize || min.Height != defaultSize {
		t.Fatalf("MinSize = %v, want %dx%d", min, defaultSize, defaultSize)
	}
	r.Destroy()
}

// SetCredits must rasterise the lines into a crawl texture sized one band per
// line (blank lines included) and record the line count for the shader.
func TestSetCredits(t *testing.T) {
	test.NewTempApp(t)
	tn := New()

	lines := []string{"", "txlogger thanks", "", "Alice", "Bob", ""}
	tn.SetCredits(lines)

	img, ok := tn.shader.Textures["crawl_tex"].(*image.RGBA)
	if !ok {
		t.Fatal("crawl texture not created")
	}
	if w := img.Bounds().Dy(); w != len(lines)*crawlLineH {
		t.Fatalf("crawl height = %d, want %d", w, len(lines)*crawlLineH)
	}
	if got := tn.shader.Uniforms["crawl_lines"]; got != float32(len(lines)) {
		t.Fatalf("crawl_lines = %v, want %d", got, len(lines))
	}

	// An empty slice removes the crawl again.
	tn.SetCredits(nil)
	if _, ok := tn.shader.Textures["crawl_tex"]; ok {
		t.Fatal("crawl texture should be removed for empty credits")
	}
	if got := tn.shader.Uniforms["crawl_lines"]; got != 0 {
		t.Fatalf("crawl_lines = %v, want 0", got)
	}
}

// Both shader variants must compile; glslangValidator checks them against the
// GLSL 1.10 (desktop) and GLSL ES 1.00 specs.
func TestTunnelShaderSourcesCompile(t *testing.T) {
	validator, err := exec.LookPath("glslangValidator")
	if err != nil {
		t.Skip("glslangValidator not installed")
	}
	for name, src := range map[string]string{
		"desktop.frag": tunnelPreludeGL + tunnelBody,
		"es.frag":      tunnelPreludeES + tunnelBody,
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
