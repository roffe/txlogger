package capture

import (
	"bytes"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

// redWindow returns a 200x100 test window filled with solid red.
func redWindow(t *testing.T) fyne.Window {
	t.Helper()
	test.NewTempApp(t)
	w := test.NewTempWindow(t, canvas.NewRectangle(color.NRGBA{R: 255, A: 255}))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(200, 100))
	return w
}

// shoot runs Screenshot in a fresh working directory and returns the files it
// left there.
func shoot(t *testing.T, c fyne.Canvas) (dir string, files []string) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)
	Screenshot(c)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		files = append(files, e.Name())
	}
	return dir, files
}

// Screenshot saves the canvas into the working directory as
// capture-<YYYY-MM-DD-HH-MM-SS>.<ext>, holding an image of the canvas at its
// rendered size. The extension is checked against the content separately, in
// TestScreenshotFormatMatchesExtension.
func TestScreenshotWritesCanvasImage(t *testing.T) {
	w := redWindow(t)

	before := time.Now().Truncate(time.Second)
	dir, files := shoot(t, w.Canvas())
	after := time.Now()

	if len(files) != 1 {
		t.Fatalf("got files %q, want exactly one", files)
	}
	name := files[0]
	ext := filepath.Ext(name)
	stamp, ok := strings.CutPrefix(strings.TrimSuffix(name, ext), "capture-")
	if !ok || (ext != ".jpg" && ext != ".png") {
		t.Fatalf("file name %q, want capture-<time>.jpg or .png", name)
	}
	ts, err := time.ParseInLocation("2006-01-02-15-04-05", stamp, time.Local)
	if err != nil || ts.Before(before) || ts.After(after) {
		t.Fatalf("timestamp %q (%v) not within [%v, %v]: %v", stamp, ts, before, after, err)
	}

	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("saved file is not a decodable image: %v", err)
	}
	if got := img.Bounds().Size(); got != image.Pt(200, 100) {
		t.Fatalf("image size %v, want 200x100", got)
	}
	r, g, bl, _ := img.At(100, 50).RGBA()
	if r>>8 < 240 || g>>8 > 15 || bl>>8 > 15 {
		t.Fatalf("center pixel = (%d,%d,%d), want red", r>>8, g>>8, bl>>8)
	}
}

// The saved file's content matches its extension: JPEG data in a .jpg file or
// PNG data in a .png file.
func TestScreenshotFormatMatchesExtension(t *testing.T) {
	w := redWindow(t)
	dir, files := shoot(t, w.Canvas())
	if len(files) != 1 {
		t.Fatalf("got files %q, want exactly one", files)
	}
	f, err := os.Open(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, format, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{".jpg": "jpeg", ".jpeg": "jpeg", ".png": "png"}
	if ext := filepath.Ext(files[0]); format != want[ext] {
		t.Fatalf("%s file holds %s data", ext, format)
	}
}

// When the working directory is not writable the failure is logged and
// nothing is written.
func TestScreenshotUnwritableDir(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions as a non-root user")
	}
	w := redWindow(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	t.Chdir(dir)
	out := captureLog(t)

	Screenshot(w.Canvas())

	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files written: %v", entries)
	}
	if !strings.Contains(out.String(), "permission denied") {
		t.Fatalf("log output = %q, want permission error", out.String())
	}
}

// stubCanvas is a fyne.Canvas whose Capture returns a fixed image.
type stubCanvas struct {
	fyne.Canvas
	img image.Image
}

func (c stubCanvas) Capture() image.Image { return c.img }

// captureLog redirects the standard logger for the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// When the captured image cannot be encoded (a zero-sized canvas) the encoder
// error is logged. The file has already been created by then, so an empty
// capture file is left behind.
func TestScreenshotEncodeError(t *testing.T) {
	out := captureLog(t)
	dir, files := shoot(t, stubCanvas{img: image.NewRGBA(image.Rect(0, 0, 0, 0))})

	if !strings.Contains(out.String(), "invalid image size") {
		t.Fatalf("log output = %q, want the encoder's invalid image size error", out.String())
	}
	if len(files) != 1 {
		t.Fatalf("got files %q, want the one created before encoding", files)
	}
	if fi, err := os.Stat(filepath.Join(dir, files[0])); err != nil || fi.Size() != 0 {
		t.Fatalf("capture file: %v, %v; want an empty file", fi, err)
	}
}

// When writing the encoded image fails (here the device is full) the write
// error is logged.
func TestScreenshotWriteError(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("needs /dev/full")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	// Screenshot picks its file name from the current second, so every name
	// it could choose in the next two minutes is a link to /dev/full.
	now := time.Now()
	for i := range 120 {
		name := "capture-" + now.Add(time.Duration(i)*time.Second).Format("2006-01-02-15-04-05") + ".png"
		if err := os.Symlink("/dev/full", filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	out := captureLog(t)
	Screenshot(stubCanvas{img: image.NewRGBA(image.Rect(0, 0, 4, 4))})

	if !strings.Contains(out.String(), "no space left on device") {
		t.Fatalf("log output = %q, want the write error", out.String())
	}
}
