//go:build gpubench

// GPU cost of the plot shader, which CPU profiles cannot see. It renders
// plotShaderBody into a 1920x1080 offscreen framebuffer with a real GL context
// and times each frame with GL_TIME_ELAPSED queries:
//
//	go test -tags gpubench -run TestPlotShaderGPU -v ./pkg/widgets/plotter/
//
// PLOTBENCH_SHOT=dir writes each scenario's frame as a PNG, and
// PLOTBENCH_REF=dir compares every frame against the PNGs of an earlier run,
// so a shader change can be checked for both speed and output.
package plotter

import (
	"fmt"
	"image"
	"image/png"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.4/glfw"
)

const gpuBenchW, gpuBenchH = 1920, 1080

func TestPlotShaderGPU(t *testing.T) {
	runtime.LockOSThread() // the GL context is current on this thread only
	defer runtime.UnlockOSThread()
	if err := glfw.Init(); err != nil {
		t.Skip("no GL:", err)
	}
	defer glfw.Terminate()
	glfw.WindowHint(glfw.Visible, glfw.False)
	win, err := glfw.CreateWindow(64, 64, "plotbench", nil, nil)
	if err != nil {
		t.Skip("no GL window:", err)
	}
	defer win.Destroy()
	win.MakeContextCurrent()
	glfw.SwapInterval(0)
	if err := gl.Init(); err != nil {
		t.Fatal(err)
	}
	t.Log(gl.GoStr(gl.GetString(gl.RENDERER)))

	var fbo, target uint32
	gl.GenTextures(1, &target)
	gl.BindTexture(gl.TEXTURE_2D, target)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, gpuBenchW, gpuBenchH, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.GenFramebuffers(1, &fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, target, 0)
	if s := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); s != gl.FRAMEBUFFER_COMPLETE {
		t.Fatalf("framebuffer incomplete: %x", s)
	}
	gl.Viewport(0, 0, gpuBenchW, gpuBenchH)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	prog := gpuProgram(t, "#version 110\nattribute vec2 vert;\nvoid main() { gl_Position = vec4(vert, 0.0, 1.0); }\n",
		plotShaderPreludeGL+plotShaderBody)
	gl.UseProgram(prog)
	quad := []float32{-1, -1, 1, -1, -1, 1, 1, 1}
	var vbo uint32
	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(quad)*4, gl.Ptr(quad), gl.STATIC_DRAW)
	vert := uint32(gl.GetAttribLocation(prog, gl.Str("vert\x00")))
	gl.EnableVertexAttribArray(vert)
	gl.VertexAttribPointer(vert, 2, gl.FLOAT, false, 0, nil)

	var query uint32
	gl.GenQueries(1, &query)

	shots, ref := os.Getenv("PLOTBENCH_SHOT"), os.Getenv("PLOTBENCH_REF")
	for _, sc := range []struct {
		series, shown, highlight int
		lanes                    bool
	}{
		{10, 250, -1, false}, // default zoom
		{30, 250, -1, false},
		{30, 250, 3, false}, // hovered legend entry
		{30, 250, -1, true}, // every third series disabled via the legend
		{30, 1000, -1, false},
		{30, 5000, -1, false},  // raw min/max columns
		{30, 10000, -1, false}, // zoom slider max
	} {
		name := fmt.Sprintf("%dseries_%dpts", sc.series, sc.shown)
		if sc.highlight >= 0 {
			name += "_hover"
		}
		if sc.lanes {
			name += "_lanes"
		}
		p := shaderPlotter(t, sc.series, 30000)
		p.plotStartPos, p.dataPointsToShow, p.hilightLine, p.laneMode = 5000, sc.shown, sc.highlight, sc.lanes
		if sc.lanes {
			for i := 0; i < len(p.ts); i += 3 {
				p.ts[i].Enabled = false
			}
		}
		p.updateShaderMeta()
		p.updateShaderView()
		u := p.shader.Uniforms
		u["size_w"], u["size_h"] = gpuBenchW, gpuBenchH

		gl.Uniform2f(gl.GetUniformLocation(prog, gl.Str("frame\x00")), gpuBenchW, gpuBenchH)
		gl.Uniform4f(gl.GetUniformLocation(prog, gl.Str("bounds\x00")), 0, 0, gpuBenchW, gpuBenchH)
		for k, v := range u {
			gl.Uniform1f(gl.GetUniformLocation(prog, gl.Str(k+"\x00")), v)
		}
		texNames := slices.Sorted(maps.Keys(p.shader.Textures)) // the painter binds by sorted name
		texs := make([]uint32, len(texNames))
		gl.GenTextures(int32(len(texs)), &texs[0])
		for i, k := range texNames {
			img := p.shader.Textures[k].(*image.RGBA)
			gl.ActiveTexture(gl.TEXTURE0 + uint32(i))
			gl.BindTexture(gl.TEXTURE_2D, texs[i])
			// the painter uploads user textures with ImageScaleSmooth
			gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
			gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
			gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
			gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
			gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(img.Rect.Dx()), int32(img.Rect.Dy()), 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))
			gl.Uniform1i(gl.GetUniformLocation(prog, gl.Str(k+"\x00")), int32(i))
		}

		if e := gl.GetError(); e != 0 {
			t.Fatalf("gl error %x", e)
		}
		const warm, frames = 20, 200
		var times []time.Duration
		for i := range warm + frames {
			gl.ClearColor(0, 0, 0, 0)
			gl.Clear(gl.COLOR_BUFFER_BIT)
			gl.BeginQuery(gl.TIME_ELAPSED, query)
			gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
			gl.EndQuery(gl.TIME_ELAPSED)
			var ns uint64
			gl.GetQueryObjectui64v(query, gl.QUERY_RESULT, &ns)
			if i >= warm {
				times = append(times, time.Duration(ns))
			}
		}
		slices.Sort(times)
		t.Logf("%-26s median %7.3f ms  p90 %7.3f ms", name, ms(times[len(times)/2]), ms(times[len(times)*9/10]))

		if shots != "" || ref != "" {
			img := image.NewNRGBA(image.Rect(0, 0, gpuBenchW, gpuBenchH))
			gl.ReadPixels(0, 0, gpuBenchW, gpuBenchH, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))
			if shots != "" {
				writePNG(t, filepath.Join(shots, name+".png"), img)
			}
			if ref != "" {
				comparePNG(t, filepath.Join(ref, name+".png"), img)
			}
		}
		gl.DeleteTextures(int32(len(texs)), &texs[0])
	}
}

func ms(d time.Duration) float64 { return float64(d) / 1e6 }

func gpuProgram(t *testing.T, vs, fs string) uint32 {
	compile := func(kind uint32, src string) uint32 {
		s := gl.CreateShader(kind)
		csrc, free := gl.Strs(src + "\x00")
		defer free()
		gl.ShaderSource(s, 1, csrc, nil)
		gl.CompileShader(s)
		var ok int32
		gl.GetShaderiv(s, gl.COMPILE_STATUS, &ok)
		if ok == gl.FALSE {
			var n int32
			gl.GetShaderiv(s, gl.INFO_LOG_LENGTH, &n)
			msg := strings.Repeat("\x00", int(n+1))
			gl.GetShaderInfoLog(s, n, nil, gl.Str(msg))
			t.Fatal("compile:", msg)
		}
		return s
	}
	p := gl.CreateProgram()
	gl.AttachShader(p, compile(gl.VERTEX_SHADER, vs))
	gl.AttachShader(p, compile(gl.FRAGMENT_SHADER, fs))
	gl.LinkProgram(p)
	var ok int32
	gl.GetProgramiv(p, gl.LINK_STATUS, &ok)
	if ok == gl.FALSE {
		t.Fatal("link failed")
	}
	return p
}

func writePNG(t *testing.T, path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// comparePNG reports how many pixels differ from the reference frame and by
// how much: the anti-aliased edges may shift by rounding, the lines must not.
func comparePNG(t *testing.T, path string, got *image.NRGBA) {
	f, err := os.Open(path)
	if err != nil {
		t.Error(err)
		return
	}
	defer f.Close()
	want, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	w := want.(*image.NRGBA)
	diff, maxd := 0, 0
	for i := range got.Pix {
		d := int(got.Pix[i]) - int(w.Pix[i])
		d = max(d, -d)
		if d > 2 {
			diff++
		}
		maxd = max(maxd, d)
	}
	t.Logf("  vs %s: %d channels differ by >2, max %d", filepath.Base(path), diff, maxd)
}
