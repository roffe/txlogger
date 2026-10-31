package update

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

const (
	latestURL   = "https://api.github.com/repos/roffe/txlogger/releases/latest"
	downloadURL = "https://github.com/roffe/txlogger/releases/download/v1.2.0/txlogger-x86_64.AppImage"
)

// releaseJSON is a trimmed GitHub "latest release" reply for tag.
func releaseJSON(tag string) string {
	return fmt.Sprintf(`{
  "tag_name": %q,
  "name": "txlogger %s",
  "html_url": "https://github.com/roffe/txlogger/releases/tag/%s",
  "prerelease": false,
  "published_at": "2025-06-01T12:00:00Z",
  "author": {"login": "roffe", "id": 1},
  "assets": [
    {"name": "txlogger.zip", "size": 10, "browser_download_url": "https://github.com/roffe/txlogger/releases/download/%s/txlogger.zip"},
    {"name": "txlogger-x86_64.AppImage", "size": 20, "browser_download_url": %q}
  ]
}`, tag, tag, tag, tag, downloadURL)
}

// fakeGitHub routes every request made through http.DefaultTransport (and so
// http.Get) to handler for the duration of the test, and records the URLs
// requested. Nothing leaves the process.
type fakeGitHub struct {
	mu   sync.Mutex
	urls []string
}

func (f *fakeGitHub) requested() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.urls...)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func serveFake(t *testing.T, handler http.HandlerFunc) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{}
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		f.mu.Lock()
		f.urls = append(f.urls, r.URL.String())
		f.mu.Unlock()
		rec := httptest.NewRecorder()
		handler(rec, r)
		resp := rec.Result()
		resp.Request = r
		return resp, nil
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	return f
}

// github answers the latest-release query with tag and the AppImage
// download with image (or downloadStatus when it is not 200).
func github(tag string, image []byte, downloadStatus int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.String() {
		case latestURL:
			io.WriteString(w, releaseJSON(tag))
		case downloadURL:
			if downloadStatus != http.StatusOK {
				w.WriteHeader(downloadStatus)
				return
			}
			w.Write(image)
		default:
			http.NotFound(w, r)
		}
	}
}

// GetLatest fetches the latest release from the txlogger GitHub repo and
// decodes its tag and assets.
func TestGetLatest(t *testing.T) {
	gh := serveFake(t, github("v1.2.0", nil, http.StatusOK))
	rel, err := GetLatest()
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if got := gh.requested(); len(got) != 1 || got[0] != latestURL {
		t.Errorf("requested %v, want [%s]", got, latestURL)
	}
	if rel.TagName != "v1.2.0" || rel.Author.Login != "roffe" || rel.Prerelease {
		t.Errorf("release = tag %q author %q prerelease %v", rel.TagName, rel.Author.Login, rel.Prerelease)
	}
	if !rel.PublishedAt.Equal(time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("PublishedAt = %v", rel.PublishedAt)
	}
	if len(rel.Assets) != 2 || rel.Assets[1].Name != appImageAsset || rel.Assets[1].BrowserDownloadURL != downloadURL {
		t.Errorf("assets = %+v", rel.Assets)
	}
}

// GetLatest fails on a body that is not a release object or when the
// request itself fails.
func TestGetLatestErrors(t *testing.T) {
	serveFake(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "<html>bad gateway</html>") })
	if rel, err := GetLatest(); err == nil {
		t.Errorf("non-JSON body: got %+v, want error", rel)
	}

	netErr := errors.New("no route to host")
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, netErr })
	defer func() { http.DefaultTransport = orig }()
	if _, err := GetLatest(); !errors.Is(err, netErr) {
		t.Errorf("transport error: err = %v, want %v", err, netErr)
	}
}

// GetLatest treats a non-200 reply, such as GitHub's JSON rate-limit or
// not-found message, as an error rather than a release with no tag.
func TestGetLatestHTTPStatus(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusNotFound} {
		serveFake(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
			io.WriteString(w, `{"message":"API rate limit exceeded","documentation_url":"https://docs.github.com"}`)
		})
		if rel, err := GetLatest(); err == nil {
			t.Errorf("status %d: got release with tag %q, want error", code, rel.TagName)
		}
	}
}

// httpGetBody returns the body of a reply and the error of a failed request.
func TestHTTPGetBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello "+r.URL.Path)
	}))
	defer srv.Close()
	b, err := httpGetBody(srv.URL + "/x")
	if err != nil || string(b) != "hello /x" {
		t.Errorf("httpGetBody = %q, %v; want hello /x", b, err)
	}

	srv.Close()
	if _, err := httpGetBody(srv.URL); err == nil {
		t.Error("closed server: want error, got nil")
	}
}

// IsLatest compares the given version with the latest tag by semver and
// returns that tag; when the check fails it assumes the given version is
// current.
func TestIsLatest(t *testing.T) {
	serveFake(t, github("v1.2.0", nil, http.StatusOK))
	cases := []struct {
		version string
		want    bool
	}{
		{"v1.1.9", false},
		{"v1.2.0", true},
		{"v1.10.0", true}, // numeric, not lexical, comparison
		{"v2.0.0", true},
	}
	for _, tc := range cases {
		if ok, tag := IsLatest(tc.version); ok != tc.want || tag != "v1.2.0" {
			t.Errorf("IsLatest(%q) = %v, %q; want %v, v1.2.0", tc.version, ok, tag, tc.want)
		}
	}

	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	defer func() { http.DefaultTransport = orig }()
	if ok, tag := IsLatest("v1.0.0"); !ok || tag != "v1.0.0" {
		t.Errorf("offline IsLatest = %v, %q; want true, v1.0.0", ok, tag)
	}
}

// appImage points APPIMAGE at a fake running AppImage in a temp dir.
func appImage(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "txlogger.AppImage")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPIMAGE", path)
	return path
}

// assertOnly checks dir holds exactly the named file, i.e. no temp file was
// left behind.
func assertOnly(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != name {
		t.Errorf("dir holds %v, want only %s", names, name)
	}
}

func release(t *testing.T) *Release {
	t.Helper()
	serveFake(t, github("v1.2.0", nil, http.StatusOK))
	rel, err := GetLatest()
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

// selfUpdateAppImage downloads the release's AppImage asset and replaces the
// running AppImage in place with an executable copy.
func TestSelfUpdateAppImage(t *testing.T) {
	rel := release(t)
	target := appImage(t, "old image")
	gh := serveFake(t, github("v1.2.0", []byte("new image"), http.StatusOK))

	if err := selfUpdateAppImage(rel); err != nil {
		t.Fatalf("selfUpdateAppImage: %v", err)
	}
	if got := gh.requested(); len(got) != 1 || got[0] != downloadURL {
		t.Errorf("requested %v, want [%s]", got, downloadURL)
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "new image" {
		t.Errorf("target = %q, %v; want new image", b, err)
	}
	if fi, err := os.Stat(target); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("target mode = %v, %v; want 0755", fi.Mode(), err)
	}
	assertOnly(t, filepath.Dir(target), filepath.Base(target))
}

// errBody fails after returning some bytes, like a connection dropped mid
// download.
type errBody struct{ sent bool }

func (b *errBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		return copy(p, "partial"), nil
	}
	return 0, errors.New("connection reset")
}
func (*errBody) Close() error { return nil }

// selfUpdateAppImage leaves the running AppImage untouched and removes its
// temp file when it cannot complete the download.
func TestSelfUpdateAppImageFailures(t *testing.T) {
	cases := []struct {
		name      string
		rel       func(*Release)
		transport func(t *testing.T) *fakeGitHub
		wantErr   string
		wantGet   bool
	}{
		{
			name:    "no AppImage asset",
			rel:     func(r *Release) { r.Assets = r.Assets[:1] },
			wantErr: "release v1.2.0 has no txlogger-x86_64.AppImage",
		},
		{
			name: "download 404",
			transport: func(t *testing.T) *fakeGitHub {
				return serveFake(t, github("v1.2.0", nil, http.StatusNotFound))
			},
			wantErr: "download: 404 Not Found",
			wantGet: true,
		},
		{
			name: "dropped mid download",
			transport: func(t *testing.T) *fakeGitHub {
				f := &fakeGitHub{}
				orig := http.DefaultTransport
				http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
					f.mu.Lock()
					f.urls = append(f.urls, r.URL.String())
					f.mu.Unlock()
					return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: &errBody{}, Request: r}, nil
				})
				t.Cleanup(func() { http.DefaultTransport = orig })
				return f
			},
			wantErr: "connection reset",
			wantGet: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel := release(t)
			if tc.rel != nil {
				tc.rel(rel)
			}
			target := appImage(t, "old image")
			gh := serveFake(t, github("v1.2.0", []byte("new image"), http.StatusOK))
			if tc.transport != nil {
				gh = tc.transport(t)
			}

			err := selfUpdateAppImage(rel)
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("err = %v, want %q", err, tc.wantErr)
			}
			if b, _ := os.ReadFile(target); string(b) != "old image" {
				t.Errorf("target = %q, want untouched old image", b)
			}
			assertOnly(t, filepath.Dir(target), filepath.Base(target))
			if got := gh.requested(); (len(got) != 0) != tc.wantGet {
				t.Errorf("requested %v, want download attempted = %v", got, tc.wantGet)
			}
		})
	}
}

// selfUpdateAppImage refuses to run outside an AppImage and reports a stale
// APPIMAGE path whose directory is gone.
func TestSelfUpdateAppImageEnvironment(t *testing.T) {
	rel := release(t)
	t.Setenv("APPIMAGE", "")
	if err := selfUpdateAppImage(rel); err == nil || err.Error() != "not running as an AppImage" {
		t.Errorf("no APPIMAGE: err = %v", err)
	}

	serveFake(t, github("v1.2.0", []byte("new image"), http.StatusOK))
	t.Setenv("APPIMAGE", filepath.Join(t.TempDir(), "gone", "txlogger.AppImage"))
	if err := selfUpdateAppImage(rel); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing dir: err = %v, want not exist", err)
	}
}

// versionApp reports a fixed app version and routes fyne.Do calls to a
// channel, so the test runs them on its own goroutine.
type versionApp struct {
	fyne.App
	version string
	drv     *queueDriver
}

func (a *versionApp) Metadata() fyne.AppMetadata { return fyne.AppMetadata{Version: a.version} }
func (a *versionApp) Driver() fyne.Driver        { return a.drv }

type queueDriver struct {
	fyne.Driver
	q chan func()
}

func (d *queueDriver) DoFromGoroutine(f func(), _ bool) { d.q <- f }

// startApp installs a test app reporting version and returns it with a
// window for UpdateCheck's dialogs.
func startApp(t *testing.T, version string) (*versionApp, fyne.Window) {
	t.Helper()
	base := test.NewTempApp(t)
	a := &versionApp{App: base, version: version, drv: &queueDriver{Driver: base.Driver(), q: make(chan func(), 16)}}
	fyne.SetCurrentApp(a)
	w := test.NewWindow(nil)
	w.Resize(fyne.NewSize(800, 600))
	t.Cleanup(w.Close)
	return a, w
}

// runQueued runs fyne.Do calls queued by background goroutines until done
// reports true.
func runQueued(t *testing.T, a *versionApp, done func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for !done() {
		select {
		case f := <-a.drv.q:
			f()
		case <-deadline:
			t.Fatal("timed out waiting for UI update")
		}
	}
}

// overlayTexts collects the text of labels, hyperlinks, buttons and canvas
// text in the window's open dialogs.
func overlayTexts(w fyne.Window) string {
	var out []string
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if !o.Visible() {
			return
		}
		switch v := o.(type) {
		case *widget.Label:
			out = append(out, v.Text)
		case *widget.Hyperlink:
			out = append(out, v.Text+" <"+v.URL.String()+">")
		case *widget.Button:
			out = append(out, "["+v.Text+"]")
		case *canvas.Text:
			out = append(out, v.Text)
		}
		switch v := o.(type) {
		case *fyne.Container:
			for _, c := range v.Objects {
				walk(c)
			}
		case fyne.Widget:
			for _, c := range test.WidgetRenderer(v).Objects() {
				walk(c)
			}
		}
	}
	for _, o := range w.Canvas().Overlays().List() {
		walk(o)
	}
	return strings.Join(out, "\n")
}

// findButton returns the visible dialog button labeled text, or nil.
func findButton(w fyne.Window, text string) *widget.Button {
	var found *widget.Button
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if found != nil || !o.Visible() {
			return
		}
		if b, ok := o.(*widget.Button); ok && b.Text == text {
			found = b
			return
		}
		switch v := o.(type) {
		case *fyne.Container:
			for _, c := range v.Objects {
				walk(c)
			}
		case fyne.Widget:
			for _, c := range test.WidgetRenderer(v).Objects() {
				walk(c)
			}
		}
	}
	for _, o := range w.Canvas().Overlays().List() {
		walk(o)
	}
	return found
}

// UpdateCheck shows the current and latest versions with a link to
// txlogger.com when a newer release exists, and offers no in-place update
// outside an AppImage.
func TestUpdateCheckNewer(t *testing.T) {
	t.Setenv("APPIMAGE", "")
	a, w := startApp(t, "1.1.0")
	serveFake(t, github("v1.2.0", nil, http.StatusOK))

	UpdateCheck(a, w)
	got := overlayTexts(w)
	for _, want := range []string{
		"Update available",
		"Current version: v1.1.0",
		"Latest version: v1.2.0",
		"Get it at txlogger.com <https://txlogger.com>",
		"[Close]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dialog missing %q; got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Update AppImage now") {
		t.Errorf("AppImage update offered outside an AppImage:\n%s", got)
	}
}

// UpdateCheck says so when the running version is the latest or newer.
func TestUpdateCheckUpToDate(t *testing.T) {
	for _, version := range []string{"1.2.0", "1.3.0"} {
		a, w := startApp(t, version)
		serveFake(t, github("v1.2.0", nil, http.StatusOK))
		UpdateCheck(a, w)
		got := overlayTexts(w)
		if !strings.Contains(got, "No update available") || !strings.Contains(got, "You are running the latest version") {
			t.Errorf("version %s: dialog =\n%s", version, got)
		}
	}
}

// UpdateCheck shows an error dialog when the release lookup fails.
func TestUpdateCheckError(t *testing.T) {
	a, w := startApp(t, "1.1.0")
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	defer func() { http.DefaultTransport = orig }()

	UpdateCheck(a, w)
	got := overlayTexts(w)
	if !strings.Contains(got, "Error") || !strings.Contains(got, "offline") || strings.Contains(got, "latest version") {
		t.Errorf("dialog =\n%s", got)
	}
}

// Inside an AppImage, UpdateCheck offers an in-place update that downloads
// the new image, replaces the running one and asks for a restart.
func TestUpdateCheckAppImage(t *testing.T) {
	target := appImage(t, "old image")
	a, w := startApp(t, "1.1.0")
	serveFake(t, github("v1.2.0", []byte("new image"), http.StatusOK))

	UpdateCheck(a, w)
	btn := findButton(w, "Update AppImage now")
	if btn == nil {
		t.Fatalf("no AppImage update button; dialog:\n%s", overlayTexts(w))
	}
	test.Tap(btn)
	if got := overlayTexts(w); !strings.Contains(got, "Downloading v1.2.0...") {
		t.Errorf("no progress dialog; got:\n%s", got)
	}

	runQueued(t, a, func() bool { return strings.Contains(overlayTexts(w), "Restart txlogger to run v1.2.0") })
	if got := overlayTexts(w); strings.Contains(got, "Downloading v1.2.0...") {
		t.Errorf("progress dialog still shown after update:\n%s", got)
	}
	if b, _ := os.ReadFile(target); string(b) != "new image" {
		t.Errorf("target = %q, want new image", b)
	}
}

// A failed in-place update closes the progress dialog and shows the error.
func TestUpdateCheckAppImageFails(t *testing.T) {
	target := appImage(t, "old image")
	a, w := startApp(t, "1.1.0")
	serveFake(t, github("v1.2.0", nil, http.StatusInternalServerError))

	UpdateCheck(a, w)
	btn := findButton(w, "Update AppImage now")
	if btn == nil {
		t.Fatalf("no AppImage update button; dialog:\n%s", overlayTexts(w))
	}
	test.Tap(btn)

	runQueued(t, a, func() bool { return strings.Contains(overlayTexts(w), "Download: 500 Internal Server Error") })
	got := overlayTexts(w)
	if strings.Contains(got, "Downloading v1.2.0...") || strings.Contains(got, "Restart txlogger") {
		t.Errorf("dialogs after failed update:\n%s", got)
	}
	if b, _ := os.ReadFile(target); string(b) != "old image" {
		t.Errorf("target = %q, want untouched old image", b)
	}
}
