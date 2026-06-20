package windows

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/symbolcompare"
)

// openSymbolCompare lets the user pick a second binary and lists every symbol
// whose data differs from the currently loaded one.
func (mw *MainWindow) openSymbolCompare() {
	if mw.fw == nil {
		mw.Error(fmt.Errorf("no binary loaded"))
		return
	}
	if mw.dlc != nil {
		mw.Error(errors.New("stop logging before comparing binaries"))
		return
	}
	widgets.SelectFile(func(r fyne.URIReadCloser) {
		defer r.Close()
		data, err := io.ReadAll(r)
		if err != nil {
			mw.Error(err)
			return
		}
		filename := filepath.Base(r.URI().Path())
		otherEcu, other, err := symbol.Load(filename, data, mw.Log)
		if err != nil {
			mw.Error(fmt.Errorf("failed to load %s: %w", filename, err))
			return
		}
		typ := symbol.ECUTypeFromString(mw.selects.ecuSelect.Selected)
		if otherEcu != typ {
			mw.Error(fmt.Errorf("ECU type mismatch: current is %s, %s is %s", typ, filename, otherEcu))
			return
		}
		mw.showSymbolCompare(typ, filename, other)
	}, "Binary file", "bin")
}

func (mw *MainWindow) showSymbolCompare(typ symbol.ECUType, otherName string, other symbol.SymbolCollection) {
	var diffs []string
	for _, s := range mw.fw.Symbols() {
		o := other.GetByName(s.Name)
		if o == nil {
			continue // ponytail: only symbols present in both; added/removed skipped
		}
		if !bytes.Equal(s.Bytes(), o.Bytes()) {
			diffs = append(diffs, s.Name)
		}
	}
	sort.Strings(diffs)

	if len(diffs) == 0 {
		dialog.ShowInformation("No differences", "The two binaries have identical symbol data", mw)
		return
	}

	cmp := symbolcompare.New(&symbolcompare.Config{
		Diffs:            diffs,
		OnShowDiff:       func(name string) { mw.openCompareDiff(typ, otherName, other, name) },
		OnShowSideBySide: func(name string) { mw.openCompareTabs(typ, otherName, other, name) },
	})
	inner := multiwindow.NewInnerWindow("Compare with "+otherName, cmp)
	inner.Icon = theme.SearchReplaceIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(400, 600))
}

// openCompareTabs shows the current and other binary's version of a map in two
// tabs. ponytail: native AppTabs, no custom multi-map wrapper widget.
func (mw *MainWindow) openCompareTabs(typ symbol.ECUType, otherName string, other symbol.SymbolCollection, mapName string) {
	winName := mapName + " - compare"
	if w := mw.wm.HasWindow(winName); w != nil {
		mw.wm.Raise(w)
		return
	}
	cur, err := mw.compareMapViewer(mw.fw, typ, mapName)
	if err != nil {
		mw.Error(err)
		return
	}
	oth, err := mw.compareMapViewer(other, typ, mapName)
	if err != nil {
		mw.Error(err)
		return
	}
	tabs := container.NewAppTabs(
		container.NewTabItem("Current", cur),
		container.NewTabItem(otherName, oth),
	)
	inner := multiwindow.NewInnerWindow(winName, tabs)
	inner.Icon = theme.GridIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(900, 700))
}

// openCompareDiff shows a single read-only map of (current - other) per cell.
func (mw *MainWindow) openCompareDiff(typ symbol.ECUType, otherName string, other symbol.SymbolCollection, mapName string) {
	winName := mapName + " - diff"
	if w := mw.wm.HasWindow(winName); w != nil {
		mw.wm.Raise(w)
		return
	}
	axis, xData, yData, curZ, xPrec, yPrec, zPrec, err := compareMapData(mw.fw, typ, mapName)
	if err != nil {
		mw.Error(err)
		return
	}
	_, _, _, othZ, _, _, _, err := compareMapData(other, typ, mapName)
	if err != nil {
		mw.Error(err)
		return
	}
	if len(curZ) != len(othZ) {
		mw.Error(fmt.Errorf("%s has different dimensions in the two binaries (%d vs %d)", mapName, len(curZ), len(othZ)))
		return
	}
	diff := make([]float64, len(curZ))
	for i := range curZ {
		diff[i] = curZ[i] - othZ[i]
	}
	mv, err := mw.readonlyMapViewer(mapName, "Δ "+axis.ZDescription, axis, xData, yData, diff, xPrec, yPrec, zPrec)
	if err != nil {
		mw.Error(err)
		return
	}
	inner := multiwindow.NewInnerWindow(winName+" (current - "+otherName+")", mv)
	inner.Icon = theme.GridIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(800, 600))
}

// compareMapViewer builds a read-only MapViewer for mapName from an arbitrary
// collection. Unlike newMapViewer it has no file/ECU load+save wiring.
func (mw *MainWindow) compareMapViewer(coll symbol.SymbolCollection, typ symbol.ECUType, mapName string) (*mapviewer.MapViewer, error) {
	axis, x, y, z, xp, yp, zp, err := compareMapData(coll, typ, mapName)
	if err != nil {
		return nil, err
	}
	return mw.readonlyMapViewer(mapName, axis.ZDescription, axis, x, y, z, xp, yp, zp)
}

func (mw *MainWindow) readonlyMapViewer(name, zLabel string, axis symbol.Axis, xData, yData, zData []float64, xPrec, yPrec, zPrec int) (*mapviewer.MapViewer, error) {
	return mapviewer.New(&mapviewer.Config{
		Name:           name,
		XData:          xData,
		YData:          yData,
		ZData:          zData,
		XPrecision:     xPrec,
		YPrecision:     yPrec,
		ZPrecision:     zPrec,
		XLabel:         axis.XDescription,
		YLabel:         axis.YDescription,
		ZLabel:         zLabel,
		MeshView:       mw.settings.GetMeshView(),
		MeshRenderer:   mw.settings.GetMeshRenderer(),
		Editable:       false,
		ColorblindMode: mw.settings.GetColorBlindMode(),
	})
}

// compareMapData resolves a map's axes + data from one collection. A trimmed
// version of newMapViewer's resolution: no as2, no T5 coolant special-case.
func compareMapData(coll symbol.SymbolCollection, typ symbol.ECUType, mapName string) (axis symbol.Axis, xData, yData, zData []float64, xPrec, yPrec, zPrec int, err error) {
	axis = symbol.GetInfo(typ, mapName)

	symX := coll.GetByName(axis.X)
	if symX == nil && axis.X == "BstKnkCal.fi_offsetXSP" {
		symX = coll.GetByName("BstKnkCal.OffsetXSP")
	}
	symY := coll.GetByName(axis.Y)
	symZ := coll.GetByName(axis.Z)
	if symZ == nil {
		err = fmt.Errorf("symbol %s not found", axis.Z)
		return
	}

	zData = symZ.Float64s()
	zPrec = symbol.GetPrecision(symZ.Correctionfactor)

	if symX != nil {
		xData = symX.Float64s()
		xPrec = symbol.GetPrecision(symX.Correctionfactor)
	} else {
		xData = []float64{0}
	}

	switch {
	case symY != nil:
		yData = symY.Float64s()
		yPrec = symbol.GetPrecision(symY.Correctionfactor)
	case len(xData) <= 1 && len(zData) > 1:
		// 1xN column with no Y axis: index it so the viewer can lay it out
		yData = make([]float64, len(zData))
		for i := range yData {
			yData[i] = float64(i)
		}
	default:
		yData = []float64{0}
	}
	return
}
