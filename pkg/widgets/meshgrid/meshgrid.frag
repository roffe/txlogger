#define MAX_STEPS 160

uniform vec2 frame;
uniform vec4 bounds;

uniform sampler2D mesh_tex;     // cols x rows cell values (or corner grid), 16 bit in RG
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

// value color with depth shading; h in grid units. The wireframe keeps the
// strong ramp and haze of getColorWithDepth, its only depth cue. On the lit
// surface the ramp is slight, so a colour still reads as the value it shows
// in the map.
vec3 height_color(float h, float view_z) {
    float val = clamp(h / height_units, 0.0, 1.0);
    vec4 base = texture2D(colormap_tex, vec2(val * 0.99609375 + 0.001953125, 0.5));
    float df = clamp((view_z - view_zmin) / view_zrange, 0.0, 1.0);
    float dmin = render_mode > 1.5 ? 0.6 : 0.88;
    vec3 rgb = base.rgb * (dmin + (1.0 - dmin) * df);
    if (render_mode > 1.5) {
        rgb.b = min(1.0, rgb.b + (1.0 - df) * 0.05882353);
    }
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
void wire_seg(vec2 p_dev, mat3 rot, float pix_scale, float half_w, vec3 a, vec3 b, inout vec3 acc, inout float acc_a) {
    vec3 pa = project_grid(rot, a, pix_scale);
    vec3 pb = project_grid(rot, b, pix_scale);
    float h = seg_param(p_dev, pa.xy, pb.xy);
    float d = distance(p_dev, mix(pa.xy, pb.xy, h));
    float mask = line_mask(d, half_w);
    if (mask <= 0.0) {
        return;
    }
    vec3 rgb = height_color(mix(a.z, b.z, h), mix(pa.z, pb.z, h));
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

// fake ambient occlusion: darken concave cells (valleys, creases) using the
// height-field Laplacian sampled at the cell centre and its four neighbours.
// A positive Laplacian means the centre sits below its surroundings, so it
// would be shadowed by them; convex ridges (negative) are left untouched.
float cell_ao(float cx, float cy) {
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
vec3 shade_surface(vec3 base, vec3 n, vec3 light, vec3 view_dir, float ao) {
    float nl = length(n);
    if (nl <= 0.0) {
        return base * ao;
    }
    vec3 N = -n / nl;
    if (dot(N, view_dir) < 0.0) {
        return base * (0.32 * ao);
    }
    float diff = max(dot(N, light), 0.0);
    vec3 H = normalize(light + view_dir);
    float spec = (diff > 0.0) ? pow(max(dot(N, H), 0.0), 32.0) : 0.0;
    vec3 col = base * ((0.32 + 0.68 * diff) * ao);
    col += vec3(0.25 * spec);
    return col;
}

void main() {
    mat3 rot = mat3(r0, r3, r6, r1, r4, r7, r2, r5, r8);

    float pix_scale = (bounds.z - bounds.x) / max(size_w, 1.0);
    vec2 p_dev = vec2(gl_FragCoord.x, frame.y - gl_FragCoord.y) - bounds.xy;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > bounds.z - bounds.x || p_dev.y > bounds.w - bounds.y) {
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
    // grid-space direction from the surface toward the camera: the view ray
    // marches from near to far along rd = -(r6,r7,r8), so the viewer lies along
    // +(r6,r7,r8), already unit length since the rotation is orthonormal
    vec3 view_dir = vec3(r6, r7, r8);

    vec3 acc = vec3(0.0);
    float acc_a = 0.0;

    for (int i = 0; i < MAX_STEPS; i++) {
        float h_bl = corner_height(cx, cy);
        float h_br = corner_height(cx + 1.0, cy);
        float h_tl = corner_height(cx, cy + 1.0);
        float h_tr = corner_height(cx + 1.0, cy + 1.0);

        // cell corners; the solid fill chooses its diagonal per cell (below)
        // while the wireframe draws only the cell borders, the same grid the
        // solid+wireframe mode lays over the surface
        vec3 A = vec3(cx, cy + 1.0, h_tl);
        vec3 B = vec3(cx + 1.0, cy + 1.0, h_tr);
        vec3 C = vec3(cx + 1.0, cy, h_br);
        vec3 D = vec3(cx, cy, h_bl);

        if (mode == 2) {
            wire_seg(p_dev, rot, pix_scale, half_w, A, B, acc, acc_a);
            wire_seg(p_dev, rot, pix_scale, half_w, B, C, acc, acc_a);
            wire_seg(p_dev, rot, pix_scale, half_w, C, D, acc, acc_a);
            wire_seg(p_dev, rot, pix_scale, half_w, D, A, acc, acc_a);
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
            vec3 hit_n = vec3(0.0, 0.0, -1.0);
            float ts;
            vec3 bcs;
            if (fold_ac) {
                if (ray_tri(ro, rd, A, B, C, ts, bcs) && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * A.z + bcs.y * B.z + bcs.z * C.z;
                    hit_n = cross(B - A, C - A);
                }
                if (ray_tri(ro, rd, A, C, D, ts, bcs) && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * A.z + bcs.y * C.z + bcs.z * D.z;
                    hit_n = cross(C - A, D - A);
                }
            } else {
                if (ray_tri(ro, rd, A, B, D, ts, bcs) && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * A.z + bcs.y * B.z + bcs.z * D.z;
                    hit_n = cross(B - A, D - A);
                }
                if (ray_tri(ro, rd, B, C, D, ts, bcs) && ts < best_t) {
                    best_t = ts;
                    hit_h = bcs.x * B.z + bcs.y * C.z + bcs.z * D.z;
                    hit_n = cross(C - B, D - B);
                }
            }
            if (best_t < BIG) {
                float view_z = umax - best_t;
                vec3 rgb = height_color(hit_h, view_z);

                float ao = cell_ao(cx, cy);
                rgb = shade_surface(rgb, hit_n, light, view_dir, ao);

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
                    // only the cell borders are drawn; the per-cell fold diagonal
                    // is left in the cell colour so the two triangles blend
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
