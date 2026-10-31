package windows

import (
	"bytes"
	"cmp"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/ecu"
)

// menuMaps lists the symbols the menu items open as maps.
func menuMaps(items []MenuItem) []string {
	var out []string
	for _, it := range items {
		switch {
		case len(it.Children) > 0:
			out = append(out, menuMaps(it.Children)...)
		case it.Func != nil:
		case it.Data != "":
			for _, n := range strings.Split(it.Data, "|") {
				out = append(out, strings.TrimSpace(n))
			}
		default:
			out = append(out, it.Name)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// brokenBins can't hold an edit in every menu map.
var brokenBins = map[string]string{
	"55565020_FC0J_C_FMEP_63_FIEF_82r.BIN": "relocated BFuelCal.TempEnrichFacMap runs into the checksum area",
}

// Every map the T5, T7 and T8 menus open is saved the way the map viewer
// saves it: one cell moved by a step, the rest as shown. The saved file must
// reload with exactly that change, and writing the cells one at a time to ECU
// RAM must lay them out like the file. Runs over the binaries under
// ecusymbol's testdata (ECUSYMBOL_TESTDATA), skips when there are none.
func TestMapSave(t *testing.T) {
	log.SetOutput(io.Discard) // ecusymbol logs every checksum update
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	root := cmp.Or(os.Getenv("ECUSYMBOL_TESTDATA"), "../../../ecusymbol/testdata")
	mw := &MainWindow{previewFeatures: true}
	for _, name := range []string{"T5", "T7", "T8"} {
		t.Run(name, func(t *testing.T) {
			bins, _ := filepath.Glob(filepath.Join(root, name+"Binaries", "*.[bB][iI][nN]"))
			if len(bins) == 0 {
				t.Skipf("no %s binaries under %s", name, root)
			}
			p := ecu.GetProfile(name)
			maps := menuMaps(ecuMenus[name](mw))
			for _, bin := range bins {
				t.Run(filepath.Base(bin), func(t *testing.T) {
					t.Parallel()
					if why := brokenBins[filepath.Base(bin)]; why != "" {
						t.Skip(why)
					}
					saveEveryMap(t, bin, p, maps)
				})
			}
		})
	}
}

func saveEveryMap(t *testing.T, bin string, p *ecu.Profile, maps []string) {
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data[max(len(data)-0x100, 0):], []byte("Tag:Nordic")) {
		t.Skip("Nordic tune, symbol table broken on purpose")
	}
	_, fw, err := symbol.Load(bin, data, func(string) {})
	if err != nil {
		t.Skip(err)
	}
	out := filepath.Join(t.TempDir(), filepath.Base(bin))
	saved := 0
	for _, name := range maps {
		// the symbol newMapViewer opens for the menu entry
		if alt := p.SymbolAliases[name]; fw.GetByName(name) == nil && alt != "" {
			name = alt
		}
		name = symbol.GetInfo(p.Type, name).Z
		sym := fw.GetByName(name)
		if sym == nil || sym.Length == 0 || len(sym.Bytes()) != int(sym.Length) {
			continue // not in this firmware, or not stored in flash
		}
		orig := sym.Bytes()
		vals := sym.Float64s()
		step := sym.Correctionfactor
		i := len(vals) / 2
		edited := slices.Clone(vals)
		edited[i] += step
		if back := sym.BytesToFloat64s(sym.EncodeFloat64s(edited)); math.Abs(back[i]-edited[i]) > step/2 {
			edited[i] = vals[i] - step // at the top of its range
		}

		if err := writeMap(fw, out, name, edited); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		saved++
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		_, re, err := symbol.Load(out, data, func(string) {})
		if err != nil {
			t.Fatalf("%s: reload: %v", name, err)
		}
		rs := re.GetByName(name)
		got := rs.Float64s()
		for j := range edited {
			if math.Abs(got[j]-edited[j]) > step/2 {
				t.Errorf("%s[%d]: saved %g, reloads %g", name, j, edited[j], got[j])
				break
			}
		}

		const base = 0x100000
		ram := make([]byte, len(orig))
		for j, v := range edited {
			addr, b := ramWrite(sym, base, j, []float64{v})
			copy(ram[addr-base:], b)
		}
		if !bytes.Equal(ram, rs.Bytes()) {
			t.Errorf("%s: cell by cell RAM writes % X, file has % X", name, ram, rs.Bytes())
		}
		_ = sym.SetData(orig)
	}
	if saved == 0 {
		t.Error("none of the menu maps is in this binary")
	}
}
