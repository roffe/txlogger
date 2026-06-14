package tunnel

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"log"
	"sync/atomic"

	"fyne.io/fyne/v2/canvas"
)

//go:embed logo.png
var logoPNG []byte

// The whole effect is a single procedural fragment shader: no geometry, no
// textures, no per-frame CPU work beyond the "time" uniform that
// canvas.NewShaderAnimation advances. Each pixel reconstructs an infinite
// tube in polar coordinates - angle around the tube, 1/radius into it - and
// draws a glowing retrowave wireframe of rings and ribs that streams toward
// the viewer. A swaying vanishing point plus a depth-dependent twist give the
// rollercoaster ride; hue cycling over depth and time keeps it psychedelic.

// tunnelSeq makes each widget's Shader.Name unique so the painter caches one
// compiled program per live widget instead of evicting a shared one.
var tunnelSeq atomic.Int64

const tunnelPreludeGL = "#version 110\n"

const tunnelPreludeES = `#version 100
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif
`

const tunnelBody = `
#define PI 3.14159265359

uniform vec2 frame_size;   // output frame size, device px
uniform vec4 rect_coords;  // this object's bounds (x1, x2, y1, y2), device px
uniform float time;        // elapsed animation seconds

// optional tuning knobs (default sensibly if left at 0 by the Go side)
uniform float speed;       // forward ride speed
uniform float ring_freq;   // rings per depth unit
uniform float rib_freq;    // longitudinal ribs around the tube
uniform float logo_scale;  // logo billboard half-size, short-axis units

uniform float crawl_lines; // number of credit lines (0 = no crawl)
uniform float crawl_persp; // floor depth at the bottom edge
uniform float crawl_width; // text block half-width in world units
uniform float crawl_zlines; // credit lines spanned per unit of depth
uniform float crawl_speed; // scroll rate, lines per second

uniform sampler2D logo_tex;  // txlogger logo, premultiplied RGBA
uniform sampler2D crawl_tex; // credits strip, line 0 at the top (t = 0)

vec3 hsv2rgb(vec3 c) {
    vec3 p = abs(fract(c.xxx + vec3(0.0, 2.0 / 3.0, 1.0 / 3.0)) * 6.0 - 3.0);
    return c.z * mix(vec3(1.0), clamp(p - 1.0, 0.0, 1.0), c.y);
}

// glowing line at the nearest multiple of 1.0 of x, half width hw
float grid_line(float x, float hw) {
    float f = fract(x);
    float d = min(f, 1.0 - f);
    return smoothstep(hw, 0.0, d);
}

// triangle wave in [-1, 1] with period 1, for DVD-screensaver bouncing
float tri(float x) {
    return abs(fract(x) - 0.5) * 4.0 - 1.0;
}

void main() {
    vec2 size = vec2(rect_coords.y - rect_coords.x, rect_coords.w - rect_coords.z);
    vec2 p_dev = vec2(gl_FragCoord.x, frame_size.y - gl_FragCoord.y) - rect_coords.xz;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > size.x || p_dev.y > size.y) {
        discard;
    }

    float spd = speed > 0.0 ? speed : 1.1;
    float rings = ring_freq > 0.0 ? ring_freq : 5.0;
    float ribs = rib_freq > 0.0 ? rib_freq : 16.0;

    // centered, aspect-correct coordinates, roughly [-1, 1] on the short axis.
    // sc stays pristine for the logo billboard; uv is warped into the tunnel.
    vec2 sc = (p_dev - 0.5 * size) / min(size.x, size.y) * 2.0;
    vec2 uv = sc;

    // rollercoaster: drift the vanishing point along a couple of detuned
    // sinusoids so the track appears to bank and weave. vp is the hole's
    // position in screen (sc) space; the bank below rotates around it, so the
    // tunnel mouth always sits at sc == vp. The crawl reuses vp as the far
    // point its text converges into.
    vec2 vp = vec2(sin(time * 0.7) * 0.34 + sin(time * 1.31) * 0.11,
                   cos(time * 0.53) * 0.30 + cos(time * 1.79) * 0.09);
    uv -= vp;

    // bank the whole frame into the curves
    float bank = sin(time * 0.61) * 0.55;
    float cb = cos(bank), sb = sin(bank);
    uv = mat2(cb, -sb, sb, cb) * uv;

    float r = length(uv);
    float a = atan(uv.y, uv.x);

    // tube coordinates: depth grows toward the centre, ride forward over time,
    // and add a spiral twist with depth for the psytrance vortex
    float depth = 1.0 / max(r, 0.0008);
    float v = depth + time * spd;
    float u = a / PI + depth * 0.18 + time * 0.04;

    // wireframe: cyan rings across the tube, magenta ribs along it. The line
    // width shrinks with depth so far geometry naturally thins toward the
    // vanishing point.
    float lw = clamp(0.02 + r * 0.10, 0.02, 0.22);
    float ring = grid_line(v * rings, lw);
    float rib = grid_line(u * ribs, lw * 0.9);

    float hue = fract(0.58 + v * 0.025 + u * 0.05 + time * 0.05);
    vec3 ringCol = hsv2rgb(vec3(hue, 0.85, 1.0));
    vec3 ribCol = hsv2rgb(vec3(fract(hue + 0.45), 0.9, 1.0));

    vec3 col = ring * ringCol * 1.3 + rib * ribCol * 1.0;

    // fade the grid out right at the centre so it doesn't smear into a blob
    col *= smoothstep(0.0, 0.10, r);

    // pulsing neon "light at the end of the tunnel"
    float pulse = 0.55 + 0.45 * sin(time * 2.3);
    col += vec3(0.75, 0.30, 1.0) * smoothstep(0.45, 0.0, r) * pulse;

    // dark retrowave haze between the lines so the tube reads as a surface
    vec3 haze = mix(vec3(0.03, 0.0, 0.07), vec3(0.12, 0.01, 0.20),
                    0.5 + 0.5 * sin(v * 0.5 + time * 0.5));
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
    float yh = sc.y - vp.y;
    if (crawl_lines > 0.5 && yh > 0.045) {
        float cw = crawl_width > 0.0 ? crawl_width : 0.9;
        float cz = crawl_zlines > 0.0 ? crawl_zlines : 3.0;
        float cp = crawl_persp > 0.0 ? crawl_persp : 0.45;
        float cs = crawl_speed > 0.0 ? crawl_speed : 0.8;

        float z = cp / yh;
        // g runs 0 at the bottom anchor to 1 at the horizon. The lane centre
        // first leans on a straight line from x = 0 to the hole (vp.x)...
        float g = clamp((1.0 - sc.y) / (1.0 - vp.y), 0.0, 1.0);
        float centerX = vp.x * g;
        // ...then bows sideways so the path curves into the mouth along with
        // the tunnel. g*(1-g) pins both ends (bottom anchor and hole) while the
        // shared bank phase, plus a depth twist, make deeper sections lead the
        // curve the same way the rings bank and spiral.
        centerX += 0.9 * g * (1.0 - g) * sin(bank * 2.0 - g * 2.5);
        float s = (sc.x - centerX) * z / cw + 0.5;
        float lf = (time * cs - z * cz) / crawl_lines;
        if (s >= 0.0 && s <= 1.0 && lf >= 0.0) {
            float t = fract(lf);
            float a = texture2D(crawl_tex, vec2(s, t)).a;
            a *= smoothstep(0.045, 0.18, yh) * smoothstep(1.5, 1.0, sc.y);
            vec3 crawlCol = vec3(1.0, 0.85, 0.2); // classic crawl yellow
            col = mix(col, crawlCol, a);
            col += crawlCol * a * 0.25;           // soft neon glow
        }
    }

    // --- spinning, DVD-bouncing txlogger logo billboard, composited on top ---
    // DVD-screensaver bounce: detuned triangle waves keep it inside the frame
    vec2 lc = vec2(tri(time * 0.27) * 0.58, tri(time * 0.21) * 0.52);

    // grow and shrink as it rides the tunnel toward and away from the viewer
    float depthBob = 0.70 + 0.35 * sin(time * 0.8);
    float lscale = (logo_scale > 0.0 ? logo_scale : 0.30) * depthBob;

    // rotation couples to the bounce path: the angle tracks position, so its
    // spin rate and direction follow the travel heading and flip at every wall
    // hit (lc.x reverses on the side walls, lc.y on the top/bottom).
    float ang = lc.x * 10.0 + lc.y * 7.0;
    float ca = cos(ang), sa = sin(ang);
    vec2 luv = mat2(ca, -sa, sa, ca) * ((sc - lc) / lscale);
    if (abs(luv.x) <= 1.0 && abs(luv.y) <= 1.0) {
        vec4 logo = texture2D(logo_tex, luv * 0.5 + 0.5);
        // source-over with premultiplied alpha, then a pulsing neon rim glow
        col = col * (1.0 - logo.a) + logo.rgb;
        col += logo.a * (0.4 + 0.4 * sin(time * 2.0)) * 0.25 * vec3(0.6, 0.25, 1.0);
    }

    gl_FragColor = vec4(col, 1.0);
}
`

func (t *Tunnel) initShader() {
	t.shader = canvas.NewShader(
		fmt.Sprintf("tunnel-%d", tunnelSeq.Add(1)),
		[]byte(tunnelPreludeGL+tunnelBody),
		[]byte(tunnelPreludeES+tunnelBody),
	)
	t.shader.Uniforms = map[string]float32{
		"time":         0,
		"speed":        1.1,
		"ring_freq":    5.0,
		"rib_freq":     16.0,
		"logo_scale":   0.30,
		"crawl_lines":  0, // no credits until SetCredits is called
		"crawl_persp":  0.45,
		"crawl_width":  0.9,
		"crawl_zlines": 3.0,
		"crawl_speed":  0.8,
	}
	if logo := loadLogo(); logo != nil {
		t.shader.Textures = map[string]image.Image{"logo_tex": logo}
	}
}

// loadLogo decodes the embedded txlogger logo into a premultiplied *image.RGBA,
// the form the GL painter uploads verbatim (image row 0 -> texture t=0, so the
// logo samples upright). Returns nil if decoding fails, in which case the
// shader simply samples an unbound texture and draws no logo.
func loadLogo() image.Image {
	img, _, err := image.Decode(bytes.NewReader(logoPNG))
	if err != nil {
		log.Printf("tunnel: decoding logo.png: %v", err)
		return nil
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	return rgba
}
