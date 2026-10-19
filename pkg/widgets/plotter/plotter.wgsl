// plotter.wgsl - WGSL port of plotter.frag, for Fyne's WebGPU painter
// (canvas.Shader.SourceWGSL); every other renderer ignores it.
//
// Same algorithm, unchanged: per pixel walk the enabled series, anti-aliased
// polyline distance when zoomed in, min/max column runs (raw or via the 16:1
// mm texture) when zoomed out, combined with an order-independent max blend.
// All the sample encoding and lane conventions documented in plotter_shader.go
// still hold.
//
// Painter contract (canvas.Shader.SourceWGSL): Fyne prepends its prelude, so
// fyne.frame and fyne.bounds (pixels from the top left, like
// @builtin(position)) are declared and the entry point is main. The struct u
// at group(2) binding(0) gets Shader.Uniforms by member name, and every
// texture_2d in group(1) the Shader.Textures image of the same name.
//
// Differences from the GLSL:
//   - @builtin(position) and bounds are both top-left origin, so the GLSL
//     "frame.y - gl_FragCoord.y" flip is dropped, and frame goes unread.
//   - The uniforms are members of the PlotUniforms struct u.
//   - textureSampleLevel(..., 0.0) everywhere: every fetch sits inside a
//     divergent branch or loop, where textureSample is not allowed. None of the
//     textures have mips.
//   - The #defines are i32 consts, ?: is select(), and sample_val/sample_mm
//     take idx_in/gidx_in because WGSL parameters are immutable.
//
// All three samplers are linear, clamp-to-edge. Every fetch here targets a
// texel centre, where linear filtering returns the texel exactly, so the 16 bit
// decode is unaffected.

const MAX_SERIES: i32 = 64;
const MAX_SEG: i32 = 16; // covers the hovered 4 px line at ppd 1.5 up to 200% scaling
const MAX_RAW: i32 = 16;
const MAX_GROUPS: i32 = 24;

struct PlotUniforms {
    series_count: f32,
    lane_count: f32,    // horizontal bands, 0 = overlay on full height
    highlight: f32,     // hovered series index, -1 for none
    plot_start: f32,    // first visible sample
    points_shown: f32,  // visible sample count
    tex_w: f32,         // texel columns in data_tex and mm_tex
    rows_raw: f32,      // data_tex rows per series
    rows_mm: f32,       // mm_tex rows per series
    data_h: f32,        // data_tex height in rows
    mm_h: f32,          // mm_tex height in rows
    meta_h: f32,        // meta_tex height = series count
    size_w: f32,        // widget size, logical px
    size_h: f32,
}

@group(2) @binding(0) var<uniform> u: PlotUniforms;

@group(1) @binding(0) var data_tex: texture_2d<f32>; // one texel per sample, 16-bit value in RG
@group(1) @binding(1) var data_smp: sampler;
@group(1) @binding(2) var meta_tex: texture_2d<f32>; // per series: x0 color, x1 RGB length (0 = disabled) A lane
@group(1) @binding(3) var meta_smp: sampler;
@group(1) @binding(4) var mm_tex: texture_2d<f32>;   // min/max per 16 samples: RG=min, BA=max
@group(1) @binding(5) var mm_smp: sampler;

const BIG: f32 = 100000.0;

fn decode16(hi: f32, lo: f32) -> f32 {
    return (hi * 65280.0 + lo * 255.0) / 65535.0;
}

fn meta_at(x: f32, si: f32) -> vec4<f32> {
    return textureSampleLevel(meta_tex, meta_smp, vec2<f32>((x + 0.5) / 2.0, (si + 0.5) / u.meta_h), 0.0);
}

// normalized sample value; idx is clamped to the series
fn sample_val(si: f32, idx_in: f32, len: f32) -> f32 {
    let idx = clamp(idx_in, 0.0, len - 1.0);
    let row = floor(idx / u.tex_w);
    let colm = idx - row * u.tex_w;
    let uv = vec2<f32>((colm + 0.5) / u.tex_w, (si * u.rows_raw + row + 0.5) / u.data_h);
    let t = textureSampleLevel(data_tex, data_smp, uv, 0.0);
    return decode16(t.r, t.g);
}

// normalized (min, max) of sample group gidx
fn sample_mm(si: f32, gidx_in: f32, glen: f32) -> vec2<f32> {
    let gidx = clamp(gidx_in, 0.0, glen - 1.0);
    let row = floor(gidx / u.tex_w);
    let colm = gidx - row * u.tex_w;
    let uv = vec2<f32>((colm + 0.5) / u.tex_w, (si * u.rows_mm + row + 0.5) / u.mm_h);
    let t = textureSampleLevel(mm_tex, mm_smp, uv, 0.0);
    return vec2<f32>(decode16(t.r, t.g), decode16(t.b, t.a));
}

// device y of a normalized value inside the band [y0, y0+h): texel range 0..1
// spans one display range of headroom on each side of [Min, Max]
fn val_y(v: f32, y0: f32, h: f32) -> f32 {
    let frac = v * 3.0 - 1.0;
    return y0 + (1.0 - frac) * (h - 1.0);
}

// false when device y is out of reach of samples [ia, ib] (at most 17, so
// two 16:1 min/max groups cover them): those groups bound every segment and
// column run drawn from the samples, so one or two fetches reject a pixel
// that would otherwise read every sample
fn near_band(si: f32, ia: f32, ib: f32, len: f32, y: f32, y0: f32, h: f32, pad: f32) -> bool {
    let glen = ceil(len / 16.0);
    let ga = floor(clamp(ia, 0.0, len - 1.0) / 16.0);
    let gb = floor(clamp(ib, 0.0, len - 1.0) / 16.0);
    var mm = sample_mm(si, ga, glen);
    if (gb > ga) {
        let mm2 = sample_mm(si, gb, glen);
        mm = vec2<f32>(min(mm.x, mm2.x), max(mm.y, mm2.y));
    }
    return y >= val_y(mm.y, y0, h) - pad && y <= val_y(mm.x, y0, h) + pad;
}

fn seg_dist(p: vec2<f32>, a: vec2<f32>, b: vec2<f32>) -> f32 {
    let e = b - a;
    let ee = dot(e, e);
    let h = select(0.0, clamp(dot(p - a, e) / ee, 0.0, 1.0), ee > 0.000001);
    return distance(p, a + e * h);
}

@fragment
fn main(@builtin(position) pos: vec4<f32>) -> @location(0) vec4<f32> {
    let pix_scale = (fyne.bounds.z - fyne.bounds.x) / max(u.size_w, 1.0);
    // @builtin(position) and bounds are both top-left origin, so no vertical flip here
    let p_dev = pos.xy - fyne.bounds.xy;
    let w_dev = fyne.bounds.z - fyne.bounds.x;
    let h_dev = fyne.bounds.w - fyne.bounds.y;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > w_dev || p_dev.y > h_dev) {
        discard;
    }

    let ppd = u.points_shown / max(w_dev, 1.0);                 // samples per device px
    let spos = u.plot_start + p_dev.x / w_dev * u.points_shown; // sample at this px
    let aa = 0.6;

    var acc = vec3<f32>(0.0);
    var acc_a = 0.0;

    for (var i: i32 = 0; i < MAX_SERIES; i++) {
        if (i >= i32(u.series_count + 0.5)) {
            break;
        }
        let si = f32(i);
        let m1 = meta_at(1.0, si);
        let len = m1.r * 16711680.0 + m1.g * 65280.0 + m1.b * 255.0;
        if (len < 2.0) {
            continue; // too short, or disabled via the legend
        }
        // stacked lanes: each enabled series owns one horizontal band and is
        // clipped to it, mirroring the image backend's sub-image draw
        var lane_y0 = 0.0;
        var lane_h = h_dev;
        if (u.lane_count >= 1.0) {
            let li = floor(m1.a * 255.0 + 0.5);
            lane_y0 = floor(li * h_dev / u.lane_count);
            lane_h = floor((li + 1.0) * h_dev / u.lane_count) - lane_y0;
            if (p_dev.y < lane_y0 || p_dev.y > lane_y0 + lane_h) {
                continue;
            }
        }
        // hovered series renders at 4 logical px like PlotImage thickness 4
        let half_w = select(0.5, 2.0, abs(si - u.highlight) < 0.5) * pix_scale;

        let pad = half_w + aa + 1.0; // mask is 0 from half_w + aa out
        var mask = 0.0;
        if (ppd <= 1.5) {
            // zoomed in: true polyline, distance to the segments that can
            // reach this pixel - a segment is never closer than its
            // horizontal gap, so only those within half_w + aa in x count
            let reach = (half_w + aa) * ppd;
            var j = floor(spos - reach);
            let j_end = min(floor(spos + reach), j + f32(MAX_SEG) - 1.0);
            if (!near_band(si, j, j_end + 1.0, len, p_dev.y, lane_y0, lane_h, pad)) {
                continue;
            }
            var x0 = (j - u.plot_start) / u.points_shown * w_dev;
            var y0 = val_y(sample_val(si, j, len), lane_y0, lane_h);
            var dmin = BIG;
            for (var k: i32 = 0; k < MAX_SEG; k++) {
                if (j > j_end) {
                    break;
                }
                let x1 = (j + 1.0 - u.plot_start) / u.points_shown * w_dev;
                let y1 = val_y(sample_val(si, j + 1.0, len), lane_y0, lane_h);
                dmin = min(dmin, seg_dist(p_dev, vec2<f32>(x0, y0), vec2<f32>(x1, y1)));
                x0 = x1;
                y0 = y1;
                j += 1.0;
            }
            mask = 1.0 - smoothstep(half_w - aa, half_w + aa, dmin);
        } else {
            // zoomed out: vertical min/max run per column, like
            // plotImageDecimated (including the one-sample overlap into the
            // previous column that keeps runs connected)
            let s_a = spos - ppd * 0.5 - 1.0;
            let s_b = spos + ppd * 0.5;
            var lo = BIG;
            var hi = -BIG;
            if (ppd <= 14.0) {
                if (!near_band(si, floor(s_a), ceil(s_b), len, p_dev.y, lane_y0, lane_h, pad)) {
                    continue;
                }
                for (var k: i32 = 0; k < MAX_RAW; k++) {
                    let idx = s_a + f32(k);
                    if (idx > s_b) {
                        break;
                    }
                    let v = sample_val(si, idx, len);
                    lo = min(lo, v);
                    hi = max(hi, v);
                }
            } else {
                let g0 = floor(s_a / 16.0);
                let glen = ceil(len / 16.0);
                for (var k: i32 = 0; k < MAX_GROUPS; k++) {
                    let g = g0 + f32(k);
                    if (g * 16.0 > s_b) {
                        break;
                    }
                    let mm = sample_mm(si, g, glen);
                    lo = min(lo, mm.x);
                    hi = max(hi, mm.y);
                }
            }
            let d = max(val_y(hi, lane_y0, lane_h) - p_dev.y, p_dev.y - val_y(lo, lane_y0, lane_h));
            mask = 1.0 - smoothstep(half_w - aa, half_w + aa, d);
        }
        if (mask <= 0.0) {
            continue; // off the line, as most pixels are: skip the color fetch
        }

        // max blend, same as bresenhamCore, so overlap is order independent
        let col = meta_at(0.0, si);
        acc = max(acc, col.rgb * mask * col.a);
        acc_a = max(acc_a, mask * col.a);
    }

    if (acc_a < 0.004) {
        discard;
    }
    return vec4<f32>(acc / acc_a, acc_a);
}
