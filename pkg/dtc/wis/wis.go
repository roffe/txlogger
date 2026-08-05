// Package wis serves SAAB WIS (Workshop Information System) DTC fault
// diagnosis documents for 9-3 (9400), 9-3 (9440) and 9-5 (9600) from an
// embedded archive built by gen.go from the raw WIS dumps.
package wis

//go:generate go run gen.go

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed wis.zip
var archive []byte

// Entry is one DTC in a model/year fault diagnosis index.
type Entry struct {
	Code    string `json:"c"`           // e.g. "P0100" or "B0165 02"
	System  string `json:"s"`           // ECU/system name, e.g. "Engine, Trionic 7"
	Title   string `json:"t,omitempty"` // document heading
	Symptom string `json:"y,omitempty"`
	Doc     string `json:"d"` // document path for Render
}

// Result is a Search hit. Year is a model year or a range of consecutive
// model years sharing the same document revision, e.g. "2003" or "2007-2011".
type Result struct {
	Model string // "9400", "9440" or "9600"
	Year  string
	Entry
}

var modelNames = map[string]string{
	"9400": "9-3 (9400)",
	"9440": "9-3 (9440)",
	"9600": "9-5 (9600)",
}

// ModelName returns the display name for a model code.
func ModelName(model string) string {
	if n, ok := modelNames[model]; ok {
		return n
	}
	return model
}

var load = sync.OnceValues(func() (*db, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	f, err := zr.Open("index.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d := &db{zr: zr}
	if err := json.NewDecoder(f).Decode(&d.index); err != nil {
		return nil, err
	}
	return d, nil
})

type db struct {
	zr    *zip.Reader
	index map[string]map[string][]Entry // model -> year -> entries
}

func (d *db) readFile(name string) ([]byte, error) {
	f, err := d.zr.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// Search finds the documents for a DTC code in the given models (all models
// if none given), across all model years — different years can carry
// different document revisions with different wiring diagrams. Years sharing
// the same document are coalesced into one Result; results are ordered
// newest first per model. A bare code like "B0165" matches all failure mode
// variants ("B0165 02", "B0165 05").
func Search(code string, models ...string) []Result {
	d, err := load()
	if err != nil {
		return nil
	}
	if len(models) == 0 {
		models = []string{"9400", "9440", "9600"}
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil
	}
	base, _, _ := strings.Cut(code, " ")

	var out []Result
	for _, m := range models {
		years := make([]string, 0, len(d.index[m]))
		for y := range d.index[m] {
			years = append(years, y)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(years)))

		type key struct{ doc, code, system string }
		var order []key
		newest := map[key]*Result{}
		oldest := map[key]string{}
		for _, y := range years {
			var exact, loose []Entry
			for _, e := range d.index[m][y] {
				// entries from the 9400/9600 index can carry several codes
				// joined with "/"
				for _, c := range strings.Split(strings.ToUpper(e.Code), "/") {
					c = strings.TrimSpace(c)
					cbase, _, _ := strings.Cut(c, " ")
					if c == code {
						exact = append(exact, e)
						break
					}
					if cbase == base {
						loose = append(loose, e)
						break
					}
				}
			}
			// a full "code + failure mode" query returns only its own doc;
			// fall back to all failure mode variants of the base code
			hits := exact
			if len(hits) == 0 {
				hits = loose
			}
			for _, e := range hits {
				k := key{e.Doc, e.Code, e.System}
				if _, ok := newest[k]; !ok {
					order = append(order, k)
					newest[k] = &Result{Model: m, Year: y, Entry: e}
				}
				oldest[k] = y
			}
		}
		for _, k := range order {
			r := *newest[k]
			if oldest[k] != r.Year {
				r.Year = oldest[k] + "-" + r.Year
			}
			out = append(out, r)
		}
	}
	return out
}

// Years lists the model years available for a model, newest first.
func Years(model string) []string {
	d, err := load()
	if err != nil {
		return nil
	}
	var years []string
	for y := range d.index[model] {
		years = append(years, y)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(years)))
	return years
}

// List returns the DTC index for a model and year.
func List(model, year string) []Entry {
	d, err := load()
	if err != nil {
		return nil
	}
	return d.index[model][year]
}

var (
	rePlaceholder = regexp.MustCompile(`<wisimg id="([0-9a-zA-Z]+)"/>`)
	reXMLDecl     = regexp.MustCompile(`<\?xml[^>]*\?>`)
)

const footer = `<style>
svg { cursor: zoom-in; margin: 5px; display: block; border: 1px solid #000; max-width: 95%; height: auto; }
svg.zoomed { cursor: zoom-out; zoom: 3.0; max-width: none; }
</style>
<script>
document.querySelectorAll('svg').forEach(function (s) {
	s.addEventListener('click', function () { s.classList.toggle('zoomed'); });
});
</script>`

// Render returns a document from the archive as self-contained HTML with the
// referenced wiring diagram SVGs inlined (click to zoom).
func Render(doc string) ([]byte, error) {
	d, err := load()
	if err != nil {
		return nil, err
	}
	c, err := d.readFile(doc)
	if err != nil {
		return nil, fmt.Errorf("wis: %w", err)
	}
	model := path.Dir(doc)
	c = rePlaceholder.ReplaceAllFunc(c, func(m []byte) []byte {
		id := string(rePlaceholder.FindSubmatch(m)[1])
		svg, err := d.readFile(model + "/img/" + id + ".svg")
		if err != nil {
			return []byte("<!-- missing image " + id + " -->")
		}
		svg = reXMLDecl.ReplaceAll(svg, nil)
		svg = bytes.Replace(svg, []byte("<title>stdout</title>"), nil, 1)
		// the diagram id is drawn as a text label in the SVG; hide it
		svg = bytes.ReplaceAll(svg, []byte(id), []byte("txlogger"))
		return svg
	})
	return append(c, footer...), nil
}
