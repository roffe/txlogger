package windows

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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

func (mw *MainWindow) showSymbolCompare(typ symbol.ECUType, otherName string, other symbol.FirmwareFile) {
	var diffs []symbolcompare.Item
	for _, s := range mw.fw.Symbols() {
		o := other.GetByName(s.Name)
		if o == nil {
			continue // ponytail: only symbols present in both; added/removed skipped
		}
		if !bytes.Equal(s.Bytes(), o.Bytes()) {
			diffs = append(diffs, symbolcompare.Item{
				Name:        s.Name,
				Number:      s.Number,
				Length:      s.Length,
				OtherLength: o.Length,
				Address:     s.Address,
			})
		}
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].Name < diffs[j].Name })

	if len(diffs) == 0 {
		dialog.ShowInformation("No differences", "The two binaries have identical symbol data", mw)
		return
	}

	mapTabs := container.NewDocTabs()
	previewTab := container.NewTabItem("Preview", widget.NewLabel("Click a symbol to preview it"))
	mapTabs.Append(previewTab)
	cmp := symbolcompare.New(&symbolcompare.Config{
		Diffs: diffs,
		OnPreview: func(name string) {
			for _, ti := range mapTabs.Items {
				if ti.Text == name {
					mapTabs.Select(ti)
					return
				}
			}
			content, err := mw.compareTabContent(typ, otherName, other, name)
			if err != nil {
				mw.Error(err)
				return
			}
			previewTab.Text = "Preview: " + name
			previewTab.Content = content
			if !slices.Contains(mapTabs.Items, previewTab) {
				// closed or promoted away; bring it back as the first tab
				mapTabs.Items = append([]*container.TabItem{previewTab}, mapTabs.Items...)
			}
			mapTabs.Select(previewTab)
			mapTabs.Refresh()
		},
		OnOpen: func(name string) {
			for _, ti := range mapTabs.Items {
				if ti.Text == name {
					mapTabs.Select(ti)
					return
				}
			}
			// the first click of the double-click already built the preview:
			// promote it to a named tab instead of rebuilding
			if previewTab.Text == "Preview: "+name && slices.Contains(mapTabs.Items, previewTab) {
				promoted := previewTab
				promoted.Text = name
				previewTab = container.NewTabItem("Preview", widget.NewLabel("Click a symbol to preview it"))
				mapTabs.Refresh()
				mapTabs.Select(promoted)
				return
			}
			content, err := mw.compareTabContent(typ, otherName, other, name)
			if err != nil {
				mw.Error(err)
				return
			}
			ti := container.NewTabItem(name, content)
			mapTabs.Append(ti)
			mapTabs.Select(ti)
		},
	})
	split := container.NewHSplit(cmp, mapTabs)
	split.Offset = 0.3
	inner := multiwindow.NewInnerWindow("Compare with "+otherName, split)
	inner.Icon = theme.SearchReplaceIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(1200, 700))
}

// compareTabContent builds the current and other binary's version of a map
// plus a per-cell diff as three tabs. ponytail: native AppTabs, no custom
// wrapper.
func (mw *MainWindow) compareTabContent(typ symbol.ECUType, otherName string, other symbol.FirmwareFile, mapName string) (fyne.CanvasObject, error) {
	_, _, _, othZ, _, _, _, err := compareMapData(other, typ, mapName)
	if err != nil {
		return nil, err
	}
	return mw.mapDiffTabs(typ, mapName, "Current", otherName, othZ)
}

// mapDiffTabs renders mapName from the loaded binary next to othZ (another
// binary's values, or a proposal from the AI chat) plus a per-cell delta.
func (mw *MainWindow) mapDiffTabs(typ symbol.ECUType, mapName, curLabel, otherLabel string, othZ []float64) (fyne.CanvasObject, error) {
	axis, xData, yData, curZ, xPrec, yPrec, zPrec, err := compareMapData(mw.fw, typ, mapName)
	if err != nil {
		return nil, err
	}
	if len(curZ) != len(othZ) {
		return nil, fmt.Errorf("%s: %d values expected, got %d", mapName, len(curZ), len(othZ))
	}
	cur, err := mw.readonlyMapViewer(mapName, axis.ZDescription, axis, xData, yData, curZ, xPrec, yPrec, zPrec)
	if err != nil {
		return nil, err
	}
	oth, err := mw.readonlyMapViewer(mapName, axis.ZDescription, axis, xData, yData, othZ, xPrec, yPrec, zPrec)
	if err != nil {
		return nil, err
	}
	diff := make([]float64, len(curZ))
	for i := range curZ {
		diff[i] = curZ[i] - othZ[i]
	}
	diffMv, err := mw.readonlyMapViewer(mapName, "Δ "+axis.ZDescription, axis, xData, yData, diff, xPrec, yPrec, zPrec)
	if err != nil {
		return nil, err
	}
	return container.NewAppTabs(
		container.NewTabItem("Diff", diffMv),
		container.NewTabItem(curLabel, cur),
		container.NewTabItem(otherLabel, oth),
	), nil
}

// showAIMapDiff opens the AI chat's proposed values for a map next to the
// current ones. Called from the chat's tool loop, so it must run on the main
// thread — aichat wraps it in fyne.DoAndWait.
func (mw *MainWindow) showAIMapDiff(mapName string, proposed []float64) error {
	if mw.fw == nil {
		return errors.New("no binary loaded")
	}
	typ := symbol.ECUTypeFromString(mw.selects.ecuSelect.Selected)
	content, err := mw.mapDiffTabs(typ, mapName, "Before", "After", proposed)
	if err != nil {
		return err
	}
	title := "AI suggestion: " + mapName
	if w := mw.wm.HasWindow(title); w != nil {
		w.Close() // replace the previous suggestion for this map
	}
	inner := multiwindow.NewInnerWindow(title, content)
	inner.Icon = theme.SearchReplaceIcon()
	mw.wm.Add(inner)
	inner.Resize(fyne.NewSize(900, 600))
	return nil
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
func compareMapData(coll symbol.FirmwareFile, typ symbol.ECUType, mapName string) (axis symbol.Axis, xData, yData, zData []float64, xPrec, yPrec, zPrec int, err error) {
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
