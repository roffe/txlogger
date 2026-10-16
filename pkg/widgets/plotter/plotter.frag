#define MAX_SERIES 64
#define MAX_SEG 16 // covers the hovered 4 px line at ppd 1.5 up to 200% scaling
#define MAX_RAW 16
#define MAX_GROUPS 24

uniform vec2 frame;
uniform vec4 bounds;

uniform sampler2D data_tex; // one texel per sample, 16-bit value in RG
uniform sampler2D mm_tex;   // min/max per 16 samples: RG=min, BA=max
uniform sampler2D meta_tex; // per series: x0 color, x1 RGB length (0 = disabled) A lane

uniform float series_count;
uniform float lane_count;   // horizontal bands, 0 = overlay on full height
uniform float highlight;    // hovered series index, -1 for none
uniform float plot_start;   // first visible sample
uniform float points_shown; // visible sample count
uniform float tex_w;        // texel columns in data_tex and mm_tex
uniform float rows_raw;     // data_tex rows per series
uniform float rows_mm;      // mm_tex rows per series
uniform float data_h;       // data_tex height in rows
uniform float mm_h;         // mm_tex height in rows
uniform float meta_h;       // meta_tex height = series count
uniform float size_w;       // widget size, logical px
uniform float size_h;

const float BIG = 100000.0;

float decode16(float hi, float lo) {
    return (hi * 65280.0 + lo * 255.0) / 65535.0;
}

vec4 meta_at(float x, float si) {
    return texture2D(meta_tex, vec2((x + 0.5) / 2.0, (si + 0.5) / meta_h));
}

// normalized sample value; idx is clamped to the series
float sample_val(float si, float idx, float len) {
    idx = clamp(idx, 0.0, len - 1.0);
    float row = floor(idx / tex_w);
    float colm = idx - row * tex_w;
    vec2 uv = vec2((colm + 0.5) / tex_w, (si * rows_raw + row + 0.5) / data_h);
    vec4 t = texture2D(data_tex, uv);
    return decode16(t.r, t.g);
}

// normalized (min, max) of sample group gidx
vec2 sample_mm(float si, float gidx, float glen) {
    gidx = clamp(gidx, 0.0, glen - 1.0);
    float row = floor(gidx / tex_w);
    float colm = gidx - row * tex_w;
    vec2 uv = vec2((colm + 0.5) / tex_w, (si * rows_mm + row + 0.5) / mm_h);
    vec4 t = texture2D(mm_tex, uv);
    return vec2(decode16(t.r, t.g), decode16(t.b, t.a));
}

// device y of a normalized value inside the band [y0, y0+h): texel range 0..1
// spans one display range of headroom on each side of [Min, Max]
float val_y(float v, float y0, float h) {
    float frac = v * 3.0 - 1.0;
    return y0 + (1.0 - frac) * (h - 1.0);
}

// false when device y is out of reach of samples [ia, ib] (at most 17, so
// two 16:1 min/max groups cover them): those groups bound every segment and
// column run drawn from the samples, so one or two fetches reject a pixel
// that would otherwise read every sample
bool near_band(float si, float ia, float ib, float len, float y, float y0, float h, float pad) {
    float glen = ceil(len / 16.0);
    float ga = floor(clamp(ia, 0.0, len - 1.0) / 16.0);
    float gb = floor(clamp(ib, 0.0, len - 1.0) / 16.0);
    vec2 mm = sample_mm(si, ga, glen);
    if (gb > ga) {
        vec2 mm2 = sample_mm(si, gb, glen);
        mm = vec2(min(mm.x, mm2.x), max(mm.y, mm2.y));
    }
    return y >= val_y(mm.y, y0, h) - pad && y <= val_y(mm.x, y0, h) + pad;
}

float seg_dist(vec2 p, vec2 a, vec2 b) {
    vec2 e = b - a;
    float ee = dot(e, e);
    float h = ee > 0.000001 ? clamp(dot(p - a, e) / ee, 0.0, 1.0) : 0.0;
    return distance(p, a + e * h);
}

void main() {
    float pix_scale = (bounds.z - bounds.x) / max(size_w, 1.0);
    vec2 p_dev = vec2(gl_FragCoord.x, frame.y - gl_FragCoord.y) - bounds.xy;
    float w_dev = bounds.z - bounds.x;
    float h_dev = bounds.w - bounds.y;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > w_dev || p_dev.y > h_dev) {
        discard;
    }

    float ppd = points_shown / max(w_dev, 1.0);              // samples per device px
    float spos = plot_start + p_dev.x / w_dev * points_shown; // sample at this px
    float aa = 0.6;

    vec3 acc = vec3(0.0);
    float acc_a = 0.0;

    for (int i = 0; i < MAX_SERIES; i++) {
        if (i >= int(series_count + 0.5)) {
            break;
        }
        float si = float(i);
        vec4 m1 = meta_at(1.0, si);
        float len = m1.r * 16711680.0 + m1.g * 65280.0 + m1.b * 255.0;
        if (len < 2.0) {
            continue; // too short, or disabled via the legend
        }
        // stacked lanes: each enabled series owns one horizontal band and is
        // clipped to it, mirroring the image backend's sub-image draw
        float lane_y0 = 0.0;
        float lane_h = h_dev;
        if (lane_count >= 1.0) {
            float li = floor(m1.a * 255.0 + 0.5);
            lane_y0 = floor(li * h_dev / lane_count);
            lane_h = floor((li + 1.0) * h_dev / lane_count) - lane_y0;
            if (p_dev.y < lane_y0 || p_dev.y > lane_y0 + lane_h) {
                continue;
            }
        }
        // hovered series renders at 4 logical px like PlotImage thickness 4
        float half_w = (abs(si - highlight) < 0.5 ? 2.0 : 0.5) * pix_scale;

        float pad = half_w + aa + 1.0; // mask is 0 from half_w + aa out
        float mask = 0.0;
        if (ppd <= 1.5) {
            // zoomed in: true polyline, distance to the segments that can
            // reach this pixel - a segment is never closer than its
            // horizontal gap, so only those within half_w + aa in x count
            float reach = (half_w + aa) * ppd;
            float j = floor(spos - reach);
            float j_end = min(floor(spos + reach), j + float(MAX_SEG) - 1.0);
            if (!near_band(si, j, j_end + 1.0, len, p_dev.y, lane_y0, lane_h, pad)) {
                continue;
            }
            float x0 = (j - plot_start) / points_shown * w_dev;
            float y0 = val_y(sample_val(si, j, len), lane_y0, lane_h);
            float dmin = BIG;
            for (int k = 0; k < MAX_SEG; k++) {
                if (j > j_end) {
                    break;
                }
                float x1 = (j + 1.0 - plot_start) / points_shown * w_dev;
                float y1 = val_y(sample_val(si, j + 1.0, len), lane_y0, lane_h);
                dmin = min(dmin, seg_dist(p_dev, vec2(x0, y0), vec2(x1, y1)));
                x0 = x1;
                y0 = y1;
                j += 1.0;
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
                if (!near_band(si, floor(s_a), ceil(s_b), len, p_dev.y, lane_y0, lane_h, pad)) {
                    continue;
                }
                for (int k = 0; k < MAX_RAW; k++) {
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
                for (int k = 0; k < MAX_GROUPS; k++) {
                    float g = g0 + float(k);
                    if (g * 16.0 > s_b) {
                        break;
                    }
                    vec2 mm = sample_mm(si, g, glen);
                    lo = min(lo, mm.x);
                    hi = max(hi, mm.y);
                }
            }
            float d = max(val_y(hi, lane_y0, lane_h) - p_dev.y, p_dev.y - val_y(lo, lane_y0, lane_h));
            mask = 1.0 - smoothstep(half_w - aa, half_w + aa, d);
        }
        if (mask <= 0.0) {
            continue; // off the line, as most pixels are: skip the color fetch
        }

        // max blend, same as bresenhamCore, so overlap is order independent
        vec4 col = meta_at(0.0, si);
        acc = max(acc, col.rgb * mask * col.a);
        acc_a = max(acc_a, mask * col.a);
    }

    if (acc_a < 0.004) {
        discard;
    }
    gl_FragColor = vec4(acc / acc_a, acc_a);
}
