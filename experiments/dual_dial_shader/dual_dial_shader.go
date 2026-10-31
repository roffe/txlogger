package dualdial

import _ "embed"

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

//go:embed dual_dial.frag
var dualDialShaderBody string

//go:embed dual_dial.wgsl
var dualDialShaderWGSL []byte
