package datalogger

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Verifies the append-style formatting still produces the TXL line format:
// "ts|Name=value|...|IMPORTANTLINE=0|" with the first decimal '.' swapped for ','.
func TestTXWriterLineFormat(t *testing.T) {
	sv := NewThreadSafeMap()
	sv.Set("Rpm", 3000)     // whole number -> no decimals
	sv.Set("Lambda", 0.987) // decimal -> comma separator
	sv.Set("Lambda.External", 0.987)
	chans := []Channel{newSysvarChannel(sv, "Rpm"), newSysvarChannel(sv, "Lambda"), newSysvarChannel(sv, "Lambda.External")}

	f, err := os.CreateTemp(t.TempDir(), "*.t7l")
	if err != nil {
		t.Fatal(err)
	}
	w := NewTXLWriter(f)
	ts := time.Date(2026, 6, 25, 12, 30, 0, 0, time.UTC)
	if err := w.Write(ts, chans); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimRight(string(got), "\n")
	want := "25-06-2026 12:30:00|Rpm=3000|Lambda=0,99|Lambda.External=0,987|IMPORTANTLINE=0|"
	if line != want {
		t.Fatalf("got %q\nwant %q", line, want)
	}
}
