package windows

import (
	"fmt"
	"slices"
	"testing"
)

func TestPushRecent(t *testing.T) {
	var got []string
	for i := range maxRecentFiles + 5 {
		got = pushRecent(got, fmt.Sprintf("/f%d.bin", i))
	}
	if len(got) != maxRecentFiles {
		t.Fatalf("len = %d, want %d", len(got), maxRecentFiles)
	}
	if got[0] != "/f14.bin" || got[len(got)-1] != "/f5.bin" {
		t.Fatalf("window = %v, want newest first /f14.bin .. /f5.bin", got)
	}

	// Reopening an entry promotes it rather than duplicating it.
	got = pushRecent(got, "/f10.bin")
	if got[0] != "/f10.bin" {
		t.Fatalf("front = %q, want /f10.bin", got[0])
	}
	if n := len(slices.DeleteFunc(slices.Clone(got), func(s string) bool { return s != "/f10.bin" })); n != 1 {
		t.Fatalf("/f10.bin appears %d times, want 1", n)
	}
	if len(got) != maxRecentFiles {
		t.Fatalf("len = %d after promote, want %d", len(got), maxRecentFiles)
	}
}
