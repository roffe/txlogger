package wis

import (
	"bytes"
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
