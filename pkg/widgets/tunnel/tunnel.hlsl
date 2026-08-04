// tunnel.hlsl - HLSL port of tunnelBody in tunnel_shader.go, used by Fyne's
// Direct3D 11 driver (the `directx` build tag). initShader hands it to
// canvas.Shader.SourceHLSL alongside the two GLSL sources; every OpenGL target
// ignores it.
//
// Same effect, unchanged: procedural retrowave tunnel, credits crawl, bouncing
// logo billboard. All the coordinate and texture conventions documented on the
// Go side still hold.
//
// The driver prepends its common prelude, so `frame`, `bounds`, PSIn and PI
// are already declared - only this shader's own uniforms go in the b1 cbuffer
// below. Textures bind by sorted name, which puts crawl_tex at t0 and logo_tex
// at t1; the Go side keeps a 1x1 transparent crawl_tex bound when there are no
// credits so that assignment never shifts.
//
// Differences from the GLSL, all forced by D3D:
//   - SV_Position and `bounds` are both top-left origin here, so the GLSL
//     "frame.y - gl_FragCoord.y" flip is dropped, and `frame` goes unread.
//   - SampleLevel(..., 0): both samples sit inside divergent branches where
//     Sample()'s implicit derivatives are undefined. Neither texture has mips.
//   - The GLSL's reversed-edge smoothstep(hi, lo, x) calls, formally undefined
//     in both languages, are written as 1.0 - smoothstep(lo, hi, x).
//   - GLSL's column-major mat2 rotations are written out as the two dot
//     products instead of a float2x2, sidestepping the row/column swap.

cbuffer TunnelCB : register(b1)
{
    float time;        // elapsed animation seconds

    // optional tuning knobs (default sensibly if left at 0 by the Go side)
    float speed;       // forward ride speed
    float ring_freq;   // rings per depth unit
    float rib_freq;    // longitudinal ribs around the tube
    float logo_scale;  // logo billboard half-size, short-axis units

    float crawl_lines; // number of credit lines (0 = no crawl)
    float crawl_persp; // floor depth at the bottom edge
    float crawl_width; // text block half-width in world units
    float crawl_zlines; // credit lines spanned per unit of depth
    float crawl_speed; // scroll rate, lines per second
};

// bound by sorted texture name: crawl_tex, then logo_tex
Texture2D    crawl_tex : register(t0); // credits strip, line 0 at the top (t = 0)
SamplerState crawl_smp : register(s0);
Texture2D    logo_tex  : register(t1); // txlogger logo, premultiplied RGBA
SamplerState logo_smp  : register(s1);

float3 hsv2rgb(float3 c)
{
    float3 p = abs(frac(c.xxx + float3(0.0, 2.0 / 3.0, 1.0 / 3.0)) * 6.0 - 3.0);
    return c.z * lerp(float3(1.0, 1.0, 1.0), clamp(p - 1.0, 0.0, 1.0), c.y);
}

// glowing line at the nearest multiple of 1.0 of x, half width hw
float grid_line(float x, float hw)
{
    float f = frac(x);
    float d = min(f, 1.0 - f);
    return 1.0 - smoothstep(0.0, hw, d);
}

// triangle wave in [-1, 1] with period 1, for DVD-screensaver bouncing
float tri(float x)
{
    return abs(frac(x) - 0.5) * 4.0 - 1.0;
}

float4 main(PSIn input) : SV_TARGET
{
    float2 size = float2(bounds.z - bounds.x, bounds.w - bounds.y);
    // SV_Position and bounds are both top-left origin, so no vertical flip here
    float2 p_dev = input.pos.xy - bounds.xy;

    // the painter expands the quad slightly for edge softness; stay inside
    if (p_dev.x < 0.0 || p_dev.y < 0.0 || p_dev.x > size.x || p_dev.y > size.y) {
        discard;
    }

    float spd = speed > 0.0 ? speed : 1.1;
    float rings = ring_freq > 0.0 ? ring_freq : 5.0;
    float ribs = rib_freq > 0.0 ? rib_freq : 16.0;

    // centered, aspect-correct coordinates, roughly [-1, 1] on the short axis.
    // sc stays pristine for the logo billboard; uv is warped into the tunnel.
    float2 sc = (p_dev - 0.5 * size) / min(size.x, size.y) * 2.0;
    float2 uv = sc;

    // rollercoaster vanishing point; see tunnel_shader.go for the geometry
    float2 vp = float2(sin(time * 0.7) * 0.34 + sin(time * 1.31) * 0.11,
                       cos(time * 0.53) * 0.30 + cos(time * 1.79) * 0.09);
    uv -= vp;

    // bank the whole frame into the curves
    float bank = sin(time * 0.61) * 0.55;
    float cb = cos(bank), sb = sin(bank);
    uv = float2(cb * uv.x + sb * uv.y, -sb * uv.x + cb * uv.y);

    float r = length(uv);
    float a = atan2(uv.y, uv.x);

    // tube coordinates: depth grows toward the centre, ride forward over time,
    // and add a spiral twist with depth for the psytrance vortex
    float depth = 1.0 / max(r, 0.0008);
    float v = depth + time * spd;
    float u = a / PI + depth * 0.18 + time * 0.04;

    // wireframe: rings across the tube, ribs along it, thinning with depth
    float lw = clamp(0.02 + r * 0.10, 0.02, 0.22);
    float ring = grid_line(v * rings, lw);
    float rib = grid_line(u * ribs, lw * 0.9);

    float hue = frac(0.58 + v * 0.025 + u * 0.05 + time * 0.05);
    float3 ringCol = hsv2rgb(float3(hue, 0.85, 1.0));
    float3 ribCol = hsv2rgb(float3(frac(hue + 0.45), 0.9, 1.0));

    float3 col = ring * ringCol * 1.3 + rib * ribCol * 1.0;

    // fade the grid out right at the centre so it doesn't smear into a blob
    col *= smoothstep(0.0, 0.10, r);

    // pulsing neon "light at the end of the tunnel"
    float pulse = 0.55 + 0.45 * sin(time * 2.3);
    col += float3(0.75, 0.30, 1.0) * (1.0 - smoothstep(0.0, 0.45, r)) * pulse;

    // dark retrowave haze between the lines so the tube reads as a surface
    float3 haze = lerp(float3(0.03, 0.0, 0.07), float3(0.12, 0.01, 0.20),
                       0.5 + 0.5 * sin(v * 0.5 + time * 0.5));
    col += haze * (1.0 - clamp(ring + rib, 0.0, 1.0)) * smoothstep(0.0, 0.12, r);

    // vignette toward the outer edge
    col *= 1.0 - 0.40 * smoothstep(0.7, 1.5, r);

    // Star Wars credits crawl; geometry notes live in tunnel_shader.go
    float yh = sc.y - vp.y;
    if (crawl_lines > 0.5 && yh > 0.045) {
        float cw = crawl_width > 0.0 ? crawl_width : 0.9;
        float cz = crawl_zlines > 0.0 ? crawl_zlines : 3.0;
        float cp = crawl_persp > 0.0 ? crawl_persp : 0.45;
        float cs = crawl_speed > 0.0 ? crawl_speed : 0.8;

        float z = cp / yh;
        float g = clamp((1.0 - sc.y) / (1.0 - vp.y), 0.0, 1.0);
        float centerX = vp.x * g;
        centerX += 0.9 * g * (1.0 - g) * sin(bank * 2.0 - g * 2.5);
        float s = (sc.x - centerX) * z / cw + 0.5;
        float lf = (time * cs - z * cz) / crawl_lines;
        if (s >= 0.0 && s <= 1.0 && lf >= 0.0) {
            float t = frac(lf);
            float ta = crawl_tex.SampleLevel(crawl_smp, float2(s, t), 0).a;
            ta *= smoothstep(0.045, 0.18, yh) * (1.0 - smoothstep(1.0, 1.5, sc.y));
            float3 crawlCol = float3(1.0, 0.85, 0.2); // classic crawl yellow
            col = lerp(col, crawlCol, ta);
            col += crawlCol * ta * 0.25;              // soft neon glow
        }
    }

    // spinning, DVD-bouncing txlogger logo billboard, composited on top
    float2 lc = float2(tri(time * 0.27) * 0.58, tri(time * 0.21) * 0.52);

    // grow and shrink as it rides the tunnel toward and away from the viewer
    float depthBob = 0.70 + 0.35 * sin(time * 0.8);
    float lscale = (logo_scale > 0.0 ? logo_scale : 0.30) * depthBob;

    // rotation couples to the bounce path, flipping at every wall hit
    float ang = lc.x * 10.0 + lc.y * 7.0;
    float ca = cos(ang), sa = sin(ang);
    float2 q = (sc - lc) / lscale;
    float2 luv = float2(ca * q.x + sa * q.y, -sa * q.x + ca * q.y);
    if (abs(luv.x) <= 1.0 && abs(luv.y) <= 1.0) {
        float4 logo = logo_tex.SampleLevel(logo_smp, luv * 0.5 + 0.5, 0);
        // source-over with premultiplied alpha, then a pulsing neon rim glow
        col = col * (1.0 - logo.a) + logo.rgb;
        col += logo.a * (0.4 + 0.4 * sin(time * 2.0)) * 0.25 * float3(0.6, 0.25, 1.0);
    }

    return float4(col, 1.0);
}
