package wis

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
)

// useFakeArchive swaps the embedded archive loader for an in-memory one
// holding index and files until the test ends.
func useFakeArchive(t *testing.T, index map[string]map[string][]Entry, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	d := &db{zr: zr, index: index}
	old := load
	load = func() (*db, error) { return d, nil }
	t.Cleanup(func() { load = old })
}

// hit is the part of a Result the search tests compare.
type hit struct{ model, year, code, system, doc string }

func hits(res []Result) []hit {
	out := make([]hit, len(res))
	for i, r := range res {
		out[i] = hit{r.Model, r.Year, r.Code, r.System, r.Doc}
	}
	return out
}

func e(code, system, doc string) Entry {
	return Entry{Code: code, System: system, Doc: doc}
}

var fakeIndex = map[string]map[string][]Entry{
	"9440": {
		"2007": {e("B0165 02", "BCM", "9440/b2.htm"), e("B0165 05", "BCM", "9440/b5.htm"), e("P0100 00", "T8", "9440/p100new.htm")},
		"2006": {e("B0165 02", "BCM", "9440/b2.htm"), e("B0165 05", "BCM", "9440/b5.htm"), e("P0100 00", "T8", "9440/p100new.htm")},
		"2005": {e("B0165 02", "BCM", "9440/b2.htm"), e("P0100 00", "T8", "9440/p100old.htm")},
		"2004": {e("B0165 02", "BCM", "9440/b2old.htm"), e("P0100 00", "T8", "9440/p100old.htm")},
	},
	"9400": {
		"2002": {e("P0100", "Engine, Trionic 7", "9400/p100.htm"), e("P0105/P0106", "Engine, Trionic 7", "9400/map.htm")},
		"2001": {e("P0100", "Engine, Trionic 7", "9400/p100.htm"), e("P0100", "Engine, B308E", "9400/p100.htm")},
	},
	"9600": {
		"2003": {e("P0100", "Engine, Trionic 7", "9600/p100.htm")},
	},
}

// Search coalesces consecutive model years sharing a document into one
// "oldest-newest" result, keeps each revision separate, orders results
// newest first per model and models in the order asked for.
func TestSearchCoalescesYears(t *testing.T) {
	useFakeArchive(t, fakeIndex, nil)

	got := hits(Search("B0165 02", "9440"))
	want := []hit{
		{"9440", "2005-2007", "B0165 02", "BCM", "9440/b2.htm"},
		{"9440", "2004", "B0165 02", "BCM", "9440/b2old.htm"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("B0165 02:\n got %+v\nwant %+v", got, want)
	}

	got = hits(Search("P0100", "9600", "9440"))
	want = []hit{
		{"9600", "2003", "P0100", "Engine, Trionic 7", "9600/p100.htm"},
		{"9440", "2006-2007", "P0100 00", "T8", "9440/p100new.htm"},
		{"9440", "2004-2005", "P0100 00", "T8", "9440/p100old.htm"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("P0100 in 9600,9440:\n got %+v\nwant %+v", got, want)
	}
}

// The same document listed under two systems gives one result per system.
func TestSearchSeparatesSystems(t *testing.T) {
	useFakeArchive(t, fakeIndex, nil)
	got := hits(Search("P0100", "9400"))
	want := []hit{
		{"9400", "2001-2002", "P0100", "Engine, Trionic 7", "9400/p100.htm"},
		{"9400", "2001", "P0100", "Engine, B308E", "9400/p100.htm"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// A full "code + failure mode" query returns only that variant in the years
// that list it; a bare code, or a failure mode a year has no document for,
// returns every variant of the base code for that year. The query is
// trimmed and upper-cased first.
func TestSearchFailureModeMatching(t *testing.T) {
	useFakeArchive(t, fakeIndex, nil)
	codes := func(q string) []string {
		var out []string
		for _, r := range Search(q, "9440") {
			out = append(out, r.Code+"@"+r.Year)
		}
		return out
	}
	tests := []struct {
		query string
		want  []string
	}{
		{"B0165 02", []string{"B0165 02@2005-2007", "B0165 02@2004"}},
		// 2005 and 2004 list no B0165 05 document, so those years fall
		// back to the variants they do list
		{"B0165 05", []string{"B0165 05@2006-2007", "B0165 02@2005", "B0165 02@2004"}},
		{"B0165", []string{"B0165 02@2005-2007", "B0165 05@2006-2007", "B0165 02@2004"}},
		{"B0165 0F", []string{"B0165 02@2005-2007", "B0165 05@2006-2007", "B0165 02@2004"}},
		{"  b0165 02\t", []string{"B0165 02@2005-2007", "B0165 02@2004"}},
		{"B016", nil},
		{"P0101", nil},
	}
	for _, tt := range tests {
		if got := codes(tt.query); !slices.Equal(got, tt.want) {
			t.Errorf("Search(%q) = %q, want %q", tt.query, got, tt.want)
		}
	}
}

// Index entries carrying several codes joined with "/" match a query for
// any one of them; the result keeps the joined code.
func TestSearchSlashJoinedCodes(t *testing.T) {
	useFakeArchive(t, fakeIndex, nil)
	for _, q := range []string{"P0105", "P0106", "p0106 "} {
		got := hits(Search(q, "9400"))
		want := []hit{{"9400", "2002", "P0105/P0106", "Engine, Trionic 7", "9400/map.htm"}}
		if !slices.Equal(got, want) {
			t.Errorf("Search(%q) = %+v, want %+v", q, got, want)
		}
	}
}

// With no models given Search covers 9400, 9440 and 9600 in that order; an
// empty query or an unknown model finds nothing.
func TestSearchModelsAndEmptyQuery(t *testing.T) {
	useFakeArchive(t, fakeIndex, nil)
	var models []string
	for _, r := range Search("P0100") {
		if len(models) == 0 || models[len(models)-1] != r.Model {
			models = append(models, r.Model)
		}
	}
	if want := []string{"9400", "9440", "9600"}; !slices.Equal(models, want) {
		t.Errorf("default models = %q, want %q", models, want)
	}
	for _, q := range []string{"", "   "} {
		if res := Search(q); res != nil {
			t.Errorf("Search(%q) = %+v, want nil", q, res)
		}
	}
	if res := Search("P0100", "9999"); len(res) != 0 {
		t.Errorf("unknown model: got %+v, want none", res)
	}
}

// A year range claims every model year in it: a document missing from a
// year in between must split into separate results rather than one range
// spanning the gap.
func TestSearchYearRangeSkipsGaps(t *testing.T) {
	useFakeArchive(t, map[string]map[string][]Entry{
		"9440": {
			"2006": {e("P0411 00", "T8", "9440/air.htm")},
			"2005": {e("P0411 00", "T8", "9440/air.htm")},
			"2004": {e("P0420 00", "T8", "9440/cat.htm")},
			"2003": {e("P0411 00", "T8", "9440/air.htm")},
		},
	}, nil)
	var years []string
	for _, r := range Search("P0411 00", "9440") {
		years = append(years, r.Year)
	}
	if want := []string{"2005-2006", "2003"}; !slices.Equal(years, want) {
		t.Errorf("years = %q, want %q", years, want)
	}
}

// Years lists a model's years newest first; List returns one year's
// entries as stored; both are empty for unknown models or years.
func TestYearsAndListFake(t *testing.T) {
	useFakeArchive(t, fakeIndex, nil)
	if got, want := Years("9440"), []string{"2007", "2006", "2005", "2004"}; !slices.Equal(got, want) {
		t.Errorf("Years(9440) = %q, want %q", got, want)
	}
	if got := Years("9999"); len(got) != 0 {
		t.Errorf("Years(9999) = %q, want none", got)
	}
	want := []Entry{
		e("P0100", "Engine, Trionic 7", "9400/p100.htm"),
		e("P0100", "Engine, B308E", "9400/p100.htm"),
	}
	if got := List("9400", "2001"); !slices.Equal(got, want) {
		t.Errorf("List(9400, 2001) = %+v, want %+v", got, want)
	}
	if got := List("9400", "1990"); got != nil {
		t.Errorf("List(9400, 1990) = %+v, want nil", got)
	}
	if got := List("9999", "2001"); got != nil {
		t.Errorf("List(9999, 2001) = %+v, want nil", got)
	}
}

// Render inlines each referenced SVG from the document's own model folder,
// stripping its XML declaration and "stdout" title and replacing the
// diagram id label; unknown images become an HTML comment and the zoom
// footer is appended.
func TestRenderInlinesImages(t *testing.T) {
	const svg = `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<svg><title>stdout</title><text>D124F001</text><path d="M0 0"/></svg>`
	useFakeArchive(t, nil, map[string]string{
		"9600/doc1.htm":          `<h2>P0106</h2><p>see</p><wisimg id="D124F001"/><p>and</p><wisimg id="GONE1"/>`,
		"9600/img/D124F001.svg":  svg,
		"9400/img/GONE1.svg":     `<svg>other model</svg>`,
		"9600/img/UNUSED001.svg": `<svg>unused</svg>`,
	})
	out, err := Render("9600/doc1.htm")
	if err != nil {
		t.Fatal(err)
	}
	want := `<h2>P0106</h2><p>see</p>` + "\n" +
		`<svg><text>txlogger</text><path d="M0 0"/></svg>` +
		`<p>and</p><!-- missing image GONE1 -->` + footer
	if string(out) != want {
		t.Errorf("Render =\n%s\nwant\n%s", out, want)
	}
}

// Render of a document without placeholders returns it unchanged plus the
// zoom footer.
func TestRenderPlainDocument(t *testing.T) {
	useFakeArchive(t, nil, map[string]string{"9440/docida1.htm": "<p>no images</p>"})
	out, err := Render("9440/docida1.htm")
	if err != nil {
		t.Fatal(err)
	}
	if want := "<p>no images</p>" + footer; string(out) != want {
		t.Errorf("Render = %q, want %q", out, want)
	}
}

// Render of a document that is not in the archive fails with a "wis:"
// error wrapping fs.ErrNotExist.
func TestRenderMissingDocument(t *testing.T) {
	useFakeArchive(t, nil, map[string]string{"9440/docida1.htm": "x"})
	out, err := Render("9440/docida2.htm")
	if out != nil || err == nil {
		t.Fatalf("Render = %q, %v; want nil, error", out, err)
	}
	if !errors.Is(err, fs.ErrNotExist) || !strings.HasPrefix(err.Error(), "wis: ") {
		t.Errorf("err = %v, want wis: ... wrapping fs.ErrNotExist", err)
	}
}

// When the archive cannot be loaded every lookup comes back empty and
// Render returns the load error.
func TestLoadFailure(t *testing.T) {
	errBroken := errors.New("broken archive")
	old := load
	load = func() (*db, error) { return nil, errBroken }
	t.Cleanup(func() { load = old })

	if res := Search("P0100"); res != nil {
		t.Errorf("Search = %+v, want nil", res)
	}
	if y := Years("9440"); y != nil {
		t.Errorf("Years = %q, want nil", y)
	}
	if l := List("9440", "2007"); l != nil {
		t.Errorf("List = %+v, want nil", l)
	}
	if out, err := Render("9440/docida1.htm"); out != nil || !errors.Is(err, errBroken) {
		t.Errorf("Render = %q, %v; want nil, %v", out, err, errBroken)
	}
}

// ModelName maps the WIS model codes to their display names and passes
// anything else through unchanged.
func TestModelName(t *testing.T) {
	tests := map[string]string{
		"9400": "9-3 (9400)",
		"9440": "9-3 (9440)",
		"9600": "9-5 (9600)",
		"9000": "9000",
		"":     "",
	}
	for in, want := range tests {
		if got := ModelName(in); got != want {
			t.Errorf("ModelName(%q) = %q, want %q", in, got, want)
		}
	}
}
