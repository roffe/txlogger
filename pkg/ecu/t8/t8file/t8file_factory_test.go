package t8file

import (
	"bytes"
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"testing"

	symbol "github.com/roffe/ecusymbol"
)

// On every T8 binary under ecusymbol's testdata (ECUSYMBOL_TESTDATA): a new
// VIN, PIN, PSK and ISK saved into the NVDM records leave a file that still
// loads with valid checksums (when it did) and reads them back; every record keeps its own
// other bytes, nothing outside the records moves, and saving the old values
// gives back every record that held them.
func TestSaveFlashBlockFactory(t *testing.T) {
	root := cmp.Or(os.Getenv("ECUSYMBOL_TESTDATA"), filepath.Join("..", "..", "..", "..", "..", "ecusymbol", "testdata"))
	bins, _ := filepath.Glob(filepath.Join(root, "T8Binaries", "*.[bB][iI][nN]"))
	if len(bins) == 0 {
		t.Skip("no T8 binaries under " + root)
	}
	for _, bin := range bins {
		t.Run(filepath.Base(bin), func(t *testing.T) {
			orig, err := os.ReadFile(bin)
			if err != nil {
				t.Fatal(err)
			}
			_, err = symbol.NewT8File(bytes.Clone(orig), symbol.WithT8PrintFunc(func(string) {}))
			valid := err == nil // a file that loaded strictly must still load after the save
			path := writeBin(t, orig)
			tf := new(T8File)
			th := tf.GetInfo(path)
			blocks := slices.Clone(th.FlashBlocks())
			if len(blocks) == 0 {
				t.Skip("no flash blocks")
			}
			vin, pin, psk, isk := th.VIN(), th.PIN(), th.PSK(), th.ISK()
			th.SetVIN("YS3FB49F741999999")
			th.SetPIN("ZZ99")
			th.SetPSK("CAFEBABE0001")
			th.SetISK("A1A2A3A4A5A6")
			if err := tf.SaveFlashBlock(); err != nil {
				t.Fatal(err)
			}
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := symbol.NewT8File(bytes.Clone(saved), symbol.WithT8PrintFunc(func(string) {})); valid && err != nil {
				t.Fatalf("saved file doesn't load: %v", err)
			}
			for i := range orig {
				in := false
				for _, fb := range blocks {
					in = in || i >= fb.BlockAddress && i < fb.BlockAddress+len(fb.BlockData)
				}
				if saved[i] != orig[i] && !in {
					t.Fatalf("byte %X outside the flash blocks changed: %02X -> %02X", i, orig[i], saved[i])
				}
			}
			edited := func(off int) bool { // VIN, PIN, PSK, ISK
				return off >= 0xE0 && off < 0xE0+17 || off >= 0x40 && off < 0x44 || off >= 0x55 && off < 0x5B || off >= 0x61 && off < 0x67
			}
			holdsOld := map[int]bool{} // records that held the values the header shows
			for _, fb := range blocks {
				nfb := FlashBlock{BlockData: saved[fb.BlockAddress : fb.BlockAddress+len(fb.BlockData)]}
				was, now := new(T8Header), new(T8Header)
				fb.DecodeBlock(was)
				nfb.DecodeBlock(now)
				holdsOld[fb.BlockAddress] = was.VIN() == vin && was.PIN() == pin && was.PSK() == psk && was.ISK() == isk
				if now.VIN() != "YS3FB49F741999999" || now.PIN() != "ZZ99" || now.PSK() != "CAFEBABE0001" || now.ISK() != "A1A2A3A4A5A6" {
					t.Errorf("record %X reads back VIN %q PIN %q PSK %q ISK %q", fb.BlockAddress, now.VIN(), now.PIN(), now.PSK(), now.ISK())
				}
				a, sa, _ := fb.record()
				b, sb, _ := nfb.record()
				if sa != sb {
					t.Errorf("record %X changed between scrambled and plain", fb.BlockAddress)
				}
				for i := range a {
					if a[i] != b[i] && !edited(i) {
						t.Errorf("record %X: byte %X outside the edited fields changed", fb.BlockAddress, i)
						break
					}
				}
			}
			th.SetVIN(vin)
			th.SetPIN(pin)
			th.SetPSK(psk)
			th.SetISK(isk)
			if err := tf.SaveFlashBlock(); err != nil {
				t.Fatal(err)
			}
			back, _ := os.ReadFile(path)
			for i := range orig {
				in, keep := false, true
				for _, fb := range blocks {
					if i >= fb.BlockAddress && i < fb.BlockAddress+len(fb.BlockData) {
						in, keep = true, holdsOld[fb.BlockAddress]
					}
				}
				if back[i] != orig[i] && (!in || keep) {
					t.Errorf("the old values back leave byte %X off the original (%d bytes in all)", i, countDiffBytes(back, orig))
					break
				}
			}
		})
	}
}

func countDiffBytes(a, b []byte) int {
	n := 0
	for i := range a {
		if a[i] != b[i] {
			n++
		}
	}
	return n
}
