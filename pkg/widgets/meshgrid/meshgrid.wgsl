// meshgrid.wgsl - WGSL port of meshgrid.frag, for Fyne's WebGPU painter
// (canvas.Shader.SourceWGSL); every other renderer ignores it.
//
// Same algorithm, unchanged: orthographic view ray per pixel, 2D DDA across the
// grid, Moeller-Trumbore against the two triangles of each visited cell. All the
// grid-space conventions documented in meshgrid_shader.go still hold.
//
// Painter contract (canvas.Shader.SourceWGSL): Fyne prepends its prelude, so
// fyne.frame and fyne.bounds (pixels from the top left, like
// @builtin(position)) are declared and the entry point is main. The struct u
// at group(2) binding(0) gets Shader.Uniforms by member name, and every
// texture_2d in group(1) the Shader.Textures image of the same name.
//
// Both samplers are linear, matching the GL painter. corner_height samples
// texel centres, where linear filtering returns the texel exactly; cell_ao's
// half-texel offsets do blend, across the 16 bit value's byte split included,
// the same on every backend.
//
// Differences from the GLSL:
//   - @builtin(position) and bounds are both top-left origin, so the GLSL
//     "frame.y - gl_FragCoord.y" flip is dropped and frame goes unread.
//   - textureSampleLevel(..., 0.0) everywhere: textureSample needs uniform
//     control flow, and both textures are sampled inside the divergent DDA
//     loop. Neither texture has mips.
//   - out/inout parameters are ptr<function, T> parameters, passed as &var;
//     wire_seg spells "*p += x" out as "*p = *p + x" for gogpu/naga.
//   - ?: is select(false_value, true_value, condition).
//   - ray_tri's hit flag is its own variable, instead of
//     relying on && to call ray_tri before reading the ts it writes.
//   - ray_tri's barycentric u, v are named bu, bv; u is the uniform struct.
//   - mix() with a scalar weight on vectors splats the weight explicitly.
//   - main ends in an unreachable return after discard, since WGSL wants
//     every path of a value-returning function to return.

const MAX_STEPS: i32 = 160;

struct MeshUniforms {
    // camera rotation R, row major; view = R * model
    r0: f32,
    r1: f32,
    r2: f32,
    r3: f32,
    r4: f32,
    r5: f32,
    r6: f32,
    r7: f32,
    r8: f32,

    grid_cols: f32,    // cells in X
    grid_rows: f32,    // cells in Y
    scale_px: f32,     // logical px per grid unit
    height_units: f32, // full value range height, grid units
    z_off: f32,        // corner height = z_off + z_gain * value16
    z_gain: f32,
    center_gx: f32,    // mesh center, grid units
    center_gy: f32,
    center_gz: f32,
    cam_x: f32,        // camera pan, logical px
    cam_y: f32,
    size_w: f32,       // widget size, logical px
    size_h: f32,
    view_zmin: f32,    // view-space depth extent of the mesh, logical px
    view_zrange: f32,
    render_mode: f32,  // 0 solid+wire, 1 solid, 2 wireframe
    light_x: f32,      // light direction in model space, unit length
    light_y: f32,
    light_z: f32,
}

@group(2) @binding(0) var<uniform> u: MeshUniforms;

@group(1) @binding(0) var colormap_tex: texture_2d<f32>; // 256x1 value -> base color lookup
@group(1) @binding(1) var colormap_smp: sampler;
@group(1) @binding(2) var mesh_tex: texture_2d<f32>;     // cols x rows cell values (or corner grid), 16 bit in RG
@group(1) @binding(3) var mesh_smp: sampler;

const BIG: f32 = 100000.0;

fn corner_height(gx: f32, gy: f32) -> f32 {
    let uv = vec2<f32>((gx + 0.5) / (u.grid_cols + 1.0), (gy + 0.5) / (u.grid_rows + 1.0));
    let t = textureSampleLevel(mesh_tex, mesh_smp, uv, 0.0);
    return u.z_off + u.z_gain * (t.r * 65280.0 + t.g * 255.0) / 65535.0;
}

// narrow the line/AABB overlap [umin, umax] by one slab
fn slab(o: f32, d: f32, lo: f32, hi: f32, umin: ptr<function, f32>, umax: ptr<function, f32>) {
    if (abs(d) < 0.00000001) {
        if (o < lo || o > hi) {
            *umin = BIG;
            *umax = -BIG;
        }
        return;
    }
    var t1 = (lo - o) / d;
    var t2 = (hi - o) / d;
    if (t1 > t2) {
        let tmp = t1;
        t1 = t2;
        t2 = tmp;
    }
    *umin = max(*umin, t1);
    *umax = min(*umax, t2);
}

// Moeller-Trumbore; on a hit t is the ray parameter and bary the (a, b, c) weights
fn ray_tri(ro: vec3<f32>, rd: vec3<f32>, a: vec3<f32>, b: vec3<f32>, c: vec3<f32>, t: ptr<function, f32>, bary: ptr<function, vec3<f32>>) -> bool {
    *t = 0.0;
    *bary = vec3<f32>(0.0);
    let e1 = b - a;
    let e2 = c - a;
    let pv = cross(rd, e2);
    let det = dot(e1, pv);
    if (abs(det) < 0.0000000001) {
        return false;
    }
    let inv_det = 1.0 / det;
    let tv = ro - a;
    let bu = dot(tv, pv) * inv_det;
    if (bu < -0.0001 || bu > 1.0001) {
        return false;
    }
    let qv = cross(tv, e1);
    let bv = dot(rd, qv) * inv_det;
    if (bv < -0.0001 || bu + bv > 1.0001) {
        return false;
    }
    *t = dot(e2, qv) * inv_det;
    *bary = vec3<f32>(1.0 - bu - bv, bu, bv);
    return true;
}

// grid point -> (device px x, device px y, view-space z in logical px)
fn project_grid(rot: mat3x3<f32>, g: vec3<f32>, pix_scale: f32) -> vec3<f32> {
    let v = rot * ((g - vec3<f32>(u.center_gx, u.center_gy, u.center_gz)) * u.scale_px) - vec3<f32>(u.cam_x, u.cam_y, 0.0);
    return vec3<f32>((v.xy + 0.5 * vec2<f32>(u.size_w, u.size_h)) * pix_scale, v.z);
}

// value color with depth shading; h in grid units. The wireframe keeps the
// strong ramp and haze of getColorWithDepth, its only depth cue. On the lit
// surface the ramp is slight, so a colour still reads as the value it shows
// in the map.
fn height_color(h: f32, view_z: f32) -> vec3<f32> {
    let val = clamp(h / u.height_units, 0.0, 1.0);
    let base = textureSampleLevel(colormap_tex, colormap_smp, vec2<f32>(val * 0.99609375 + 0.001953125, 0.5), 0.0);
    let df = clamp((view_z - u.view_zmin) / u.view_zrange, 0.0, 1.0);
    let dmin = select(0.88, 0.6, u.render_mode > 1.5);
    var rgb = base.rgb * (dmin + (1.0 - dmin) * df);
    if (u.render_mode > 1.5) {
        rgb.b = min(1.0, rgb.b + (1.0 - df) * 0.05882353);
    }
    // Yellow emphasis from getColorWithDepth, but ramped smoothly: the CPU
    // path applies the 10% boost per vertex and lets Gouraud blur its edge,
    // while this shader runs per pixel, so a hard threshold would draw a
    // visible band where the boost switches on. smoothstep fades it in around
    // pure yellow so the mid-range stays a continuous gradient.
    let yellow = smoothstep(0.6, 0.95, base.r) * smoothstep(0.6, 0.95, base.g) * (1.0 - smoothstep(0.2, 0.4, base.b));
    let boost = 1.0 + 0.1 * yellow;
    rgb.r = min(1.0, rgb.r * boost);
    rgb.g = min(1.0, rgb.g * boost);
    return rgb;
}

// closest-point parameter of p on segment a-b
fn seg_param(p: vec2<f32>, a: vec2<f32>, b: vec2<f32>) -> f32 {
    let e = b - a;
    let ee = dot(e, e);
    if (ee < 0.000001) {
        return 0.0;
    }
    return clamp(dot(p - a, e) / ee, 0.0, 1.0);
}

// anti-aliased coverage of a line of the given half width at distance d
fn line_mask(d: f32, half_w: f32) -> f32 {
    return 1.0 - smoothstep(half_w - 0.6, half_w + 0.6, d);
}

// front-to-back "under" compositing of one wireframe segment
fn wire_seg(p_dev: vec2<f32>, rot: mat3x3<f32>, pix_scale: f32, half_w: f32, a: vec3<f32>, b: vec3<f32>, acc: ptr<function, vec3<f32>>, acc_a: ptr<function, f32>) {
    let pa = project_grid(rot, a, pix_scale);
    let pb = project_grid(rot, b, pix_scale);
    let h = seg_param(p_dev, pa.xy, pb.xy);
    let d = distance(p_dev, mix(pa.xy, pb.xy, vec2<f32>(h)));
    let mask = line_mask(d, half_w);
    if (mask <= 0.0) {
        return;
    }
    let rgb = height_color(mix(a.z, b.z, h), mix(pa.z, pb.z, h));
    *acc = *acc + (1.0 - *acc_a) * mask * rgb;
    *acc_a = *acc_a + (1.0 - *acc_a) * mask;
}

// track the nearest cell border for the solid+wireframe grid lines
fn edge_check(p_dev: vec2<f32>, pa: vec3<f32>, pb: vec3<f32>, ha: f32, hb: f32, best_d: ptr<function, f32>, best_h: ptr<function, f32>, best_z: ptr<function, f32>) {
    let t = seg_param(p_dev, pa.xy, pb.xy);
    let d = distance(p_dev, mix(pa.xy, pb.xy, vec2<f32>(t)));
    if (d < *best_d) {
        *best_d = d;
        *best_h = mix(ha, hb, t);
        *best_z = mix(pa.z, pb.z, t);
    }
}

// fake ambient occlusion: darken concave cells (valleys, creases) using the
// height-field Laplacian sampled at the cell centre and its four neighbours.
// A positive Laplacian means the centre sits below its surroundings, so it
// would be shadowed by them; convex ridges (negative) are left untouched.
fn cell_ao(cx: f32, cy: f32) -> f32 {
    let cxm = max(cx - 0.5, 0.0);
    let cxp = min(cx + 1.5, u.grid_cols);
    let cym = max(cy - 0.5, 0.0);
    let cyp = min(cy + 1.5, u.grid_rows);
    let hc = corner_height(cx + 0.5, cy + 0.5);
    let lap = corner_height(cxp, cy + 0.5) + corner_height(cxm, cy + 0.5)
            + corner_height(cx + 0.5, cyp) + corner_height(cx + 0.5, cym) - 4.0 * hc;
    let c = clamp(lap / max(u.height_units, 0.0001), 0.0, 1.0);
    return 1.0 - 0.4 * c;
}

// Blinn-Phong shading with an ambient floor and fake AO. n is the raw triangle
// normal, which points along -Z for a flat cell, so it is flipped to face up.
// light and view_dir are unit vectors in grid space; the specular term is
// gated to the lit side and the ambient term keeps shadowed faces readable
// instead of black. The steep diffuse term is deliberate: the brightness step
// between differently tilted triangles is what shows each cell's fold, and
// with it the shape of the surface. A wrap-around (half-Lambert) term was
// tried and flattened that step to under half. The underside is left unlit at
// the ambient level whatever the light does: that darkness is how you tell
// which side you are looking at while orbiting.
fn shade_surface(base: vec3<f32>, n: vec3<f32>, light: vec3<f32>, view_dir: vec3<f32>, ao: f32) -> vec3<f32> {
    let nl = length(n);
    if (nl <= 0.0) {
        return base * ao;
    }
    let N = -n / nl;
    if (dot(N, view_dir) < 0.0) {
        return base * (0.32 * ao);
    }
    let diff = max(dot(N, light), 0.0);
    let H = normalize(light + view_dir);
    let spec = select(0.0, pow(max(dot(N, H), 0.0), 32.0), diff > 0.0);
    var col = base * ((0.32 + 0.68 * diff) * ao);
    col += vec3<f32>(0.25 * spec);
    return col;
}

@fragment
fn main(@builtin(position) pos: vec4<f32>) -> @location(0) vec4<f32> {
    let rot = mat3x3<f32>(u.r0, u.r3, u.r6, u.r1, u.r4, u.r7, u.r2, u.r5, u.r8);

    let pix_scale = (fyne.bounds.z - fyne.bounds.x) / max(u.size_w, 1.0);
    // @builtin(position) and bounds are both top-left origin, so no vertical flip here
    let p_dev = pos.xy - fyne.bounds.xy;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > fyne.bounds.z - fyne.bounds.x || p_dev.y > fyne.bounds.w - fyne.bounds.y) {
        discard;
    }

    let view_xy = p_dev / pix_scale - 0.5 * vec2<f32>(u.size_w, u.size_h) + vec2<f32>(u.cam_x, u.cam_y);

    // pixel ray in grid space: g(u) = g0 + u * dg with u the view-space
    // depth; g0 = transpose(R) * (view_xy, 0) / scale + center
    let g0 = vec3<f32>(
        (u.r0 * view_xy.x + u.r3 * view_xy.y) / u.scale_px + u.center_gx,
        (u.r1 * view_xy.x + u.r4 * view_xy.y) / u.scale_px + u.center_gy,
        (u.r2 * view_xy.x + u.r5 * view_xy.y) / u.scale_px + u.center_gz);
    let dg = vec3<f32>(u.r6, u.r7, u.r8) / u.scale_px;

    let z_lo = min(u.z_off, u.z_off + u.z_gain) - 0.05;
    let z_hi = max(u.z_off, u.z_off + u.z_gain) + 0.05;

    var umin = -BIG;
    var umax = BIG;
    slab(g0.x, dg.x, 0.0, u.grid_cols, &umin, &umax);
    slab(g0.y, dg.y, 0.0, u.grid_rows, &umin, &umax);
    slab(g0.z, dg.z, z_lo, z_hi, &umin, &umax);
    if (umax <= umin) {
        discard;
    }

    // march from the near side (largest view z) toward the far side
    var ro = g0 + umax * dg;
    let rd = -dg;
    let tend = umax - umin;
    ro += rd * 0.0001;

    var cx = clamp(floor(ro.x), 0.0, u.grid_cols - 1.0);
    var cy = clamp(floor(ro.y), 0.0, u.grid_rows - 1.0);

    let step_x = select(-1.0, 1.0, rd.x > 0.0);
    let step_y = select(-1.0, 1.0, rd.y > 0.0);
    let td_x = select(1.0 / abs(rd.x), BIG, abs(rd.x) < 0.00000001);
    let td_y = select(1.0 / abs(rd.y), BIG, abs(rd.y) < 0.00000001);
    var tm_x = select(select(ro.x - cx, cx + 1.0 - ro.x, rd.x > 0.0) / abs(rd.x), BIG, abs(rd.x) < 0.00000001);
    var tm_y = select(select(ro.y - cy, cy + 1.0 - ro.y, rd.y > 0.0) / abs(rd.y), BIG, abs(rd.y) < 0.00000001);

    let mode = i32(u.render_mode + 0.5);
    let half_w = 0.5 * pix_scale;
    let light = vec3<f32>(u.light_x, u.light_y, u.light_z);
    // grid-space direction from the surface toward the camera: the view ray
    // marches from near to far along rd = -(r6,r7,r8), so the viewer lies along
    // +(r6,r7,r8), already unit length since the rotation is orthonormal
    let view_dir = vec3<f32>(u.r6, u.r7, u.r8);

    var acc = vec3<f32>(0.0);
    var acc_a = 0.0;

    for (var i: i32 = 0; i < MAX_STEPS; i++) {
        let h_bl = corner_height(cx, cy);
        let h_br = corner_height(cx + 1.0, cy);
        let h_tl = corner_height(cx, cy + 1.0);
        let h_tr = corner_height(cx + 1.0, cy + 1.0);

        // cell corners; the solid fill chooses its diagonal per cell (below)
        // while the wireframe draws only the cell borders, the same grid the
        // solid+wireframe mode lays over the surface
        let A = vec3<f32>(cx, cy + 1.0, h_tl);
        let B = vec3<f32>(cx + 1.0, cy + 1.0, h_tr);
        let C = vec3<f32>(cx + 1.0, cy, h_br);
        let D = vec3<f32>(cx, cy, h_bl);

        if (mode == 2) {
            wire_seg(p_dev, rot, pix_scale, half_w, A, B, &acc, &acc_a);
            wire_seg(p_dev, rot, pix_scale, half_w, B, C, &acc, &acc_a);
            wire_seg(p_dev, rot, pix_scale, half_w, C, D, &acc, &acc_a);
            wire_seg(p_dev, rot, pix_scale, half_w, D, A, &acc, &acc_a);
            if (acc_a > 0.995) {
                break;
            }
        } else {
            // Two triangles per cell, but the diagonal is chosen per cell so the
            // fold runs between the two closest corners (the smaller of the two
            // diagonal height gaps). A lone outlier - a high peak or a low dip -
            // then falls on a single triangle: the other triangle keeps its three
            // similar corners as a near-flat plateau and only the outlier's
            // triangle slopes. That is the "plateau triangle + sloping triangle"
            // look of T7Suite, instead of the whole quad sagging toward the
            // outlier. Per-triangle normals let the plateau read flat while the
            // slope catches the light.
            let fold_ac = abs(h_tl - h_br) <= abs(h_tr - h_bl);
            var best_t = BIG;
            var hit_h = 0.0;
            var hit_n = vec3<f32>(0.0, 0.0, -1.0);
            var ts: f32;
            var bcs: vec3<f32>;
            var hit: bool;
            // the hit flag is its own variable, so ts and bcs are read in a
            // later statement than the ray_tri call that writes them
            if (fold_ac) {
                hit = ray_tri(ro, rd, A, B, C, &ts, &bcs);
                if (hit && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * A.z + bcs.y * B.z + bcs.z * C.z;
                    hit_n = cross(B - A, C - A);
                }
                hit = ray_tri(ro, rd, A, C, D, &ts, &bcs);
                if (hit && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * A.z + bcs.y * C.z + bcs.z * D.z;
                    hit_n = cross(C - A, D - A);
                }
            } else {
                hit = ray_tri(ro, rd, A, B, D, &ts, &bcs);
                if (hit && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * A.z + bcs.y * B.z + bcs.z * D.z;
                    hit_n = cross(B - A, D - A);
                }
                hit = ray_tri(ro, rd, B, C, D, &ts, &bcs);
                if (hit && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * B.z + bcs.y * C.z + bcs.z * D.z;
                    hit_n = cross(C - B, D - B);
                }
            }
            if (best_t < BIG) {
                let view_z = umax - best_t;
                var rgb = height_color(hit_h, view_z);

                let ao = cell_ao(cx, cy);
                rgb = shade_surface(rgb, hit_n, light, view_dir, ao);

                if (mode == 0) {
                    let pa = project_grid(rot, A, pix_scale);
                    let pb = project_grid(rot, B, pix_scale);
                    let pc = project_grid(rot, C, pix_scale);
                    let pd = project_grid(rot, D, pix_scale);
                    var best_d = BIG;
                    var line_h = 0.0;
                    var line_z = 0.0;
                    edge_check(p_dev, pa, pb, A.z, B.z, &best_d, &line_h, &line_z);
                    edge_check(p_dev, pb, pc, B.z, C.z, &best_d, &line_h, &line_z);
                    edge_check(p_dev, pc, pd, C.z, D.z, &best_d, &line_h, &line_z);
                    edge_check(p_dev, pd, pa, D.z, A.z, &best_d, &line_h, &line_z);
                    // only the cell borders are drawn; the per-cell fold diagonal
                    // is left in the cell colour so the two triangles blend
                    let lm = line_mask(best_d, half_w);
                    if (lm > 0.0) {
                        rgb = mix(rgb, height_color(line_h, line_z) * 0.45, vec3<f32>(lm));
                    }
                }

                return vec4<f32>(rgb, 1.0);
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
        if (cx < -0.5 || cx > u.grid_cols - 0.5 || cy < -0.5 || cy > u.grid_rows - 0.5) {
            break;
        }
    }

    if (mode == 2 && acc_a > 0.003) {
        return vec4<f32>(acc / acc_a, acc_a);
    }
    discard;
    return vec4<f32>(0.0);
}
