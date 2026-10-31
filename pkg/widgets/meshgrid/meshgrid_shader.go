package meshgrid

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"sync/atomic"

	"fyne.io/fyne/v2/canvas"
	"github.com/roffe/txlogger/pkg/colors"
)

// meshShaderWGSL is the same shader ported to WGSL for Fyne's WebGPU painter.
// Carrying it unconditionally costs an unreferenced blob on OpenGL targets and
// keeps the widget free of build tags; see its file header for the porting
// notes.
//
//go:embed meshgrid.wgsl
var meshShaderWGSL []byte

// GPU renderer: the whole mesh is drawn by a single canvas.Shader. The mesh
// values live in a small data texture and the camera in a handful of float
// uniforms; the fragment shader reconstructs each pixel's orthographic view
// ray, walks the grid with a 2D DDA and intersects the two triangles of each
// visited cell. Rotating, zooming and panning therefore cost the CPU nothing
// but a uniform update - no projection, sorting or rasterization per frame -
// and a data edit re-uploads only the value texture.
//
// The vertices are the raw cell values (dataVertexMode): the texture is cols x
// rows, the grid has (cols-1)x(rows-1) cells and each cell triangulates the
// four data points around it, so the surface passes exactly through every value
// the way T7Suite's mesh does - a low cell only drops the two triangles meeting
// at that corner, never the wider neighbourhood that averaging values onto a
// shared corner grid would. Degenerate 1xN / Nx1 maps fall back to a corner
// grid (cols+1 x rows+1, value averaged per corner); the GLSL is identical, only
// grid_cols/grid_rows, the texture and the centre uniforms differ.
//
// The grid-space conventions shared between the Go side and the GLSL in meshgrid.frag:
//   - one grid unit = one cell = cellWidth (32) logical px before scaling
//   - in dataVertexMode the vertex for data cell (r, c) sits at grid (c,
//     rows-1-r); +Y is the low-index data rows. center_gx = (cols-1)/2 places
//     it half a cell in from the corner grid, aligning the mesh with the axis
//     ticks (drawn at cell centres) and with projectOriginal
//   - vertex height = z_off + z_gain * value16, reproducing
//     (V - zmin)/zrange * depth in grid units
//   - view = R * ((grid - center) * scale_px) - cam; the viewer sits at +Z
//     looking along -Z, so the nearest surface has the largest view Z

// shaderSeq makes each widget's Shader.Name unique: the painter caches both
// the compiled program and the bound textures per name, so two open map
// windows sharing a name would evict each other's data texture every frame.
var shaderSeq atomic.Int64

const meshShaderPreludeGL = "#version 110\n"

const meshShaderPreludeES = `#version 100
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif
`

//go:embed meshgrid.frag
var meshShaderBody string

func (m *Meshgrid) initShader() {
	m.shader = canvas.NewShader(
		fmt.Sprintf("meshgrid-%d", shaderSeq.Add(1)),
		[]byte(meshShaderPreludeGL+meshShaderBody),
		[]byte(meshShaderPreludeES+meshShaderBody),
	)
	m.shader.SourceWGSL = meshShaderWGSL
	m.shader.Textures = make(map[string]image.Image, 2)
	m.shader.Uniforms = make(map[string]float32, 32)
	m.updateShaderData()
	m.updateShaderColormap()
	m.updateShaderUniforms()
}

// dataVertexMode reports whether the shader treats raw cell values as the mesh
// vertices (the T7Suite-style triangulated surface) rather than averaging them
// onto a corner grid. It needs at least 2x2 cells to span a quad between data
// points; a 1xN / Nx1 map falls back to the corner-averaged path, which already
// renders those degenerate "ribbons" sensibly.
func (m *Meshgrid) dataVertexMode() bool {
	return m.cols >= 2 && m.rows >= 2
}

// updateShaderData re-encodes the mesh values into the data texture and the
// height-mapping uniforms. In dataVertexMode the texture holds the raw cell
// values (one texel per data cell) so the shader's per-cell triangulation
// passes exactly through each value: a low cell only pulls down the two
// triangles touching that corner, never the wider neighbourhood that corner
// averaging would. Otherwise it falls back to the (cols+1)x(rows+1) averaged
// corner grid. A fresh image is allocated on purpose: the painter re-uploads a
// texture only when the map entry points at a new image.
func (m *Meshgrid) updateShaderData() {
	if m.shader == nil {
		return
	}

	// Values are normalized against the actual data extent so the 16-bit
	// quantization keeps full resolution even when zmin/zmax (set by
	// LoadFloat64s) span a wider range than the data.
	dataMin, dataMax := math.Inf(1), math.Inf(-1)
	for _, v := range m.values {
		if v < dataMin {
			dataMin = v
		}
		if v > dataMax {
			dataMax = v
		}
	}
	dataRange := dataMax - dataMin
	encode := func(v float64) color.RGBA {
		norm := 0.0
		if dataRange > 0 {
			norm = (v - dataMin) / dataRange
		}
		q := uint16(norm*65535 + 0.5)
		return color.RGBA{R: uint8(q >> 8), G: uint8(q), A: 0xff}
	}

	var img *image.RGBA
	if m.dataVertexMode() {
		// One texel per data cell; cell (r, c) is the vertex at grid (c,
		// rows-1-r), so data row 0 stays at the far (high-Y) edge.
		img = image.NewRGBA(image.Rect(0, 0, m.cols, m.rows))
		for r := 0; r < m.rows; r++ {
			for c := 0; c < m.cols; c++ {
				img.SetRGBA(c, m.rows-1-r, encode(m.values[r*m.cols+c]))
			}
		}
	} else {
		// Corner-averaged grid: vertex row i sits at grid Y rows-i, which is
		// also its texture row.
		vRows, vCols := m.rows+1, m.cols+1
		img = image.NewRGBA(image.Rect(0, 0, vCols, vRows))
		for i := 0; i < vRows; i++ {
			row := m.vertices[i]
			for j := 0; j < vCols; j++ {
				img.SetRGBA(j, m.rows-i, encode(row[j].V))
			}
		}
	}
	m.shader.Textures["mesh_tex"] = img

	// Heights in grid units: z_off + z_gain*norm reproduces
	// (V - zmin)/zrange * depth, in units of one cell.
	zr := m.zrange
	if zr == 0 {
		zr = 1
	}
	hmax := m.depth / float64(m.cellWidth)
	m.shader.Uniforms["z_off"] = float32((dataMin - m.zmin) / zr * hmax)
	m.shader.Uniforms["z_gain"] = float32(dataRange / zr * hmax)
}

// updateShaderColormap rebuilds the 256x1 value-to-color lookup texture.
func (m *Meshgrid) updateShaderColormap() {
	if m.shader == nil {
		return
	}
	lut := image.NewRGBA(image.Rect(0, 0, 256, 1))
	for k := 0; k < 256; k++ {
		var c color.RGBA
		if m.zrange == 0 {
			// GetColorInterpolation resolves 0/0 to gray; match it
			c = color.RGBA{R: 128, G: 128, B: 128, A: 255}
		} else {
			c = colors.GetColorInterpolation(0, 255, float64(k), m.colorMode)
		}
		lut.SetRGBA(k, 0, c)
	}
	m.shader.Textures["colormap_tex"] = lut
}

// updateShaderUniforms pushes the camera and shading state; this is the whole
// per-frame CPU cost of the shader backend.
func (m *Meshgrid) updateShaderUniforms() {
	if m.shader == nil {
		return
	}

	// View-space depth extent of the transformed mesh, for the same
	// depth-shading ramp the CPU rasterizer applies.
	minZ, maxZ := math.Inf(1), math.Inf(-1)
	for i := range m.vertices {
		row := m.vertices[i]
		for j := range row {
			z := row[j].Z
			if z < minZ {
				minZ = z
			}
			if z > maxZ {
				maxZ = z
			}
		}
	}
	zRange := maxZ - minZ
	if zRange == 0 {
		zRange = 1
	}

	// Fixed view-space light from drawSurface, moved to model space so the
	// shader can shade with grid-space normals directly.
	lx, ly, lz := 0.3, -0.5, 0.8
	il := 1 / math.Sqrt(lx*lx+ly*ly+lz*lz)
	lx, ly, lz = lx*il, ly*il, lz*il

	r := m.cameraRotation
	cw := float64(m.cellWidth)

	// Grid extent and centre depend on the surface model. In dataVertexMode the
	// vertices are the cols x rows data points, so there are (cols-1)x(rows-1)
	// cells and the data point for column c sits half a cell in from the corner
	// grid (centre_gx = (cols-1)/2), which keeps the mesh aligned with the axis
	// ticks (drawn at cell centres) and with projectOriginal. The corner-grid
	// fallback keeps the old cols x rows cells and centre derived from the
	// averaged vertices.
	gridCols, gridRows := float32(m.cols), float32(m.rows)
	centerGx := float32(m.centerX / cw)
	centerGy := float32(m.centerY/cw - 1)
	if m.dataVertexMode() {
		gridCols, gridRows = float32(m.cols-1), float32(m.rows-1)
		centerGx = float32(float64(m.cols-1) / 2)
		centerGy = float32(float64(m.rows-1) / 2)
	}

	u := m.shader.Uniforms
	u["r0"], u["r1"], u["r2"] = float32(r[0][0]), float32(r[0][1]), float32(r[0][2])
	u["r3"], u["r4"], u["r5"] = float32(r[1][0]), float32(r[1][1]), float32(r[1][2])
	u["r6"], u["r7"], u["r8"] = float32(r[2][0]), float32(r[2][1]), float32(r[2][2])
	u["grid_cols"] = gridCols
	u["grid_rows"] = gridRows
	u["scale_px"] = float32(cw * m.scale)
	u["height_units"] = float32(m.depth / cw)
	u["center_gx"] = centerGx
	u["center_gy"] = centerGy
	u["center_gz"] = float32(m.centerZ / cw)
	u["cam_x"] = float32(m.cameraPosition[0])
	u["cam_y"] = float32(m.cameraPosition[1])
	u["size_w"] = float32(m.size.Width)
	u["size_h"] = float32(m.size.Height)
	u["view_zmin"] = float32(minZ)
	u["view_zrange"] = float32(zRange)
	u["render_mode"] = float32(m.renderMode)
	u["light_x"] = float32(r[0][0]*lx + r[1][0]*ly + r[2][0]*lz)
	u["light_y"] = float32(r[0][1]*lx + r[1][1]*ly + r[2][1]*lz)
	u["light_z"] = float32(r[0][2]*lx + r[1][2]*ly + r[2][2]*lz)
}
