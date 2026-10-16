package dial

import _ "embed"

// GPU renderer: the dial geometry (face rim, pips, needle, highest-observed
// marker and center cap) is drawn by a single canvas.Shader, collapsing ~30
// canvas objects per dial into one draw call; SetValue only writes a float
// uniform. Text (title, value, pip labels) stays as canvas.Text layered on
// top - rasterizing glyphs in a fragment shader buys nothing.
//
// All dials share one Shader.Name: the painter caches the compiled program
// per name and the dial needs no textures, so every instance reuses the same
// program with its own uniforms.
//
// Conventions shared between the Go side and the GLSL in dial.frag:
//   - angles are radians, 0 pointing up, clockwise positive, matching the
//     needle direction (sin a, -cos a) of the CPU renderer
//   - the needle sweeps Pi15 (270 degrees) from -3/4 pi at Min to +3/4 pi
//     at Max; pip i sits at i*Pi15/steps - 3/4 pi
//   - the shader quad is the dial square padded by dialPad logical px per
//     side, because the marker pokes 4 px past the rim like the CPU line did
const dialPad = 4

const dialShaderPreludeGL = "#version 110\n"

const dialShaderPreludeES = `#version 100
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif
`

//go:embed dial.frag
var dialShaderBody string
