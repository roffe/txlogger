package txwebclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

// request is what the fake txweb server saw of one call.
type request struct {
	method, path, auth, accept, contentType string
	body                                    []byte
	// upload fields, set for multipart posts
	formName string
	formData []byte
}

// fakeServer starts an httptest server that records every request and
// answers with handle. Multipart uploads are parsed before handle runs.
func fakeServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) (url string, reqs <-chan request) {
	t.Helper()
	ch := make(chan request, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rq := request{
			method:      r.Method,
			path:        r.URL.Path,
			auth:        r.Header.Get("Authorization"),
			accept:      r.Header.Get("Accept"),
			contentType: r.Header.Get("Content-Type"),
		}
		if strings.HasPrefix(rq.contentType, "multipart/form-data") {
			f, hdr, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				ch <- rq
				return
			}
			rq.formName = hdr.Filename
			rq.formData, _ = io.ReadAll(f)
			f.Close()
		} else {
			rq.body, _ = io.ReadAll(r.Body)
		}
		ch <- rq
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, ch
}

func status(code int) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
}

func next(t *testing.T, reqs <-chan request) request {
	t.Helper()
	select {
	case r := <-reqs:
		return r
	default:
		t.Fatal("server received no request")
		return request{}
	}
}

// closedURL returns the address of a server that is no longer listening.
func closedURL() string {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.URL
}

// Login posts the credentials as JSON to <endpoint>/login and returns the
// token and expiry from a 200 reply.
func TestLogin(t *testing.T) {
	url, reqs := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"message":"ok","token":"tok123","user":"roffe","expires":1700000000}`))
	})
	token, exp, err := Login(url, "roffe", "s3cret")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token != "tok123" || exp != 1700000000 {
		t.Errorf("Login = %q, %d; want tok123, 1700000000", token, exp)
	}
	r := next(t, reqs)
	if r.method != http.MethodPost || r.path != "/login" || r.contentType != "application/json" {
		t.Errorf("request = %s %s (%s), want POST /login (application/json)", r.method, r.path, r.contentType)
	}
	var got LoginRequest
	if err := json.Unmarshal(r.body, &got); err != nil {
		t.Fatalf("login body %q: %v", r.body, err)
	}
	if got != (LoginRequest{Username: "roffe", Password: "s3cret"}) {
		t.Errorf("login body = %+v", got)
	}
}

// Login maps rate limiting, bad credentials and other non-200 replies to
// distinct errors, and fails on an unparseable 200 body.
func TestLoginErrors(t *testing.T) {
	cases := []struct {
		name    string
		handle  func(http.ResponseWriter, *http.Request)
		wantErr string
	}{
		{"rate limited", status(http.StatusTooManyRequests), "too many login attempts"},
		{"bad credentials", status(http.StatusUnauthorized), "invalid username or password"},
		{"server error", status(http.StatusInternalServerError), "login failed (500)"},
		{"bad json", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("<html>")) }, "invalid character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, _ := fakeServer(t, tc.handle)
			token, _, err := Login(url, "u", "p")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Login err = %v, want containing %q", err, tc.wantErr)
			}
			if token != "" {
				t.Errorf("token = %q on error, want empty", token)
			}
		})
	}
	if _, _, err := Login(closedURL(), "u", "p"); err == nil {
		t.Error("Login to closed server: want error, got nil")
	}
}

// New talks to the production API by default; options and setters replace
// the endpoint and bearer token used for later requests.
func TestNewOptionsAndSetters(t *testing.T) {
	c := New()
	if c.endpoint != "https://api.txlogger.com" || c.token != "" {
		t.Errorf("defaults = %q, %q", c.endpoint, c.token)
	}

	url, reqs := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("[]")) })
	c = New(WithEndpoint("http://unused.invalid"), WithToken("old"))
	c.SetEndpoint(url)
	c.SetToken("new")
	if _, err := c.ListFiles(); err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if r := next(t, reqs); r.auth != "Bearer new" {
		t.Errorf("Authorization = %q, want %q", r.auth, "Bearer new")
	}
}

// Every authenticated call fails with ErrMissingToken, without touching the
// network, until a token is set.
func TestMissingToken(t *testing.T) {
	url, reqs := fakeServer(t, status(http.StatusOK))
	c := New(WithEndpoint(url))
	calls := map[string]func() error{
		"DeleteFile":       func() error { return c.DeleteFile("a.t7l") },
		"ListFiles":        func() error { _, err := c.ListFiles(); return err },
		"DownloadFile":     func() error { _, err := c.DownloadFile("a.t7l"); return err },
		"UploadFile":       func() error { return c.UploadFile("a.t7l") },
		"UploadFileBytes":  func() error { return c.UploadFileBytes("a.t7l", []byte("x")) },
		"UploadFileReader": func() error { return c.UploadFileReader("a.t7l", strings.NewReader("x")) },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, ErrMissingToken) {
			t.Errorf("%s: err = %v, want ErrMissingToken", name, err)
		}
	}
	if len(reqs) != 0 {
		t.Errorf("server saw %d requests, want 0", len(reqs))
	}
}

// DeleteFile sends DELETE /api/files/<name> and treats only 204 as success.
func TestDeleteFile(t *testing.T) {
	cases := []struct {
		code    int
		wantErr string
	}{
		{http.StatusNoContent, ""},
		{http.StatusOK, "failed to delete file (200)"},
		{http.StatusNotFound, "failed to delete file (404)"},
	}
	for _, tc := range cases {
		url, reqs := fakeServer(t, status(tc.code))
		c := New(WithEndpoint(url), WithToken("tok"))
		err := c.DeleteFile("2024-01-02 log.t7l")
		if tc.wantErr == "" && err != nil {
			t.Errorf("status %d: err = %v, want nil", tc.code, err)
		}
		if tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr) {
			t.Errorf("status %d: err = %v, want %q", tc.code, err, tc.wantErr)
		}
		r := next(t, reqs)
		if r.method != http.MethodDelete || r.path != "/api/files/2024-01-02 log.t7l" || r.auth != "Bearer tok" {
			t.Errorf("request = %s %s auth %q", r.method, r.path, r.auth)
		}
	}
}

// ListFiles asks for JSON at /api/files and decodes name, size and
// modification time of each entry.
func TestListFiles(t *testing.T) {
	url, reqs := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`[{"name":"a.t7l","size":1024,"mod_time":"2025-03-04T05:06:07Z"},{"name":"b.t8l","size":0,"mod_time":"2025-01-01T00:00:00+01:00"}]`))
	})
	c := New(WithEndpoint(url), WithToken("tok"))
	files, err := c.ListFiles()
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	want := []File{
		{Name: "a.t7l", Size: 1024, ModTime: time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)},
		{Name: "b.t8l", Size: 0, ModTime: time.Date(2024, 12, 31, 23, 0, 0, 0, time.UTC)},
	}
	if len(files) != len(want) {
		t.Fatalf("got %d files, want %d", len(files), len(want))
	}
	for i := range want {
		if files[i].Name != want[i].Name || files[i].Size != want[i].Size || !files[i].ModTime.Equal(want[i].ModTime) {
			t.Errorf("file %d = %+v, want %+v", i, files[i], want[i])
		}
	}
	r := next(t, reqs)
	if r.method != http.MethodGet || r.path != "/api/files" || r.accept != "application/json" || r.auth != "Bearer tok" {
		t.Errorf("request = %s %s accept %q auth %q", r.method, r.path, r.accept, r.auth)
	}
}

// ListFiles fails on a non-200 status or a body that is not a JSON list.
func TestListFilesErrors(t *testing.T) {
	cases := []struct {
		name    string
		handle  func(http.ResponseWriter, *http.Request)
		wantErr string
	}{
		{"unauthorized", status(http.StatusUnauthorized), "failed to list files (401)"},
		{"not a list", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"error":"x"}`)) }, "cannot unmarshal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, _ := fakeServer(t, tc.handle)
			files, err := New(WithEndpoint(url), WithToken("tok")).ListFiles()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tc.wantErr)
			}
			if files != nil {
				t.Errorf("files = %v on error, want nil", files)
			}
		})
	}
}

// DownloadFile GETs /api/files/<name> and returns the body of a 200 reply.
func TestDownloadFile(t *testing.T) {
	payload := []byte{0x00, 0x01, 0xFE, 0xFF, 'l', 'o', 'g'}
	url, reqs := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) { w.Write(payload) })
	data, err := New(WithEndpoint(url), WithToken("tok")).DownloadFile("a.t7l")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Errorf("data = % X, want % X", data, payload)
	}
	r := next(t, reqs)
	if r.method != http.MethodGet || r.path != "/api/files/a.t7l" || r.auth != "Bearer tok" {
		t.Errorf("request = %s %s auth %q", r.method, r.path, r.auth)
	}

	url, _ = fakeServer(t, status(http.StatusNotFound))
	data, err = New(WithEndpoint(url), WithToken("tok")).DownloadFile("gone.t7l")
	if err == nil || err.Error() != "failed to download file (404)" || data != nil {
		t.Errorf("404: data %v, err %v; want nil, failed to download file (404)", data, err)
	}
}

// truncated replies 200 with a Content-Length longer than the body it sends,
// so the server drops the connection mid-body.
func truncated(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Length", "100")
	w.Write([]byte(`{"token":`))
}

// A reply cut off mid-body is an error, not partial data.
func TestTruncatedReply(t *testing.T) {
	url, _ := fakeServer(t, truncated)
	data, err := New(WithEndpoint(url), WithToken("tok")).DownloadFile("a.t7l")
	if !errors.Is(err, io.ErrUnexpectedEOF) || data != nil {
		t.Errorf("DownloadFile = %d bytes, %v; want nil, unexpected EOF", len(data), err)
	}
	if _, _, err := Login(url, "u", "p"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("Login err = %v, want unexpected EOF", err)
	}
}

// DownloadFile and DeleteFile put the name from ListFiles into the URL path
// as is. The txweb server only stores names made of letters, digits, '_',
// '-' and '.', with a "_(n)" suffix for duplicates, so this addresses every
// file it can list.
func TestFileNameInPath(t *testing.T) {
	for _, name := range []string{"2025-01-01_T7.t7l", "log_(1).t8l"} {
		url, reqs := fakeServer(t, status(http.StatusNoContent))
		c := New(WithEndpoint(url), WithToken("tok"))
		if err := c.DeleteFile(name); err != nil {
			t.Errorf("DeleteFile(%q): %v", name, err)
		} else if r := next(t, reqs); r.path != "/api/files/"+name {
			t.Errorf("DeleteFile(%q) requested path %q", name, r.path)
		}

		url, reqs = fakeServer(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("x")) })
		c.SetEndpoint(url)
		if _, err := c.DownloadFile(name); err != nil {
			t.Errorf("DownloadFile(%q): %v", name, err)
		} else if r := next(t, reqs); r.path != "/api/files/"+name {
			t.Errorf("DownloadFile(%q) requested path %q", name, r.path)
		}
	}
}

// The upload helpers post a multipart form with the content in field "file",
// named after the base name of the given path, with the bearer token.
func TestUpload(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "2025-01-01 T7.t7l")
	content := bytes.Repeat([]byte("0123456789abcdef"), 64*1024) // 1 MiB, several copy chunks
	if err := os.WriteFile(local, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		upload   func(c *Client) error
		wantName string
		wantData []byte
	}{
		{"UploadFile", func(c *Client) error { return c.UploadFile(local) }, "2025-01-01 T7.t7l", content},
		{"UploadFileBytes", func(c *Client) error { return c.UploadFileBytes("some/dir/b.t8l", []byte("bytes")) }, "b.t8l", []byte("bytes")},
		{"UploadFileReader", func(c *Client) error { return c.UploadFileReader("c.t5l", strings.NewReader("reader")) }, "c.t5l", []byte("reader")},
		{"empty", func(c *Client) error { return c.UploadFileBytes("empty.t7l", nil) }, "empty.t7l", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, reqs := fakeServer(t, status(http.StatusCreated))
			if err := tc.upload(New(WithEndpoint(url), WithToken("tok"))); err != nil {
				t.Fatalf("upload: %v", err)
			}
			r := next(t, reqs)
			if r.method != http.MethodPost || r.path != "/api/upload" || r.auth != "Bearer tok" {
				t.Errorf("request = %s %s auth %q", r.method, r.path, r.auth)
			}
			if r.formName != tc.wantName {
				t.Errorf("form file name = %q, want %q", r.formName, tc.wantName)
			}
			if !bytes.Equal(r.formData, tc.wantData) {
				t.Errorf("form file data: got %d bytes, want %d", len(r.formData), len(tc.wantData))
			}
		})
	}
}

// UploadFile reports a missing local file as an "open:" error wrapping
// fs.ErrNotExist, without contacting the server.
func TestUploadFileMissing(t *testing.T) {
	url, reqs := fakeServer(t, status(http.StatusCreated))
	err := New(WithEndpoint(url), WithToken("tok")).UploadFile(filepath.Join(t.TempDir(), "nope.t7l"))
	if !errors.Is(err, fs.ErrNotExist) || !strings.HasPrefix(err.Error(), "open: ") {
		t.Errorf("err = %v, want open: ... not exist", err)
	}
	if len(reqs) != 0 {
		t.Errorf("server saw %d requests, want 0", len(reqs))
	}
}

// A read error from the upload source aborts the request and is returned.
func TestUploadSourceError(t *testing.T) {
	url, _ := fakeServer(t, status(http.StatusCreated))
	srcErr := errors.New("disk gone")
	err := New(WithEndpoint(url), WithToken("tok")).UploadFileReader("x.t7l", iotest.ErrReader(srcErr))
	if !errors.Is(err, srcErr) || !strings.HasPrefix(err.Error(), "post: ") {
		t.Errorf("err = %v, want post: ... %v", err, srcErr)
	}
}

// Uploading to an unreachable server returns a "post:" error.
func TestUploadUnreachable(t *testing.T) {
	err := New(WithEndpoint(closedURL()), WithToken("tok")).UploadFileBytes("x.t7l", []byte("x"))
	if err == nil || !strings.HasPrefix(err.Error(), "post: ") {
		t.Errorf("err = %v, want post: ...", err)
	}
}

// An upload the server rejects is reported as an error, like the other calls
// do for a non-success status.
func TestUploadRejectedStatus(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusRequestEntityTooLarge, http.StatusInternalServerError} {
		url, _ := fakeServer(t, status(code))
		if err := New(WithEndpoint(url), WithToken("tok")).UploadFileBytes("x.t7l", []byte("x")); err == nil {
			t.Errorf("status %d: err = nil, want error", code)
		}
	}
}

// An upload may run longer than the 5 s API timeout; uploadMultipart sets up
// a 5 minute deadline for it.
func TestUploadAllowsSlowServer(t *testing.T) {
	url, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(6 * time.Second): // server still processing the upload
			w.WriteHeader(http.StatusCreated)
		case <-r.Context().Done():
		}
	})
	if err := New(WithEndpoint(url), WithToken("tok")).UploadFileBytes("x.t7l", []byte("x")); err != nil {
		t.Errorf("upload answered after 6 s: %v", err)
	}
}

// Every call fails cleanly, without a request, when the endpoint is not a
// valid URL.
func TestInvalidEndpoint(t *testing.T) {
	c := New(WithEndpoint("http://bad\x7fhost"), WithToken("tok"))
	calls := map[string]func() error{
		"DeleteFile":      func() error { return c.DeleteFile("a") },
		"ListFiles":       func() error { _, err := c.ListFiles(); return err },
		"DownloadFile":    func() error { _, err := c.DownloadFile("a"); return err },
		"UploadFileBytes": func() error { return c.UploadFileBytes("a", []byte("x")) },
	}
	for name, call := range calls {
		if err := call(); err == nil || !strings.Contains(err.Error(), "invalid control character") {
			t.Errorf("%s: err = %v, want invalid URL error", name, err)
		}
	}
	if _, _, err := Login("http://bad\x7fhost", "u", "p"); err == nil {
		t.Error("Login: want invalid URL error, got nil")
	}
}

// Calls to an unreachable server return the transport error.
func TestUnreachable(t *testing.T) {
	c := New(WithEndpoint(closedURL()), WithToken("tok"))
	if err := c.DeleteFile("a"); err == nil {
		t.Error("DeleteFile: want error, got nil")
	}
	if _, err := c.ListFiles(); err == nil {
		t.Error("ListFiles: want error, got nil")
	}
	if _, err := c.DownloadFile("a"); err == nil {
		t.Error("DownloadFile: want error, got nil")
	}
}

// shortWriter accepts at most n bytes per call and then fails.
type shortWriter struct {
	n   int
	got []byte
}

var errShort = errors.New("short write")

func (s *shortWriter) Write(b []byte) (int, error) {
	if len(b) > s.n {
		s.got = append(s.got, b[:s.n]...)
		return s.n, errShort
	}
	s.got = append(s.got, b...)
	return len(b), nil
}

// progressWriter forwards writes, counts only the bytes the inner writer
// accepted and passes its error through.
func TestProgressWriterCounts(t *testing.T) {
	inner := &shortWriter{n: 4}
	p := &progressWriter{w: inner, lastPrint: time.Now(), printEvery: time.Hour}
	if n, err := p.Write([]byte("abc")); n != 3 || err != nil {
		t.Errorf("Write(abc) = %d, %v; want 3, nil", n, err)
	}
	if n, err := p.Write([]byte("defghi")); n != 4 || !errors.Is(err, errShort) {
		t.Errorf("Write(defghi) = %d, %v; want 4, short write", n, err)
	}
	if p.uploaded != 7 || string(inner.got) != "abcdefg" {
		t.Errorf("uploaded %d, inner got %q; want 7, abcdefg", p.uploaded, inner.got)
	}
}

// progressWriter logs the running total at most once per printEvery.
func TestProgressWriterLogInterval(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	p := &progressWriter{w: io.Discard, lastPrint: time.Now(), printEvery: time.Hour}
	p.Write([]byte("12345"))
	if buf.Len() != 0 {
		t.Errorf("logged %q before printEvery elapsed", buf.String())
	}

	p.lastPrint = time.Time{} // interval long past
	p.Write([]byte("678"))
	if !strings.Contains(buf.String(), "uploaded 8 bytes...") {
		t.Errorf("log = %q, want running total of 8 bytes", buf.String())
	}
	if p.lastPrint.IsZero() {
		t.Error("lastPrint not advanced after logging")
	}
}
