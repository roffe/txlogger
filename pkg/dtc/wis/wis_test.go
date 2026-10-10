package wis

import (
	"bytes"
	"io/fs"
	"slices"
	"strings"
	"testing"
)

func TestSearchAndRender(t *testing.T) {
	// T7 engine code, present in both 9400 and 9600
	res := Search("P0100", "9400", "9600")
	if len(res) < 2 {
		t.Fatalf("P0100: got %d results, want >= 2: %+v", len(res), res)
	}
	for _, r := range res {
		if r.Title == "" {
			t.Errorf("%s %s: empty title", r.Model, r.Year)
		}
		html, err := Render(r.Doc)
		if err != nil {
			t.Fatalf("Render(%s): %v", r.Doc, err)
		}
		if bytes.Contains(html, []byte("<wisimg")) || bytes.Contains(html, []byte("<script LANGUAGE")) {
			t.Errorf("%s: unresolved placeholder or leftover script", r.Doc)
		}
	}

	// a doc known to reference wiring diagrams renders them inline
	if res := Search("P0106", "9600"); len(res) == 0 {
		t.Error("P0106: no results")
	} else if html, err := Render(res[0].Doc); err != nil {
		t.Fatal(err)
	} else if !bytes.Contains(html, []byte("<svg")) {
		t.Errorf("%s: no inlined SVG", res[0].Doc)
	}

	// T8 body code: a bare code from the ECU matches the stored
	// "code + failure mode" form; years sharing a doc revision coalesce
	// (B0165 02 has three revisions: 2003, 2004-2006, 2007-2011)
	res = Search("B0165", "9440")
	if len(res) != 3 {
		t.Fatalf("B0165: got %d results, want 3: %+v", len(res), res)
	}
	for _, r := range res {
		if r.Code != "B0165 02" {
			t.Errorf("B0165: matched %q", r.Code)
		}
	}
	if res[0].Year != "2007-2011" || res[2].Year != "2003" {
		t.Errorf("B0165: year ranges %q..%q, want 2007-2011..2003", res[0].Year, res[2].Year)
	}
	if res := Search("B0165 02", "9440"); len(res) != 3 {
		t.Errorf("B0165 02: got %d results, want 3", len(res))
	}

	// a full "code + failure mode" query must not drag in other variants
	exact := Search("B1000 08", "9440")
	if len(exact) == 0 {
		t.Fatal("B1000 08: no results")
	}
	for _, r := range exact {
		if r.Code != "B1000 08" {
			t.Errorf("B1000 08: matched variant %q", r.Code)
		}
	}
	if bare := Search("B1000", "9440"); len(bare) <= len(exact) {
		t.Errorf("bare B1000: got %d results, want more than %d", len(bare), len(exact))
	}
	if res := Search("P9999"); len(res) != 0 {
		t.Errorf("P9999: got %d results, want 0", len(res))
	}
}

// Years lists every model year the embedded archive holds, newest first.
func TestYearsArchive(t *testing.T) {
	tests := map[string][]string{
		"9400": {"2003", "2002", "2001", "2000", "1999", "1998"},
		"9440": {"2011", "2010", "2009", "2008", "2007", "2006", "2005", "2004", "2003"},
		"9600": {"2010", "2009", "2008", "2007", "2006", "2005", "2004", "2003", "2002", "2001", "2000", "1999", "1998"},
	}
	for model, want := range tests {
		if got := Years(model); !slices.Equal(got, want) {
			t.Errorf("Years(%s) = %q, want %q", model, got, want)
		}
	}
	if got := Years("9000"); len(got) != 0 {
		t.Errorf("Years(9000) = %q, want none (T5 cars are not in the archive)", got)
	}
}

// Every indexed entry names a code, a system and a document of its own
// model that exists in the archive, so Render can open whatever Search or
// List returns.
func TestIndexDocumentsExist(t *testing.T) {
	d, err := load()
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"9400", "9440", "9600"} {
		seen := map[string]bool{}
		for _, year := range Years(model) {
			entries := List(model, year)
			if len(entries) == 0 {
				t.Errorf("List(%s, %s): no entries", model, year)
			}
			for _, e := range entries {
				if e.Code == "" || e.System == "" {
					t.Errorf("%s %s: incomplete entry %+v", model, year, e)
				}
				if !strings.HasPrefix(e.Doc, model+"/") {
					t.Errorf("%s %s %s: doc %q outside the model folder", model, year, e.Code, e.Doc)
				}
				if seen[e.Doc] {
					continue
				}
				seen[e.Doc] = true
				if _, err := fs.Stat(d.zr, e.Doc); err != nil {
					t.Errorf("%s %s %s: %v", model, year, e.Code, err)
				}
			}
		}
	}
}

// Rendered archive documents have their diagrams inlined without XML
// declarations or "stdout" titles, and a diagram the archive lacks leaves
// a comment instead of a broken placeholder.
func TestRenderArchiveImages(t *testing.T) {
	res := Search("P0106", "9600")
	if len(res) == 0 {
		t.Fatal("P0106: no results")
	}
	html, err := Render(res[0].Doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<?xml", "<title>stdout</title>", "<wisimg"} {
		if bytes.Contains(html, []byte(bad)) {
			t.Errorf("%s: output contains %q", res[0].Doc, bad)
		}
	}
	if !bytes.HasSuffix(html, []byte(footer)) {
		t.Errorf("%s: missing zoom footer", res[0].Doc)
	}

	// docida607 references img2474, which the WIS image dump does not have
	html, err = Render("9440/docida607.htm")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte("<!-- missing image img2474 -->")) {
		t.Error("9440/docida607.htm: no missing image comment for img2474")
	}
}

// 9400/9600 index entries list several codes separated by commas with
// irregular spacing ("P0122, P0123", "P0120,P0121,P0122, P0123"); a search
// for any listed code must find the entry.
func TestSearchCommaListedCodes(t *testing.T) {
	tests := []struct {
		code, model, doc string
	}{
		{"P0122", "9400", "9400/doc1273.htm"},  // "P0122, P0123", Motronic 2.10.3
		{"P0123", "9400", "9400/doc1273.htm"},  // second code of the same entry
		{"P0122", "9400", "9400/doc10947.htm"}, // "P0120,P0121,P0122, P0123", Trionic T5
		{"B1227", "9400", "9400/doc15489.htm"}, // "B1227, B1228", SRS
		{"B1228", "9400", "9400/doc15489.htm"},
	}
	for _, tt := range tests {
		found := false
		for _, r := range Search(tt.code, tt.model) {
			if r.Doc == tt.doc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Search(%s, %s): %s not found", tt.code, tt.model, tt.doc)
		}
	}
}
