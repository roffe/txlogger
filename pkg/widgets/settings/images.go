package settings

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/roffe/txlogger/pkg/assets"
)

// imageSpec describes a bundled wideband product image and its display size.
type imageSpec struct {
	content []byte
	w, h    float32
}

var imageSpecs = map[string]imageSpec{
	"mtx-l":       {assets.MtxL, 224, 224},
	"lc-2":        {assets.Lc2, 400, 224},
	"uego":        {assets.Uego, 315, 224},
	"lambdatocan": {assets.LambdaToCan, 481, 224},
	"t7":          {assets.T7, 320, 224},
	"plx":         {assets.PLX, 470, 224},
	"combi":       {assets.CombiV2, 360, 245},
	"zeitronix":   {assets.ZeitronixZT2, 252, 252},
	"stagafr":     {assets.STAGAfr, 252, 252},
}

// newImageFromResource builds a contained, fast-scaling image for the named
// product. Returns nil if the name is unknown.
func newImageFromResource(name string) *canvas.Image {
	spec, ok := imageSpecs[name]
	if !ok {
		return nil
	}
	img := canvas.NewImageFromResource(fyne.NewStaticResource(name, spec.content))
	img.SetMinSize(fyne.NewSize(spec.w, spec.h))
	img.FillMode = canvas.ImageFillContain
	img.ScaleMode = canvas.ImageScaleFastest
	return img
}
