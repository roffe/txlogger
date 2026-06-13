package meshgrid

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sync/atomic"

	"fyne.io/fyne/v2/canvas"
	"github.com/roffe/txlogger/pkg/colors"
)

// GPU renderer: the whole mesh is drawn by a single canvas.Shader. The corner
// values live in a small data texture and the camera in a handful of float
// uniforms; the fragment shader reconstructs each pixel's orthographic view
// ray, walks the grid with a 2D DDA and intersects the two triangles of each
// visited cell. Rotating, zooming and panning therefore cost the CPU nothing
// but a uniform update - no projection, sorting or rasterization per frame -
// and a data edit re-uploads only the (cols+1)x(rows+1) texture.
//
// The grid-space conventions shared between the Go side and the GLSL below:
//   - one grid unit = one cell = cellWidth (32) logical px before scaling
//   - corner (vertex row i, col j) sits at (j, rows-i); +Y is the low-index
//     data rows, exactly like the Oy = (vRows-i)*cellHeight CPU layout
//   - corner height = z_off + z_gain * value16, reproducing
//     Oz = (V - zmin)/zrange * depth in grid units
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

const meshShaderBody = `
#define MAX_STEPS 160

uniform vec2 frame_size;
uniform vec4 rect_coords;

uniform sampler2D mesh_tex;     // (cols+1)x(rows+1) corner values, 16 bit in RG
uniform sampler2D colormap_tex; // 256x1 value -> base color lookup

// camera rotation R, row major; view = R * model
uniform float r0;
uniform float r1;
uniform float r2;
uniform float r3;
uniform float r4;
uniform float r5;
uniform float r6;
uniform float r7;
uniform float r8;

uniform float grid_cols;    // cells in X
uniform float grid_rows;    // cells in Y
uniform float scale_px;     // logical px per grid unit
uniform float height_units; // full value range height, grid units
uniform float z_off;        // corner height = z_off + z_gain * value16
uniform float z_gain;
uniform float center_gx;    // mesh center, grid units
uniform float center_gy;
uniform float center_gz;
uniform float cam_x;        // camera pan, logical px
uniform float cam_y;
uniform float size_w;       // widget size, logical px
uniform float size_h;
uniform float view_zmin;    // view-space depth extent of the mesh, logical px
uniform float view_zrange;
uniform float render_mode;  // 0 solid+wire, 1 solid, 2 wireframe
uniform float light_x;      // light direction in model space, unit length
uniform float light_y;
uniform float light_z;

const float BIG = 100000.0;

float corner_height(float gx, float gy) {
    vec2 uv = vec2((gx + 0.5) / (grid_cols + 1.0), (gy + 0.5) / (grid_rows + 1.0));
    vec4 t = texture2D(mesh_tex, uv);
    return z_off + z_gain * (t.r * 65280.0 + t.g * 255.0) / 65535.0;
}

// narrow the line/AABB overlap [umin, umax] by one slab
void slab(float o, float d, float lo, float hi, inout float umin, inout float umax) {
    if (abs(d) < 0.00000001) {
        if (o < lo || o > hi) {
            umin = BIG;
            umax = -BIG;
        }
        return;
    }
    float t1 = (lo - o) / d;
    float t2 = (hi - o) / d;
    if (t1 > t2) {
        float tmp = t1;
        t1 = t2;
        t2 = tmp;
    }
    umin = max(umin, t1);
    umax = min(umax, t2);
}

// Moeller-Trumbore; on a hit t is the ray parameter and bary the (a, b, c) weights
bool ray_tri(vec3 ro, vec3 rd, vec3 a, vec3 b, vec3 c, out float t, out vec3 bary) {
    t = 0.0;
    bary = vec3(0.0);
    vec3 e1 = b - a;
    vec3 e2 = c - a;
    vec3 pv = cross(rd, e2);
    float det = dot(e1, pv);
    if (abs(det) < 0.0000000001) {
        return false;
    }
    float inv_det = 1.0 / det;
    vec3 tv = ro - a;
    float u = dot(tv, pv) * inv_det;
    if (u < -0.0001 || u > 1.0001) {
        return false;
    }
    vec3 qv = cross(tv, e1);
    float v = dot(rd, qv) * inv_det;
    if (v < -0.0001 || u + v > 1.0001) {
        return false;
    }
    t = dot(e2, qv) * inv_det;
    bary = vec3(1.0 - u - v, u, v);
    return true;
}

// grid point -> (device px x, device px y, view-space z in logical px)
vec3 project_grid(mat3 rot, vec3 g, float pix_scale) {
    vec3 v = rot * ((g - vec3(center_gx, center_gy, center_gz)) * scale_px) - vec3(cam_x, cam_y, 0.0);
    return vec3((v.xy + 0.5 * vec2(size_w, size_h)) * pix_scale, v.z);
}

// value color with the depth shading of getColorWithDepth; h in grid units
vec3 height_color(float h, float view_z) {
    float val = clamp(h / height_units, 0.0, 1.0);
    vec4 base = texture2D(colormap_tex, vec2(val * 0.99609375 + 0.001953125, 0.5));
    float df = clamp((view_z - view_zmin) / view_zrange, 0.0, 1.0);
    vec3 rgb = base.rgb * (0.6 + 0.4 * df);
    rgb.b = min(1.0, rgb.b + (1.0 - df) * 0.05882353);
    // Yellow emphasis from getColorWithDepth, but ramped smoothly: the CPU
    // path applies the 10% boost per vertex and lets Gouraud blur its edge,
    // while this shader runs per pixel, so a hard threshold would draw a
    // visible band where the boost switches on. smoothstep fades it in around
    // pure yellow so the mid-range stays a continuous gradient.
    float yellow = smoothstep(0.6, 0.95, base.r) * smoothstep(0.6, 0.95, base.g) * (1.0 - smoothstep(0.2, 0.4, base.b));
    float boost = 1.0 + 0.1 * yellow;
    rgb.r = min(1.0, rgb.r * boost);
    rgb.g = min(1.0, rgb.g * boost);
    return rgb;
}

// closest-point parameter of p on segment a-b
float seg_param(vec2 p, vec2 a, vec2 b) {
    vec2 e = b - a;
    float ee = dot(e, e);
    if (ee < 0.000001) {
        return 0.0;
    }
    return clamp(dot(p - a, e) / ee, 0.0, 1.0);
}

// anti-aliased coverage of a line of the given half width at distance d
float line_mask(float d, float half_w) {
    return 1.0 - smoothstep(half_w - 0.6, half_w + 0.6, d);
}

// front-to-back "under" compositing of one wireframe segment
void wire_seg(vec2 p_dev, mat3 rot, float pix_scale, float half_w, vec3 a, vec3 b, float fade, inout vec3 acc, inout float acc_a) {
    vec3 pa = project_grid(rot, a, pix_scale);
    vec3 pb = project_grid(rot, b, pix_scale);
    float h = seg_param(p_dev, pa.xy, pb.xy);
    float d = distance(p_dev, mix(pa.xy, pb.xy, h));
    float mask = line_mask(d, half_w);
    if (mask <= 0.0) {
        return;
    }
    vec3 rgb = height_color(mix(a.z, b.z, h), mix(pa.z, pb.z, h)) * fade;
    acc += (1.0 - acc_a) * mask * rgb;
    acc_a += (1.0 - acc_a) * mask;
}

// track the nearest cell border for the solid+wireframe grid lines
void edge_check(vec2 p_dev, vec3 pa, vec3 pb, float ha, float hb, inout float best_d, inout float best_h, inout float best_z) {
    float t = seg_param(p_dev, pa.xy, pb.xy);
    float d = distance(p_dev, mix(pa.xy, pb.xy, t));
    if (d < best_d) {
        best_d = d;
        best_h = mix(ha, hb, t);
        best_z = mix(pa.z, pb.z, t);
    }
}

void main() {
    mat3 rot = mat3(r0, r3, r6, r1, r4, r7, r2, r5, r8);

    float pix_scale = (rect_coords.y - rect_coords.x) / max(size_w, 1.0);
    vec2 p_dev = vec2(gl_FragCoord.x, frame_size.y - gl_FragCoord.y) - rect_coords.xz;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > rect_coords.y - rect_coords.x || p_dev.y > rect_coords.w - rect_coords.z) {
        discard;
    }

    vec2 view_xy = p_dev / pix_scale - 0.5 * vec2(size_w, size_h) + vec2(cam_x, cam_y);

    // pixel ray in grid space: g(u) = g0 + u * dg with u the view-space
    // depth; g0 = transpose(R) * (view_xy, 0) / scale + center
    vec3 g0 = vec3(
        (r0 * view_xy.x + r3 * view_xy.y) / scale_px + center_gx,
        (r1 * view_xy.x + r4 * view_xy.y) / scale_px + center_gy,
        (r2 * view_xy.x + r5 * view_xy.y) / scale_px + center_gz);
    vec3 dg = vec3(r6, r7, r8) / scale_px;

    float z_lo = min(z_off, z_off + z_gain) - 0.05;
    float z_hi = max(z_off, z_off + z_gain) + 0.05;

    float umin = -BIG;
    float umax = BIG;
    slab(g0.x, dg.x, 0.0, grid_cols, umin, umax);
    slab(g0.y, dg.y, 0.0, grid_rows, umin, umax);
    slab(g0.z, dg.z, z_lo, z_hi, umin, umax);
    if (umax <= umin) {
        discard;
    }

    // march from the near side (largest view z) toward the far side
    vec3 ro = g0 + umax * dg;
    vec3 rd = -dg;
    float tend = umax - umin;
    ro += rd * 0.0001;

    float cx = clamp(floor(ro.x), 0.0, grid_cols - 1.0);
    float cy = clamp(floor(ro.y), 0.0, grid_rows - 1.0);

    float step_x = rd.x > 0.0 ? 1.0 : -1.0;
    float step_y = rd.y > 0.0 ? 1.0 : -1.0;
    float td_x = abs(rd.x) < 0.00000001 ? BIG : 1.0 / abs(rd.x);
    float td_y = abs(rd.y) < 0.00000001 ? BIG : 1.0 / abs(rd.y);
    float tm_x = abs(rd.x) < 0.00000001 ? BIG : (rd.x > 0.0 ? cx + 1.0 - ro.x : ro.x - cx) / abs(rd.x);
    float tm_y = abs(rd.y) < 0.00000001 ? BIG : (rd.y > 0.0 ? cy + 1.0 - ro.y : ro.y - cy) / abs(rd.y);

    int mode = int(render_mode + 0.5);
    float half_w = 0.5 * pix_scale;
    vec3 light = vec3(light_x, light_y, light_z);

    vec3 acc = vec3(0.0);
    float acc_a = 0.0;

    for (int i = 0; i < MAX_STEPS; i++) {
        float h_bl = corner_height(cx, cy);
        float h_br = corner_height(cx + 1.0, cy);
        float h_tl = corner_height(cx, cy + 1.0);
        float h_tr = corner_height(cx + 1.0, cy + 1.0);

        // cell corners; the fill splits along A-C like the CPU rasterizer
        // while the wireframe diagonal runs B-D like the CPU line mesh
        vec3 A = vec3(cx, cy + 1.0, h_tl);
        vec3 B = vec3(cx + 1.0, cy + 1.0, h_tr);
        vec3 C = vec3(cx + 1.0, cy, h_br);
        vec3 D = vec3(cx, cy, h_bl);

        if (mode == 2) {
            wire_seg(p_dev, rot, pix_scale, half_w, A, B, 1.0, acc, acc_a);
            wire_seg(p_dev, rot, pix_scale, half_w, B, C, 1.0, acc, acc_a);
            wire_seg(p_dev, rot, pix_scale, half_w, C, D, 1.0, acc, acc_a);
            wire_seg(p_dev, rot, pix_scale, half_w, D, A, 1.0, acc, acc_a);
            wire_seg(p_dev, rot, pix_scale, half_w, B, D, 0.7, acc, acc_a);
            if (acc_a > 0.995) {
                break;
            }
        } else {
            float t1;
            vec3 bc1;
            float t2;
            vec3 bc2;
            bool hit1 = ray_tri(ro, rd, A, B, C, t1, bc1);
            bool hit2 = ray_tri(ro, rd, A, C, D, t2, bc2);
            float best_t = BIG;
            float hit_h = 0.0;
            if (hit1) {
                best_t = t1;
                hit_h = bc1.x * A.z + bc1.y * B.z + bc1.z * C.z;
            }
            if (hit2 && t2 < best_t) {
                best_t = t2;
                hit_h = bc2.x * A.z + bc2.y * C.z + bc2.z * D.z;
            }
            if (best_t < BIG) {
                float view_z = umax - best_t;
                vec3 rgb = height_color(hit_h, view_z);

                vec3 n = cross(C - A, D - B);
                float nl = length(n);
                if (nl > 0.0) {
                    rgb *= 0.6 + 0.4 * abs(dot(n / nl, light));
                }

                if (mode == 0) {
                    vec3 pa = project_grid(rot, A, pix_scale);
                    vec3 pb = project_grid(rot, B, pix_scale);
                    vec3 pc = project_grid(rot, C, pix_scale);
                    vec3 pd = project_grid(rot, D, pix_scale);
                    float best_d = BIG;
                    float line_h = 0.0;
                    float line_z = 0.0;
                    edge_check(p_dev, pa, pb, A.z, B.z, best_d, line_h, line_z);
                    edge_check(p_dev, pb, pc, B.z, C.z, best_d, line_h, line_z);
                    edge_check(p_dev, pc, pd, C.z, D.z, best_d, line_h, line_z);
                    edge_check(p_dev, pd, pa, D.z, A.z, best_d, line_h, line_z);
                    float lm = line_mask(best_d, half_w);
                    if (lm > 0.0) {
                        rgb = mix(rgb, height_color(line_h, line_z) * 0.45, lm);
                    }
                }

                gl_FragColor = vec4(rgb, 1.0);
                return;
            }
        }

        if (min(tm_x, tm_y) >= tend) {
            break;
        }
        if (tm_x < tm_y) {
            cx += step_x;
            tm_x += td_x;
        } else {
            cy += step_y;
            tm_y += td_y;
        }
        if (cx < -0.5 || cx > grid_cols - 0.5 || cy < -0.5 || cy > grid_rows - 0.5) {
            break;
        }
    }

    if (mode == 2 && acc_a > 0.003) {
        gl_FragColor = vec4(acc / acc_a, acc_a);
        return;
    }
    discard;
}
`

func (m *Meshgrid) initShader() {
	m.shader = canvas.NewShader(
		fmt.Sprintf("meshgrid-%d", shaderSeq.Add(1)),
		[]byte(meshShaderPreludeGL+meshShaderBody),
		[]byte(meshShaderPreludeES+meshShaderBody),
	)
	m.shader.Textures = make(map[string]image.Image, 2)
	m.shader.Uniforms = make(map[string]float32, 32)
	m.updateShaderData()
	m.updateShaderColormap()
	m.updateShaderUniforms()
}

// updateShaderData re-encodes the corner values into the mesh data texture
// and the height-mapping uniforms. A fresh image is allocated on purpose:
// the painter re-uploads a texture only when the map entry points at a new
// image.
func (m *Meshgrid) updateShaderData() {
	if m.shader == nil {
		return
	}
	vRows, vCols := m.rows+1, m.cols+1

	// Values are normalized against the actual data extent so the 16-bit
	// quantization keeps full resolution even when zmin/zmax (set by
	// LoadFloat64s) span a wider range than the data.
	dataMin, dataMax := math.Inf(1), math.Inf(-1)
	for i := range m.vertices {
		row := m.vertices[i]
		for j := range row {
			v := row[j].V
			if v < dataMin {
				dataMin = v
			}
			if v > dataMax {
				dataMax = v
			}
		}
	}
	dataRange := dataMax - dataMin

	img := image.NewRGBA(image.Rect(0, 0, vCols, vRows))
	for i := 0; i < vRows; i++ {
		row := m.vertices[i]
		for j := 0; j < vCols; j++ {
			norm := 0.0
			if dataRange > 0 {
				norm = (row[j].V - dataMin) / dataRange
			}
			q := uint16(norm*65535 + 0.5)
			// vertex row i sits at grid Y rows-i (data row 0 is the far
			// edge), which is also its texture row
			img.SetRGBA(j, m.rows-i, color.RGBA{R: uint8(q >> 8), G: uint8(q), A: 0xff})
		}
	}
	m.shader.Textures["mesh_tex"] = img

	// Heights in grid units: z_off + z_gain*norm reproduces the CPU
	// Oz = (V - zmin)/zrange * depth, in units of one cell.
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

	u := m.shader.Uniforms
	u["r0"], u["r1"], u["r2"] = float32(r[0][0]), float32(r[0][1]), float32(r[0][2])
	u["r3"], u["r4"], u["r5"] = float32(r[1][0]), float32(r[1][1]), float32(r[1][2])
	u["r6"], u["r7"], u["r8"] = float32(r[2][0]), float32(r[2][1]), float32(r[2][2])
	u["grid_cols"] = float32(m.cols)
	u["grid_rows"] = float32(m.rows)
	u["scale_px"] = float32(cw * m.scale)
	u["height_units"] = float32(m.depth / cw)
	u["center_gx"] = float32(m.centerX / cw)
	u["center_gy"] = float32(m.centerY/cw - 1)
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
