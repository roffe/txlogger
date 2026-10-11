package seedkey

import (
	"bytes"
	"cmp"
	"os"
	"path/filepath"
	"testing"

	symbol "github.com/roffe/ecusymbol"
)

// On every T7 binary under ecusymbol's testdata (ECUSYMBOL_TESTDATA): a
// patched seed/key routine saves as a file that loads with valid checksums,
// reads back the new XOR and SUB, changes nothing but the routine and the
// checksums, and patching the old values back gives the original file.
func TestSavePatchedFactory(t *testing.T) {
	root := cmp.Or(os.Getenv("ECUSYMBOL_TESTDATA"), filepath.Join("..", "..", "..", "..", "ecusymbol", "testdata"))
	bins, _ := filepath.Glob(filepath.Join(root, "T7Binaries", "*.[bB][iI][nN]"))
	if len(bins) == 0 {
		t.Skip("no T7 binaries under " + root)
	}
	for _, bin := range bins {
		t.Run(filepath.Base(bin), func(t *testing.T) {
			t.Parallel()
			orig, err := os.ReadFile(bin)
			if err != nil {
				t.Fatal(err)
			}
			off, xor, sub, err := Find(orig)
			if err != nil {
				t.Skip(err)
			}
			nx, ns := xor^0x5A5A, sub+0x1111
			dst := filepath.Join(t.TempDir(), "patched.bin")
			if err := SavePatched(dst, bytes.Clone(orig), off, nx, ns); err != nil {
				t.Fatal(err)
			}
			saved, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := symbol.NewT7File(bytes.Clone(saved), symbol.WithT7PrintFunc(func(string) {})); err != nil {
				t.Fatalf("patched file doesn't load: %v", err)
			}
			if o, x, s, err := Find(saved); err != nil || o != off || x != nx || s != ns {
				t.Fatalf("reads back off %X xor %04X sub %04X (%v), want %X %04X %04X", o, x, s, err, off, nx, ns)
			}
			patch := func(i int) bool {
				return i >= off+xorOff && i < off+xorOff+2 || i >= off+subOff && i < off+subOff+2
			}
			moved := 0
			for i := range orig {
				if saved[i] != orig[i] && !patch(i) {
					moved++
				}
			}
			if moved > 256 { // checksums and footer fields
				t.Errorf("%d bytes outside the routine changed", moved)
			}
			back := filepath.Join(t.TempDir(), "back.bin")
			if err := SavePatched(back, saved, off, xor, sub); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(back); !bytes.Equal(got, orig) {
				t.Errorf("patching the old values back doesn't give the original file")
			}
		})
	}
}
