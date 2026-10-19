// tunnel.wgsl - WGSL port of tunnel.frag, for Fyne's WebGPU painter
// (canvas.Shader.SourceWGSL); every other renderer ignores it.
//
// Same effect: procedural retrowave tunnel, credits crawl, bouncing logo
// billboard. The uniforms and textures are set up in tunnel_shader.go.
//
// Painter contract (canvas.Shader.SourceWGSL): Fyne prepends its prelude, so
// fyne.frame and fyne.bounds (pixels from the top left, like
// @builtin(position)) are declared and the entry point is main. The struct u
// at group(2) binding(0) gets Shader.Uniforms by member name, and every
// texture_2d in group(1) the Shader.Textures image of the same name.
//
// Differences from the GLSL:
//   - @builtin(position) and `bounds` are both top-left origin, so the GLSL
//     "frame.y - gl_FragCoord.y" flip is dropped, and `frame` goes unread.
//   - Uniforms are read through the struct `u`, so the GLSL's tube coordinate
//     `u` is named `tu` here, and the crawl's sampled alpha is `ta` instead of
//     a second `a`.
//   - textureSampleLevel(..., 0.0): both samples sit inside non-uniform
//     branches, where textureSample is rejected. Neither texture has mips.
//   - The GLSL's reversed-edge smoothstep(hi, lo, x) calls, undefined in WGSL,
//     are written as 1.0 - smoothstep(lo, hi, x).
//   - `cond ? a : b` defaults are select(b, a, cond).

const PI: f32 = 3.14159265359;

struct TunnelUniforms {
    time: f32,          // elapsed animation seconds

    // optional tuning knobs (default sensibly if left at 0 by the Go side)
    speed: f32,         // forward ride speed
    ring_freq: f32,     // rings per depth unit
    rib_freq: f32,      // longitudinal ribs around the tube
    logo_scale: f32,    // logo billboard half-size, short-axis units

    crawl_lines: f32,   // number of credit lines (0 = no crawl)
    crawl_persp: f32,   // floor depth at the bottom edge
    crawl_width: f32,   // text block half-width in world units
    crawl_zlines: f32,  // credit lines spanned per unit of depth
    crawl_speed: f32,   // scroll rate, lines per second
}

@group(2) @binding(0) var<uniform> u: TunnelUniforms;

@group(1) @binding(0) var crawl_tex: texture_2d<f32>; // credits strip, line 0 at the top (t = 0)
@group(1) @binding(1) var crawl_smp: sampler;
@group(1) @binding(2) var logo_tex: texture_2d<f32>;  // txlogger logo, premultiplied RGBA
@group(1) @binding(3) var logo_smp: sampler;

fn hsv2rgb(c: vec3<f32>) -> vec3<f32> {
    let p = abs(fract(c.xxx + vec3<f32>(0.0, 2.0 / 3.0, 1.0 / 3.0)) * 6.0 - 3.0);
    return c.z * mix(vec3<f32>(1.0), clamp(p - 1.0, vec3<f32>(0.0), vec3<f32>(1.0)), vec3<f32>(c.y));
}

// glowing line at the nearest multiple of 1.0 of x, half width hw
fn grid_line(x: f32, hw: f32) -> f32 {
    let f = fract(x);
    let d = min(f, 1.0 - f);
    return 1.0 - smoothstep(0.0, hw, d);
}

// triangle wave in [-1, 1] with period 1, for DVD-screensaver bouncing
fn tri(x: f32) -> f32 {
    return abs(fract(x) - 0.5) * 4.0 - 1.0;
}

@fragment
fn main(@builtin(position) pos: vec4<f32>) -> @location(0) vec4<f32> {
    let size = vec2<f32>(fyne.bounds.z - fyne.bounds.x, fyne.bounds.w - fyne.bounds.y);
    // @builtin(position) and bounds are both top-left origin, so no vertical flip here
    let p_dev = pos.xy - fyne.bounds.xy;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > size.x || p_dev.y > size.y) {
        discard;
    }

    let spd = select(1.1, u.speed, u.speed > 0.0);
    let rings = select(5.0, u.ring_freq, u.ring_freq > 0.0);
    let ribs = select(16.0, u.rib_freq, u.rib_freq > 0.0);

    // centered, aspect-correct coordinates, roughly [-1, 1] on the short axis.
    // sc stays pristine for the logo billboard; uv is warped into the tunnel.
    let sc = (p_dev - 0.5 * size) / min(size.x, size.y) * 2.0;
    var uv = sc;

    // rollercoaster: drift the vanishing point along a couple of detuned
    // sinusoids so the track appears to bank and weave. vp is the hole's
    // position in screen (sc) space; the bank below rotates around it, so the
    // tunnel mouth always sits at sc == vp. The crawl reuses vp as the far
    // point its text converges into.
    let vp = vec2<f32>(sin(u.time * 0.7) * 0.34 + sin(u.time * 1.31) * 0.11,
                       cos(u.time * 0.53) * 0.30 + cos(u.time * 1.79) * 0.09);
    uv -= vp;

    // bank the whole frame into the curves
    let bank = sin(u.time * 0.61) * 0.55;
    let cb = cos(bank);
    let sb = sin(bank);
    uv = mat2x2<f32>(cb, -sb, sb, cb) * uv;

    let r = length(uv);
    let a = atan2(uv.y, uv.x);

    // tube coordinates: depth grows toward the centre, ride forward over time,
    // and add a spiral twist with depth for the psytrance vortex
    let depth = 1.0 / max(r, 0.0008);
    let v = depth + u.time * spd;
    let tu = a / PI + depth * 0.18 + u.time * 0.04;

    // wireframe: cyan rings across the tube, magenta ribs along it. The line
    // width shrinks with depth so far geometry naturally thins toward the
    // vanishing point.
    let lw = clamp(0.02 + r * 0.10, 0.02, 0.22);
    let ring = grid_line(v * rings, lw);
    let rib = grid_line(tu * ribs, lw * 0.9);

    let hue = fract(0.58 + v * 0.025 + tu * 0.05 + u.time * 0.05);
    let ringCol = hsv2rgb(vec3<f32>(hue, 0.85, 1.0));
    let ribCol = hsv2rgb(vec3<f32>(fract(hue + 0.45), 0.9, 1.0));

    var col = ring * ringCol * 1.3 + rib * ribCol * 1.0;

    // fade the grid out right at the centre so it doesn't smear into a blob
    col *= smoothstep(0.0, 0.10, r);

    // pulsing neon "light at the end of the tunnel"
    let pulse = 0.55 + 0.45 * sin(u.time * 2.3);
    col += vec3<f32>(0.75, 0.30, 1.0) * (1.0 - smoothstep(0.0, 0.45, r)) * pulse;

    // dark retrowave haze between the lines so the tube reads as a surface
    let haze = mix(vec3<f32>(0.03, 0.0, 0.07), vec3<f32>(0.12, 0.01, 0.20),
                   vec3<f32>(0.5 + 0.5 * sin(v * 0.5 + u.time * 0.5)));
    col += haze * (1.0 - clamp(ring + rib, 0.0, 1.0)) * smoothstep(0.0, 0.12, r);

    // vignette toward the outer edge
    col *= 1.0 - 0.40 * smoothstep(0.7, 1.5, r);

    // --- Star Wars credits crawl: flows up the floor into the moving hole ---
    // A floor plane recedes from the bottom edge (near) toward the tunnel
    // mouth (far), but its far end now converges on the drifting hole vp
    // instead of the screen centre. yh is depth below the moving horizon
    // (the floor lives where sc.y > vp.y); z = persp/yh is depth along it.
    // The lane centre slides from x = 0 at the bottom to vp.x at the horizon,
    // so the text stays anchored where it emerges yet leans and sways toward
    // the wandering hole. s narrows the text with depth, and lf scrolls in
    // line units that wrap with fract (a blank first/last line hides the loop
    // seam) while staying < 0 on the first pass so the strip rises in from the
    // bottom.
    let yh = sc.y - vp.y;
    if (u.crawl_lines > 0.5 && yh > 0.045) {
        let cw = select(0.9, u.crawl_width, u.crawl_width > 0.0);
        let cz = select(3.0, u.crawl_zlines, u.crawl_zlines > 0.0);
        let cp = select(0.45, u.crawl_persp, u.crawl_persp > 0.0);
        let cs = select(0.8, u.crawl_speed, u.crawl_speed > 0.0);

        let z = cp / yh;
        // g runs 0 at the bottom anchor to 1 at the horizon. The lane centre
        // first leans on a straight line from x = 0 to the hole (vp.x)...
        let g = clamp((1.0 - sc.y) / (1.0 - vp.y), 0.0, 1.0);
        var centerX = vp.x * g;
        // ...then bows sideways so the path curves into the mouth along with
        // the tunnel. g*(1-g) pins both ends (bottom anchor and hole) while the
        // shared bank phase, plus a depth twist, make deeper sections lead the
        // curve the same way the rings bank and spiral.
        centerX += 0.9 * g * (1.0 - g) * sin(bank * 2.0 - g * 2.5);
        let s = (sc.x - centerX) * z / cw + 0.5;
        let lf = (u.time * cs - z * cz) / u.crawl_lines;
        if (s >= 0.0 && s <= 1.0 && lf >= 0.0) {
            let t = fract(lf);
            var ta = textureSampleLevel(crawl_tex, crawl_smp, vec2<f32>(s, t), 0.0).a;
            ta *= smoothstep(0.045, 0.18, yh) * (1.0 - smoothstep(1.0, 1.5, sc.y));
            let crawlCol = vec3<f32>(1.0, 0.85, 0.2); // classic crawl yellow
            col = mix(col, crawlCol, vec3<f32>(ta));
            col += crawlCol * ta * 0.25;              // soft neon glow
        }
    }

    // --- spinning, DVD-bouncing txlogger logo billboard, composited on top ---
    // DVD-screensaver bounce: detuned triangle waves keep it inside the frame
    let lc = vec2<f32>(tri(u.time * 0.27) * 0.58, tri(u.time * 0.21) * 0.52);

    // grow and shrink as it rides the tunnel toward and away from the viewer
    let depthBob = 0.70 + 0.35 * sin(u.time * 0.8);
    let lscale = select(0.30, u.logo_scale, u.logo_scale > 0.0) * depthBob;

    // rotation couples to the bounce path: the angle tracks position, so its
    // spin rate and direction follow the travel heading and flip at every wall
    // hit (lc.x reverses on the side walls, lc.y on the top/bottom).
    let ang = lc.x * 10.0 + lc.y * 7.0;
    let ca = cos(ang);
    let sa = sin(ang);
    let luv = mat2x2<f32>(ca, -sa, sa, ca) * ((sc - lc) / lscale);
    if (abs(luv.x) <= 1.0 && abs(luv.y) <= 1.0) {
        let logo = textureSampleLevel(logo_tex, logo_smp, luv * 0.5 + 0.5, 0.0);
        // source-over with premultiplied alpha, then a pulsing neon rim glow
        col = col * (1.0 - logo.a) + logo.rgb;
        col += logo.a * (0.4 + 0.4 * sin(u.time * 2.0)) * 0.25 * vec3<f32>(0.6, 0.25, 1.0);
    }

    return vec4<f32>(col, 1.0);
}
