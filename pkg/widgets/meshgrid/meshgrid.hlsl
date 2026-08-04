// meshgrid.hlsl - HLSL port of meshShaderBody in meshgrid_shader.go, used by
// Fyne's Direct3D 11 driver (the `directx` build tag). initShader hands it to
// canvas.Shader.SourceHLSL alongside the two GLSL sources; every OpenGL target
// ignores it.
//
// Same algorithm, unchanged: orthographic view ray per pixel, 2D DDA across the
// grid, Moeller-Trumbore against the two triangles of each visited cell. All the
// grid-space conventions documented on the Go side still hold.
//
// The driver prepends its shaderCommon prelude, so `frame`, `bounds` and PSIn
// are already declared - only this shader's own uniforms go in the b1 cbuffer
// below. Textures bind by sorted name, which puts colormap_tex at t0 and
// mesh_tex at t1.
//
// Differences from the GLSL, all forced by D3D:
//   - SV_Position and `bounds` are both top-left origin here, so the GLSL
//     "frame.y - gl_FragCoord.y" flip is dropped, and `frame` goes unread.
//   - SampleLevel(..., 0) everywhere: Sample() needs derivatives, which are
//     undefined inside the divergent DDA loop. Neither texture has mips.
//   - ray_tri's hit flag is pulled into its own variable: HLSL does not
//     short-circuit &&, and the right operand reads an out param of the left.
//
// Both samplers are the driver's linear one, matching the GL painter, which
// uploads user textures with ImageScaleSmooth. That is worth knowing when
// reading corner_height: it samples texel centres, where linear filtering
// returns the texel exactly, but cell_ao's half-texel offsets do blend - across
// the 16 bit value's byte split included. Both backends blend identically, so
// the AO term matches; it is a shared quirk, not a port artifact.
//
// The b1 cbuffer is exactly 112 bytes: 28 scalars packing 4 per register, no
// padding. Textures upload top-row-first in both APIs, so updateShaderData's
// row indexing carries over as-is.

#define MAX_STEPS 160

cbuffer MeshCB : register(b1)
{
    // camera rotation R, row major; view = R * model
    float r0, r1, r2;
    float r3, r4, r5;
    float r6, r7, r8;

    float grid_cols;    // cells in X
    float grid_rows;    // cells in Y
    float scale_px;     // logical px per grid unit
    float height_units; // full value range height, grid units
    float z_off;        // corner height = z_off + z_gain * value16
    float z_gain;
    float center_gx;    // mesh center, grid units
    float center_gy;
    float center_gz;
    float cam_x;        // camera pan, logical px
    float cam_y;
    float size_w;       // widget size, logical px
    float size_h;
    float view_zmin;    // view-space depth extent of the mesh, logical px
    float view_zrange;
    float render_mode;  // 0 solid+wire, 1 solid, 2 wireframe
    float light_x;      // light direction in model space, unit length
    float light_y;
    float light_z;
};

// bound by sorted texture name: colormap_tex, then mesh_tex
Texture2D    colormap_tex : register(t0); // 256x1 value -> base color lookup
SamplerState colormap_smp : register(s0);
Texture2D    mesh_tex     : register(t1); // cols x rows cell values (or corner grid), 16 bit in RG
SamplerState mesh_smp     : register(s1);

static const float BIG = 100000.0;

float corner_height(float gx, float gy)
{
    float2 uv = float2((gx + 0.5) / (grid_cols + 1.0), (gy + 0.5) / (grid_rows + 1.0));
    float4 t = mesh_tex.SampleLevel(mesh_smp, uv, 0);
    return z_off + z_gain * (t.r * 65280.0 + t.g * 255.0) / 65535.0;
}

// narrow the line/AABB overlap [umin, umax] by one slab
void slab(float o, float d, float lo, float hi, inout float umin, inout float umax)
{
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
bool ray_tri(float3 ro, float3 rd, float3 a, float3 b, float3 c, out float t, out float3 bary)
{
    t = 0.0;
    bary = float3(0.0, 0.0, 0.0);
    float3 e1 = b - a;
    float3 e2 = c - a;
    float3 pv = cross(rd, e2);
    float det = dot(e1, pv);
    if (abs(det) < 0.0000000001) {
        return false;
    }
    float inv_det = 1.0 / det;
    float3 tv = ro - a;
    float u = dot(tv, pv) * inv_det;
    if (u < -0.0001 || u > 1.0001) {
        return false;
    }
    float3 qv = cross(tv, e1);
    float v = dot(rd, qv) * inv_det;
    if (v < -0.0001 || u + v > 1.0001) {
        return false;
    }
    t = dot(e2, qv) * inv_det;
    bary = float3(1.0 - u - v, u, v);
    return true;
}

// grid point -> (device px x, device px y, view-space z in logical px)
float3 project_grid(float3x3 rot, float3 g, float pix_scale)
{
    float3 v = mul(rot, (g - float3(center_gx, center_gy, center_gz)) * scale_px) - float3(cam_x, cam_y, 0.0);
    return float3((v.xy + 0.5 * float2(size_w, size_h)) * pix_scale, v.z);
}

// value color with the depth shading of getColorWithDepth; h in grid units
float3 height_color(float h, float view_z)
{
    float val = clamp(h / height_units, 0.0, 1.0);
    float4 base = colormap_tex.SampleLevel(colormap_smp, float2(val * 0.99609375 + 0.001953125, 0.5), 0);
    float df = clamp((view_z - view_zmin) / view_zrange, 0.0, 1.0);
    float3 rgb = base.rgb * (0.6 + 0.4 * df);
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
float seg_param(float2 p, float2 a, float2 b)
{
    float2 e = b - a;
    float ee = dot(e, e);
    if (ee < 0.000001) {
        return 0.0;
    }
    return clamp(dot(p - a, e) / ee, 0.0, 1.0);
}

// anti-aliased coverage of a line of the given half width at distance d
float line_mask(float d, float half_w)
{
    return 1.0 - smoothstep(half_w - 0.6, half_w + 0.6, d);
}

// front-to-back "under" compositing of one wireframe segment
void wire_seg(float2 p_dev, float3x3 rot, float pix_scale, float half_w, float3 a, float3 b, float fade, inout float3 acc, inout float acc_a)
{
    float3 pa = project_grid(rot, a, pix_scale);
    float3 pb = project_grid(rot, b, pix_scale);
    float h = seg_param(p_dev, pa.xy, pb.xy);
    float d = distance(p_dev, lerp(pa.xy, pb.xy, h));
    float mask = line_mask(d, half_w);
    if (mask <= 0.0) {
        return;
    }
    float3 rgb = height_color(lerp(a.z, b.z, h), lerp(pa.z, pb.z, h)) * fade;
    acc += (1.0 - acc_a) * mask * rgb;
    acc_a += (1.0 - acc_a) * mask;
}

// track the nearest cell border for the solid+wireframe grid lines
void edge_check(float2 p_dev, float3 pa, float3 pb, float ha, float hb, inout float best_d, inout float best_h, inout float best_z)
{
    float t = seg_param(p_dev, pa.xy, pb.xy);
    float d = distance(p_dev, lerp(pa.xy, pb.xy, t));
    if (d < best_d) {
        best_d = d;
        best_h = lerp(ha, hb, t);
        best_z = lerp(pa.z, pb.z, t);
    }
}

// fake ambient occlusion: darken concave cells (valleys, creases) using the
// height-field Laplacian sampled at the cell centre and its four neighbours.
// A positive Laplacian means the centre sits below its surroundings, so it
// would be shadowed by them; convex ridges (negative) are left untouched.
float cell_ao(float cx, float cy)
{
    float cxm = max(cx - 0.5, 0.0);
    float cxp = min(cx + 1.5, grid_cols);
    float cym = max(cy - 0.5, 0.0);
    float cyp = min(cy + 1.5, grid_rows);
    float hc = corner_height(cx + 0.5, cy + 0.5);
    float lap = corner_height(cxp, cy + 0.5) + corner_height(cxm, cy + 0.5)
              + corner_height(cx + 0.5, cyp) + corner_height(cx + 0.5, cym) - 4.0 * hc;
    float c = clamp(lap / max(height_units, 0.0001), 0.0, 1.0);
    return 1.0 - 0.4 * c;
}

// Blinn-Phong shading with an ambient floor and fake AO. n is the raw cell
// normal from cross(C-A, D-B), which points along -Z for a flat cell, so it is
// flipped to face up. light and view_dir are unit vectors in grid space; the
// specular term is gated to the lit side and the ambient term keeps shadowed
// faces readable instead of black.
float3 shade_surface(float3 base, float3 n, float3 light, float3 view_dir, float ao)
{
    float nl = length(n);
    if (nl <= 0.0) {
        return base * ao;
    }
    float3 N = -n / nl;
    float diff = max(dot(N, light), 0.0);
    float3 H = normalize(light + view_dir);
    float spec = (diff > 0.0) ? pow(max(dot(N, H), 0.0), 32.0) : 0.0;
    float3 col = base * ((0.32 + 0.68 * diff) * ao);
    col += 0.25 * spec;
    return col;
}

float4 main(PSIn input) : SV_TARGET
{
    // HLSL's float3x3 constructor takes rows, and r0..r8 are row major already
    float3x3 rot = float3x3(r0, r1, r2, r3, r4, r5, r6, r7, r8);

    float pix_scale = (bounds.z - bounds.x) / max(size_w, 1.0);
    // SV_Position and bounds are both top-left origin, so no vertical flip here
    float2 p_dev = input.pos.xy - bounds.xy;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > bounds.z - bounds.x || p_dev.y > bounds.w - bounds.y) {
        discard;
    }

    float2 view_xy = p_dev / pix_scale - 0.5 * float2(size_w, size_h) + float2(cam_x, cam_y);

    // pixel ray in grid space: g(u) = g0 + u * dg with u the view-space
    // depth; g0 = transpose(R) * (view_xy, 0) / scale + center
    float3 g0 = float3(
        (r0 * view_xy.x + r3 * view_xy.y) / scale_px + center_gx,
        (r1 * view_xy.x + r4 * view_xy.y) / scale_px + center_gy,
        (r2 * view_xy.x + r5 * view_xy.y) / scale_px + center_gz);
    float3 dg = float3(r6, r7, r8) / scale_px;

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
    float3 ro = g0 + umax * dg;
    float3 rd = -dg;
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
    float3 light = float3(light_x, light_y, light_z);
    // grid-space direction from the surface toward the camera: the view ray
    // marches from near to far along rd = -(r6,r7,r8), so the viewer lies along
    // +(r6,r7,r8), already unit length since the rotation is orthonormal
    float3 view_dir = float3(r6, r7, r8);

    float3 acc = float3(0.0, 0.0, 0.0);
    float acc_a = 0.0;

    // [loop] keeps fxc/dxc from unrolling 160 iterations
    [loop]
    for (int i = 0; i < MAX_STEPS; i++) {
        float h_bl = corner_height(cx, cy);
        float h_br = corner_height(cx + 1.0, cy);
        float h_tl = corner_height(cx, cy + 1.0);
        float h_tr = corner_height(cx + 1.0, cy + 1.0);

        // cell corners; the solid fill chooses its diagonal per cell (below)
        // while the wireframe diagonal runs B-D like the CPU line mesh
        float3 A = float3(cx, cy + 1.0, h_tl);
        float3 B = float3(cx + 1.0, cy + 1.0, h_tr);
        float3 C = float3(cx + 1.0, cy, h_br);
        float3 D = float3(cx, cy, h_bl);

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
            // Two triangles per cell, but the diagonal is chosen per cell so the
            // fold runs between the two closest corners (the smaller of the two
            // diagonal height gaps). A lone outlier - a high peak or a low dip -
            // then falls on a single triangle: the other triangle keeps its three
            // similar corners as a near-flat plateau and only the outlier's
            // triangle slopes. That is the "plateau triangle + sloping triangle"
            // look of T7Suite, instead of the whole quad sagging toward the
            // outlier. Per-triangle normals let the plateau read flat while the
            // slope catches the light.
            bool fold_ac = abs(h_tl - h_br) <= abs(h_tr - h_bl);
            float best_t = BIG;
            float hit_h = 0.0;
            float3 hit_n = float3(0.0, 0.0, -1.0);
            float ts;
            float3 bcs;
            bool hit;
            // the hit flag is hoisted out of the && because HLSL does not
            // short-circuit and the comparison reads ray_tri's out param
            if (fold_ac) {
                hit = ray_tri(ro, rd, A, B, C, ts, bcs);
                if (hit && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * A.z + bcs.y * B.z + bcs.z * C.z;
                    hit_n = cross(B - A, C - A);
                }
                hit = ray_tri(ro, rd, A, C, D, ts, bcs);
                if (hit && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * A.z + bcs.y * C.z + bcs.z * D.z;
                    hit_n = cross(C - A, D - A);
                }
            } else {
                hit = ray_tri(ro, rd, A, B, D, ts, bcs);
                if (hit && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * A.z + bcs.y * B.z + bcs.z * D.z;
                    hit_n = cross(B - A, D - A);
                }
                hit = ray_tri(ro, rd, B, C, D, ts, bcs);
                if (hit && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * B.z + bcs.y * C.z + bcs.z * D.z;
                    hit_n = cross(C - B, D - B);
                }
            }
            if (best_t < BIG) {
                float view_z = umax - best_t;
                float3 rgb = height_color(hit_h, view_z);

                float ao = cell_ao(cx, cy);
                rgb = shade_surface(rgb, hit_n, light, view_dir, ao);

                if (mode == 0) {
                    float3 pa = project_grid(rot, A, pix_scale);
                    float3 pb = project_grid(rot, B, pix_scale);
                    float3 pc = project_grid(rot, C, pix_scale);
                    float3 pd = project_grid(rot, D, pix_scale);
                    float best_d = BIG;
                    float line_h = 0.0;
                    float line_z = 0.0;
                    edge_check(p_dev, pa, pb, A.z, B.z, best_d, line_h, line_z);
                    edge_check(p_dev, pb, pc, B.z, C.z, best_d, line_h, line_z);
                    edge_check(p_dev, pc, pd, C.z, D.z, best_d, line_h, line_z);
                    edge_check(p_dev, pd, pa, D.z, A.z, best_d, line_h, line_z);
                    // only the cell borders are drawn; the per-cell fold diagonal
                    // is left in the cell colour so the two triangles blend
                    float lm = line_mask(best_d, half_w);
                    if (lm > 0.0) {
                        rgb = lerp(rgb, height_color(line_h, line_z) * 0.45, lm);
                    }
                }

                return float4(rgb, 1.0);
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
        return float4(acc / acc_a, acc_a);
    }
    discard;
    return float4(0.0, 0.0, 0.0, 0.0);
}
