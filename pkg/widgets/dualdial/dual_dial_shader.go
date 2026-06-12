package dualdial

// GPU renderer: like the dial widget, the whole gauge geometry (face rim,
// pips, both needles and center cap) is drawn by a single canvas.Shader;
// a SetValue/SetValue2 only writes that needle's angle uniform. Text
// (title, both values, pip labels) stays as canvas.Text layered on top.
//
// All dual dials share one Shader.Name: the painter caches the compiled
// program per name and the widget needs no textures, so every instance
// reuses the same program with its own uniforms.
//
// Conventions match the dial shader: angles are radians, 0 pointing up,
// clockwise positive; the needles sweep Pi15 (270 degrees) from -3/4 pi at
// Min to +3/4 pi at Max. There is no marker overhanging the rim, so the
// shader quad is exactly the dial square.

const dualDialShaderPreludeGL = "#version 110\n"

const dualDialShaderPreludeES = `#version 100
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif
`

const dualDialShaderBody = `
uniform vec2 frame_size;
uniform vec4 rect_coords;

uniform float size_d;    // dial diameter, logical px
uniform float steps;     // pip intervals; steps+1 pips are drawn
uniform float needle_a;  // primary needle angle, radians
uniform float needle2_a; // secondary needle angle, radians

const float PIP_ANG  = 2.35619449; // 3/4 pi, the first/last pip angle
const float FACE_ANG = 2.3693;     // rim ends just past the end pips, like the canvas.Arc face

const vec3 FACE_COL    = vec3(0.502, 0.502, 0.502);    // 0x808080
const vec3 CENTER_COL  = vec3(0.0039, 0.0431, 0.0745); // 0x010B13
const vec3 NEEDLE_COL  = vec3(1.0, 0.4039, 0.0);       // 0xFF6700
const vec3 NEEDLE2_COL = vec3(0.9765, 0.1059, 0.0078); // 0xF91B02

// distance to the radial bar at angle a covering radius [r0, r1], half width hw
float radial_d(vec2 p, float a, float r0, float r1, float hw) {
    vec2 dir = vec2(sin(a), -cos(a));
    float u = dot(p, dir);
    float v = dot(p, vec2(-dir.y, dir.x));
    return length(vec2(u - clamp(u, r0, r1), v)) - hw;
}

// 1 px anti-aliased coverage of signed distance d (device px)
float aa(float d) {
    return clamp(0.5 - d, 0.0, 1.0);
}

// src-over: lay coverage a of colour c on top; col stays premultiplied
void over(inout vec3 col, inout float alpha, vec3 c, float a) {
    col = col * (1.0 - a) + c * a;
    alpha = alpha * (1.0 - a) + a;
}

void main() {
    vec2 ext = vec2(rect_coords.y - rect_coords.x, rect_coords.w - rect_coords.z);
    vec2 p_dev = vec2(gl_FragCoord.x, frame_size.y - gl_FragCoord.y) - rect_coords.xz;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > ext.x || p_dev.y > ext.y) {
        discard;
    }

    float px = ext.x / max(size_d, 1.0); // device px per logical px
    float r = 0.5 * ext.x;               // dial radius, device px
    vec2 p = p_dev - 0.5 * ext;

    float len = length(p);
    float theta = atan(p.x, -p.y); // 0 up, clockwise positive

    vec3 col = vec3(0.0);
    float alpha = 0.0;

    // pips: strokes are far thinner than the pip spacing, so only the
    // nearest pip can cover this pixel - no loop needed
    float n = max(steps, 1.0);
    float step_a = 4.71238898 / n; // Pi15 between first and last pip
    float i = clamp(floor((theta + PIP_ANG) / step_a + 0.5), 0.0, n);
    float odd = mod(i, 2.0);
    float hw = 0.5 * px * mix(max(2.0, size_d / 80.0), max(2.0, size_d / 200.0), odd);
    float rin = mix(0.75, 0.875, odd) * r;
    // intersect with a disc one logical px inside the rim edge: the round
    // end cap must not poke past the rim, and the AA fringe of the cut has
    // to stay under the opaque part of the rim
    float d = max(radial_d(p, i * step_a - PIP_ANG, rin, r - px, hw), len - (r - px));
    // green -> yellow -> red, like the CPU pip gradient
    float t = i / n;
    vec3 pip_col = vec3(clamp(2.0 * t, 0.0, 1.0), clamp(2.0 - 2.0 * t, 0.0, 1.0), 0.0);
    over(col, alpha, pip_col, aa(d));

    // face rim: the ring [0.985r, r] over the pip arc; the angular term is
    // the arc length past the rim ends
    d = max(abs(len - 0.9925 * r) - 0.0075 * r, (abs(theta) - FACE_ANG) * len);
    over(col, alpha, FACE_COL, aa(d));

    // center cap, diameter r/4
    over(col, alpha, CENTER_COL, aa(len - 0.125 * r));

    // needles: offset -0.15r, length 1.14r, tips pulled in 2 logical px;
    // the primary draws on top
    float nhw = 0.5 * px * (size_d / 60.0);
    float ntip = 0.99 * r - 2.0 * px;
    over(col, alpha, NEEDLE2_COL, aa(radial_d(p, needle2_a, -0.15 * r, ntip, nhw)));
    over(col, alpha, NEEDLE_COL, aa(radial_d(p, needle_a, -0.15 * r, ntip, nhw)));

    if (alpha < 0.004) {
        discard;
    }
    gl_FragColor = vec4(col / alpha, alpha);
}
`
