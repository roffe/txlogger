// dual_dial.wgsl - WGSL port of dual_dial.frag, for Fyne's WebGPU painter
// (canvas.Shader.SourceWGSL); every other renderer ignores it.
//
// Same gauge, unchanged: face rim, pips, center cap and both needles in one
// pass. Angles and sizes follow the conventions in dual_dial_shader.go.
//
// Painter contract (canvas.Shader.SourceWGSL): Fyne prepends its prelude, so
// fyne.frame and fyne.bounds (pixels from the top left, like
// @builtin(position)) are declared and the entry point is main. The struct u
// at group(2) binding(0) gets Shader.Uniforms by member name, and every
// texture_2d in group(1) the Shader.Textures image of the same name.
//
// Differences from the GLSL:
//   - @builtin(position) and bounds are both top-left origin, so the GLSL
//     "frame.y - gl_FragCoord.y" flip is dropped and frame goes unread.
//   - over() takes function pointers for col and alpha in place of inout.
//   - mod() is glmod(), the floored modulo GLSL defines.
//   - the colour consts live in main: gogpu/naga turns a module-scope
//     vec3 const into OpConstantNull, so they would all draw black.

struct DualDialUniforms {
    size_d: f32,    // dial diameter, logical px
    steps: f32,     // pip intervals; steps+1 pips are drawn
    needle_a: f32,  // primary needle angle, radians
    needle2_a: f32, // secondary needle angle, radians
};

@group(2) @binding(0) var<uniform> u: DualDialUniforms;

const PIP_ANG: f32 = 2.35619449; // 3/4 pi, the first/last pip angle
const FACE_ANG: f32 = 2.3693;    // rim ends just past the end pips, like the canvas.Arc face


// floored modulo, like GLSL mod()
fn glmod(x: f32, y: f32) -> f32 {
    return x - y * floor(x / y);
}

// distance to the radial bar at angle a covering radius [r0, r1], half width hw
fn radial_d(p: vec2<f32>, a: f32, r0: f32, r1: f32, hw: f32) -> f32 {
    let dir = vec2<f32>(sin(a), -cos(a));
    let u = dot(p, dir);
    let v = dot(p, vec2<f32>(-dir.y, dir.x));
    return length(vec2<f32>(u - clamp(u, r0, r1), v)) - hw;
}

// 1 px anti-aliased coverage of signed distance d (device px)
fn aa(d: f32) -> f32 {
    return clamp(0.5 - d, 0.0, 1.0);
}

// src-over: lay coverage a of colour c on top; col stays premultiplied
fn over(col: ptr<function, vec3<f32>>, alpha: ptr<function, f32>, c: vec3<f32>, a: f32) {
    *col = *col * (1.0 - a) + c * a;
    *alpha = *alpha * (1.0 - a) + a;
}

@fragment
fn main(@builtin(position) pos: vec4<f32>) -> @location(0) vec4<f32> {
    const FACE_COL    = vec3<f32>(0.502, 0.502, 0.502);    // 0x808080
    const CENTER_COL  = vec3<f32>(0.0039, 0.0431, 0.0745); // 0x010B13
    const NEEDLE_COL  = vec3<f32>(1.0, 0.4039, 0.0);       // 0xFF6700
    const NEEDLE2_COL = vec3<f32>(0.9765, 0.1059, 0.0078); // 0xF91B02

    let ext = vec2<f32>(fyne.bounds.z - fyne.bounds.x, fyne.bounds.w - fyne.bounds.y);
    // @builtin(position) and bounds are both top-left origin, so no vertical flip here
    let p_dev = pos.xy - fyne.bounds.xy;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > ext.x || p_dev.y > ext.y) {
        discard;
    }

    let px = ext.x / max(u.size_d, 1.0); // device px per logical px
    let r = 0.5 * ext.x;                 // dial radius, device px
    let p = p_dev - 0.5 * ext;

    let len = length(p);
    let theta = atan2(p.x, -p.y); // 0 up, clockwise positive

    var col = vec3<f32>(0.0);
    var alpha: f32 = 0.0;

    // pips: strokes are far thinner than the pip spacing, so only the
    // nearest pip can cover this pixel - no loop needed
    let n = max(u.steps, 1.0);
    let step_a = 4.71238898 / n; // Pi15 between first and last pip
    let i = clamp(floor((theta + PIP_ANG) / step_a + 0.5), 0.0, n);
    let odd = glmod(i, 2.0);
    let hw = 0.5 * px * mix(max(2.0, u.size_d / 80.0), max(2.0, u.size_d / 200.0), odd);
    let rin = mix(0.75, 0.875, odd) * r;
    // intersect with a disc one logical px inside the rim edge: the round
    // end cap must not poke past the rim, and the AA fringe of the cut has
    // to stay under the opaque part of the rim
    var d = max(radial_d(p, i * step_a - PIP_ANG, rin, r - px, hw), len - (r - px));
    // green -> yellow -> red, like the CPU pip gradient
    let t = i / n;
    let pip_col = vec3<f32>(clamp(2.0 * t, 0.0, 1.0), clamp(2.0 - 2.0 * t, 0.0, 1.0), 0.0);
    over(&col, &alpha, pip_col, aa(d));

    // face rim: the ring [0.985r, r] over the pip arc; the angular term is
    // the arc length past the rim ends
    d = max(abs(len - 0.9925 * r) - 0.0075 * r, (abs(theta) - FACE_ANG) * len);
    over(&col, &alpha, FACE_COL, aa(d));

    // center cap, diameter r/4
    over(&col, &alpha, CENTER_COL, aa(len - 0.125 * r));

    // needles: offset -0.15r, length 1.14r, tips pulled in 2 logical px;
    // the primary draws on top
    let nhw = 0.5 * px * (u.size_d / 60.0);
    let ntip = 0.99 * r - 2.0 * px;
    over(&col, &alpha, NEEDLE2_COL, aa(radial_d(p, u.needle2_a, -0.15 * r, ntip, nhw)));
    over(&col, &alpha, NEEDLE_COL, aa(radial_d(p, u.needle_a, -0.15 * r, ntip, nhw)));

    if (alpha < 0.004) {
        discard;
    }
    return vec4<f32>(col / alpha, alpha);
}
