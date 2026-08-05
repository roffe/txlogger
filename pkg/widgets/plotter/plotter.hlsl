// plotter.hlsl - HLSL port of plotShaderBody in plotter_shader.go, used by
// Fyne's Direct3D 11 driver (the `directx` build tag). initShader hands it to
// canvas.Shader.SourceHLSL alongside the two GLSL sources; every OpenGL target
// ignores it.
//
// Same algorithm, unchanged: per pixel walk the enabled series, anti-aliased
// polyline distance when zoomed in, min/max column runs (raw or via the 16:1
// mm texture) when zoomed out, combined with an order-independent max blend.
// All the sample encoding and lane conventions documented on the Go side still
// hold.
//
// The driver prepends its common prelude, so `frame`, `bounds` and PSIn are
// already declared - only this shader's own uniforms go in the b1 cbuffer
// below. Textures bind by sorted name, which puts data_tex at t0, meta_tex at
// t1 and mm_tex at t2; all three are always present in Shader.Textures, so that
// assignment never shifts.
//
// Differences from the GLSL, all forced by D3D:
//   - SV_Position and `bounds` are both top-left origin here, so the GLSL
//     "frame.y - gl_FragCoord.y" flip is dropped, and `frame` goes unread.
//   - SampleLevel(..., 0) everywhere: every fetch sits inside a divergent
//     branch or loop where Sample()'s implicit derivatives are undefined. None
//     of the textures have mips.
//   - [loop] on the series and decimation loops: D3DCompile would otherwise
//     unroll 64 series x 24 groups into an enormous program for no gain.
//
// All three samplers are the driver's linear one, matching the GL painter,
// which uploads user textures with ImageScaleSmooth. Every fetch here targets a
// texel centre, where linear filtering returns the texel exactly, so the 16 bit
// decode is unaffected.

#define MAX_SERIES 64
#define MAX_SEG 4
#define MAX_RAW 16
#define MAX_GROUPS 24

cbuffer PlotCB : register(b1)
{
    float series_count;
    float lane_count;   // horizontal bands, 0 = overlay on full height
    float highlight;    // hovered series index, -1 for none
    float plot_start;   // first visible sample
    float points_shown; // visible sample count
    float tex_w;        // texel columns in data_tex and mm_tex
    float rows_raw;     // data_tex rows per series
    float rows_mm;      // mm_tex rows per series
    float data_h;       // data_tex height in rows
    float mm_h;         // mm_tex height in rows
    float meta_h;       // meta_tex height = series count
    float size_w;       // widget size, logical px
    float size_h;
};

// bound by sorted texture name: data_tex, meta_tex, then mm_tex
Texture2D    data_tex : register(t0); // one texel per sample, 16-bit value in RG
SamplerState data_smp : register(s0);
Texture2D    meta_tex : register(t1); // per series: x0 color, x1 enabled, x2 length, x3 lane
SamplerState meta_smp : register(s1);
Texture2D    mm_tex   : register(t2); // min/max per 16 samples: RG=min, BA=max
SamplerState mm_smp   : register(s2);

static const float BIG = 100000.0;

float decode16(float hi, float lo)
{
    return (hi * 65280.0 + lo * 255.0) / 65535.0;
}

float4 meta_at(float x, float si)
{
    return meta_tex.SampleLevel(meta_smp, float2((x + 0.5) / 4.0, (si + 0.5) / meta_h), 0);
}

// normalized sample value; idx is clamped to the series
float sample_val(float si, float idx, float len)
{
    idx = clamp(idx, 0.0, len - 1.0);
    float row = floor(idx / tex_w);
    float colm = idx - row * tex_w;
    float2 uv = float2((colm + 0.5) / tex_w, (si * rows_raw + row + 0.5) / data_h);
    float4 t = data_tex.SampleLevel(data_smp, uv, 0);
    return decode16(t.r, t.g);
}

// normalized (min, max) of sample group gidx
float2 sample_mm(float si, float gidx, float glen)
{
    gidx = clamp(gidx, 0.0, glen - 1.0);
    float row = floor(gidx / tex_w);
    float colm = gidx - row * tex_w;
    float2 uv = float2((colm + 0.5) / tex_w, (si * rows_mm + row + 0.5) / mm_h);
    float4 t = mm_tex.SampleLevel(mm_smp, uv, 0);
    return float2(decode16(t.r, t.g), decode16(t.b, t.a));
}

// device y of a normalized value inside the band [y0, y0+h): texel range 0..1
// spans one display range of headroom on each side of [Min, Max]
float val_y(float v, float y0, float h)
{
    float frac_v = v * 3.0 - 1.0;
    return y0 + (1.0 - frac_v) * (h - 1.0);
}

float seg_dist(float2 p, float2 a, float2 b)
{
    float2 e = b - a;
    float ee = dot(e, e);
    float h = ee > 0.000001 ? clamp(dot(p - a, e) / ee, 0.0, 1.0) : 0.0;
    return distance(p, a + e * h);
}

float4 main(PSIn input) : SV_TARGET
{
    float pix_scale = (bounds.z - bounds.x) / max(size_w, 1.0);
    // SV_Position and bounds are both top-left origin, so no vertical flip here
    float2 p_dev = input.pos.xy - bounds.xy;
    float w_dev = bounds.z - bounds.x;
    float h_dev = bounds.w - bounds.y;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > w_dev || p_dev.y > h_dev) {
        discard;
    }

    float ppd = points_shown / max(w_dev, 1.0);              // samples per device px
    float spos = plot_start + p_dev.x / w_dev * points_shown; // sample at this px
    float aa = 0.6;

    float3 acc = float3(0.0, 0.0, 0.0);
    float acc_a = 0.0;

    [loop] for (int i = 0; i < MAX_SERIES; i++) {
        if (i >= int(series_count + 0.5)) {
            break;
        }
        float si = float(i);
        if (meta_at(1.0, si).r < 0.5) {
            continue; // disabled via the legend
        }
        float4 m2 = meta_at(2.0, si);
        float len = m2.r * 16711680.0 + m2.g * 65280.0 + m2.b * 255.0;
        if (len < 2.0) {
            continue;
        }
        // stacked lanes: each enabled series owns one horizontal band and is
        // clipped to it, mirroring the image backend's sub-image draw
        float lane_y0 = 0.0;
        float lane_h = h_dev;
        if (lane_count >= 1.0) {
            float li = floor(meta_at(3.0, si).r * 255.0 + 0.5);
            lane_y0 = floor(li * h_dev / lane_count);
            lane_h = floor((li + 1.0) * h_dev / lane_count) - lane_y0;
            if (p_dev.y < lane_y0 || p_dev.y > lane_y0 + lane_h) {
                continue;
            }
        }
        // hovered series renders at 4 logical px like PlotImage thickness 4
        float half_w = (abs(si - highlight) < 0.5 ? 2.0 : 0.5) * pix_scale;

        float mask = 0.0;
        if (ppd <= 1.5) {
            // zoomed in: true polyline, distance to the segments around
            // this pixel's column
            float i0 = floor(spos);
            float dmin = BIG;
            [loop] for (int k = -MAX_SEG; k < MAX_SEG; k++) {
                float j = i0 + float(k);
                float x0 = (j - plot_start) / points_shown * w_dev;
                float x1 = (j + 1.0 - plot_start) / points_shown * w_dev;
                float y0 = val_y(sample_val(si, j, len), lane_y0, lane_h);
                float y1 = val_y(sample_val(si, j + 1.0, len), lane_y0, lane_h);
                dmin = min(dmin, seg_dist(p_dev, float2(x0, y0), float2(x1, y1)));
            }
            mask = 1.0 - smoothstep(half_w - aa, half_w + aa, dmin);
        } else {
            // zoomed out: vertical min/max run per column, like
            // plotImageDecimated (including the one-sample overlap into the
            // previous column that keeps runs connected)
            float s_a = spos - ppd * 0.5 - 1.0;
            float s_b = spos + ppd * 0.5;
            float lo = BIG;
            float hi = -BIG;
            if (ppd <= 14.0) {
                [loop] for (int k = 0; k < MAX_RAW; k++) {
                    float idx = s_a + float(k);
                    if (idx > s_b) {
                        break;
                    }
                    float v = sample_val(si, idx, len);
                    lo = min(lo, v);
                    hi = max(hi, v);
                }
            } else {
                float g0 = floor(s_a / 16.0);
                float glen = ceil(len / 16.0);
                [loop] for (int k = 0; k < MAX_GROUPS; k++) {
                    float g = g0 + float(k);
                    if (g * 16.0 > s_b) {
                        break;
                    }
                    float2 mm = sample_mm(si, g, glen);
                    lo = min(lo, mm.x);
                    hi = max(hi, mm.y);
                }
            }
            float d = max(val_y(hi, lane_y0, lane_h) - p_dev.y, p_dev.y - val_y(lo, lane_y0, lane_h));
            mask = 1.0 - smoothstep(half_w - aa, half_w + aa, d);
        }

        // max blend, same as bresenhamCore, so overlap is order independent
        float4 col = meta_at(0.0, si);
        acc = max(acc, col.rgb * mask * col.a);
        acc_a = max(acc_a, mask * col.a);
    }

    if (acc_a < 0.004) {
        discard;
    }
    return float4(acc / acc_a, acc_a);
}
