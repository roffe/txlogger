//go:build !android

package widgets

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/godbus/dbus/v5"
	"github.com/roffe/browse"
)

// Environment of the fake dialog child: dialog re-executes the test binary,
// which then answers from TestMain instead of running the tests.
const (
	envFakeReply   = "TXL_TEST_DIALOG_REPLY"   // printed on stdout as the child's answer
	envFakeRequest = "TXL_TEST_DIALOG_REQUEST" // file the child copies its stdin to
	envFakeExit    = "TXL_TEST_DIALOG_EXIT"    // exit status of the child
)

// TestMain lets the test binary act as the file dialog child: a fake one that
// replays a canned answer when envFakeReply is set, otherwise the real
// RunFileChild.
func TestMain(m *testing.M) {
	if os.Getenv(dialogChildEnv) == "1" {
		if reply, ok := os.LookupEnv(envFakeReply); ok {
			os.Exit(fakeDialogChild(reply))
		}
		if RunFileChild() {
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func fakeDialogChild(reply string) int {
	req, _ := io.ReadAll(os.Stdin)
	if path := os.Getenv(envFakeRequest); path != "" {
		if err := os.WriteFile(path, req, 0o600); err != nil {
			return 99
		}
	}
	os.Stdout.WriteString(reply)
	code, _ := strconv.Atoi(os.Getenv(envFakeExit))
	return code
}

// quickChildExit stops a -race child from idling for a second when it exits.
func quickChildExit(t *testing.T) {
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
}

// fakeChild makes the next dialog children print reply and returns a function
// that reads back the last request a child received.
func fakeChild(t *testing.T, reply string) func() dialogRequest {
	t.Helper()
	quickChildExit(t)
	reqFile := filepath.Join(t.TempDir(), "request.json")
	t.Setenv(envFakeReply, reply)
	t.Setenv(envFakeRequest, reqFile)
	return func() dialogRequest {
		t.Helper()
		b, err := os.ReadFile(reqFile)
		if err != nil {
			t.Fatalf("child got no request: %v", err)
		}
		var req dialogRequest
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("child request %q: %v", b, err)
		}
		return req
	}
}

func replyJSON(t *testing.T, r dialogResponse) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// noPortal points the session bus at a socket that does not exist, so browse
// fails with ErrUnavailable instead of showing a real dialog.
func noPortal(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+filepath.Join(t.TempDir(), "no-bus"))
}

// await returns the next value from ch, failing the test if the dialog
// goroutine never delivers one.
func await[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(30 * time.Second):
		t.Fatal("callback was not called")
		panic("unreachable")
	}
}

// syncBuffer is a log destination that can be written from any goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf
}

// dialog sends the operation and options to the child as JSON and turns its
// answer back into paths, ErrCancelled or an error carrying the child's
// message; a child that fails or answers garbage is reported as such.
func TestDialogChildAnswers(t *testing.T) {
	opts := browse.Options{
		Title:   "Open Binary file",
		Dir:     "/tmp/bins",
		Name:    "suggested.bin",
		Filters: []browse.Filter{{Name: "Binary file", Extensions: []string{"bin", "BIN"}}},
	}
	for _, tc := range []struct {
		name      string
		reply     string
		exit      string
		paths     []string
		cancelled bool
		errText   string // exact error, or prefix when it ends in "*"
	}{
		{name: "paths", reply: `{"Paths":["/a/one.bin","/b/two.bin"]}`, paths: []string{"/a/one.bin", "/b/two.bin"}},
		{name: "cancelled", reply: `{"Cancelled":true,"Err":"browse: cancelled"}`, cancelled: true, errText: browse.ErrCancelled.Error()},
		{name: "error", reply: `{"Err":"browse: no native file dialog available: no bus"}`, errText: "browse: no native file dialog available: no bus"},
		{name: "garbage", reply: "not json", errText: "file dialog child: *"},
		{name: "child failed", reply: "", exit: "3", errText: "file dialog child: exit status 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := fakeChild(t, tc.reply)
			t.Setenv(envFakeExit, tc.exit)

			paths, err := dialog(opOpenFile, opts)
			if !reflect.DeepEqual(paths, tc.paths) {
				t.Errorf("paths %q, want %q", paths, tc.paths)
			}
			switch {
			case tc.errText == "" && err != nil:
				t.Errorf("unexpected error %v", err)
			case tc.errText != "" && err == nil:
				t.Errorf("no error, want %q", tc.errText)
			case strings.HasSuffix(tc.errText, "*") && err != nil:
				if !strings.HasPrefix(err.Error(), strings.TrimSuffix(tc.errText, "*")) {
					t.Errorf("error %q, want prefix %q", err, strings.TrimSuffix(tc.errText, "*"))
				}
			case err != nil && err.Error() != tc.errText:
				t.Errorf("error %q, want %q", err, tc.errText)
			}
			if got := errors.Is(err, browse.ErrCancelled); got != tc.cancelled {
				t.Errorf("errors.Is(err, ErrCancelled) = %v, want %v", got, tc.cancelled)
			}

			want := dialogRequest{Op: opOpenFile, Options: opts}
			if got := request(); !reflect.DeepEqual(got, want) {
				t.Errorf("child got request %+v, want %+v", got, want)
			}
		})
	}
}

// The real child re-executed from dialog runs the dialog and reports its
// failures back to the parent: an unknown operation, or no portal to show
// the dialog with. Neither counts as the user cancelling.
func TestDialogRealChildReportsErrors(t *testing.T) {
	noPortal(t)
	quickChildExit(t)
	for _, tc := range []struct {
		op   string
		want string
	}{
		{op: "bogus", want: `unknown file dialog operation "bogus"`},
		{op: opOpenFile, want: browse.ErrUnavailable.Error()},
		{op: opSaveFile, want: browse.ErrUnavailable.Error()},
	} {
		paths, err := dialog(tc.op, browse.Options{Title: "t"})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want it to contain %q", tc.op, err, tc.want)
		}
		if errors.Is(err, browse.ErrCancelled) {
			t.Errorf("%s: error %v reported as a cancel", tc.op, err)
		}
		if paths != nil {
			t.Errorf("%s: paths %q, want none", tc.op, paths)
		}
	}
}

// showDialog dispatches every known operation to browse, whose failure it
// returns unchanged.
func TestShowDialogWithoutPortal(t *testing.T) {
	noPortal(t)
	for _, op := range []string{opOpenFile, opOpenFiles, opSaveFile, opFolder} {
		paths, err := showDialog(op, browse.Options{Title: "t", Name: "n.bin"})
		if !errors.Is(err, browse.ErrUnavailable) {
			t.Errorf("%s: error %v, want ErrUnavailable", op, err)
		}
		if paths != nil {
			t.Errorf("%s: paths %q, want none", op, paths)
		}
	}
}

// pick reports whether the user chose anything; a cancel is silent while any
// other dialog failure is logged.
func TestPick(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply dialogResponse
		ok    bool
		log   string
	}{
		{name: "chosen", reply: dialogResponse{Paths: []string{"/x/y.csv"}}, ok: true},
		{name: "cancelled", reply: dialogResponse{Cancelled: true, Err: "browse: cancelled"}},
		{name: "failed", reply: dialogResponse{Err: "boom"}, log: "File dialog: boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLog(t)
			fakeChild(t, replyJSON(t, tc.reply))

			paths, ok := pick(opOpenFile, browse.Options{})
			if ok != tc.ok {
				t.Errorf("ok = %v, want %v", ok, tc.ok)
			}
			var want []string
			if tc.ok {
				want = tc.reply.Paths
			}
			if !reflect.DeepEqual(paths, want) {
				t.Errorf("paths %q, want %q", paths, want)
			}
			got := logs.String()
			if tc.log == "" && got != "" {
				t.Errorf("logged %q, want nothing", got)
			}
			if !strings.Contains(got, tc.log) {
				t.Errorf("logged %q, want %q", got, tc.log)
			}
		})
	}
}

// SelectFolder asks for a folder and passes the chosen path to the callback.
func TestSelectFolder(t *testing.T) {
	test.NewTempApp(t)
	dir := t.TempDir()
	request := fakeChild(t, replyJSON(t, dialogResponse{Paths: []string{dir}}))

	got := make(chan string, 1)
	SelectFolder(func(s string) { got <- s })
	if s := await(t, got); s != dir {
		t.Errorf("callback got %q, want %q", s, dir)
	}
	want := dialogRequest{Op: opFolder, Options: browse.Options{Title: "Select log folder"}}
	if req := request(); !reflect.DeepEqual(req, want) {
		t.Errorf("request %+v, want %+v", req, want)
	}
}

// SelectFile filters the dialog to the described extensions and hands the
// callback an open reader for the chosen file.
func TestSelectFile(t *testing.T) {
	test.NewTempApp(t)
	path := filepath.Join(t.TempDir(), "drive.t7l")
	if err := os.WriteFile(path, []byte("log data"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := fakeChild(t, replyJSON(t, dialogResponse{Paths: []string{path}}))

	got := make(chan string, 1)
	SelectFile(func(r fyne.URIReadCloser) {
		defer r.Close()
		b, err := io.ReadAll(r)
		if err != nil {
			got <- "read error: " + err.Error()
			return
		}
		got <- r.URI().Path() + "=" + string(b)
	}, "Log file", "t7l", "csv")

	if s, want := await(t, got), path+"=log data"; s != want {
		t.Errorf("callback got %q, want %q", s, want)
	}
	want := dialogRequest{Op: opOpenFile, Options: browse.Options{
		Title:   "Open Log file",
		Filters: []browse.Filter{{Name: "Log file", Extensions: []string{"t7l", "csv"}}},
	}}
	if req := request(); !reflect.DeepEqual(req, want) {
		t.Errorf("request %+v, want %+v", req, want)
	}
}

// SelectFiles opens every chosen file it can, in the order chosen, skipping
// and logging the ones that cannot be read.
func TestSelectFiles(t *testing.T) {
	test.NewTempApp(t)
	logs := captureLog(t)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.csv"), filepath.Join(dir, "b.csv")
	for path, data := range map[string]string{a: "A", b: "B"} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	missing := filepath.Join(dir, "gone.csv")
	request := fakeChild(t, replyJSON(t, dialogResponse{Paths: []string{a, missing, b}}))

	got := make(chan []string, 1)
	SelectFiles(func(rc []fyne.URIReadCloser) {
		var files []string
		for _, r := range rc {
			data, _ := io.ReadAll(r)
			r.Close()
			files = append(files, r.URI().Path()+"="+string(data))
		}
		got <- files
	}, "Log files", "csv")

	if files, want := await(t, got), []string{a + "=A", b + "=B"}; !reflect.DeepEqual(files, want) {
		t.Errorf("callback got %q, want %q", files, want)
	}
	// The log is written before the callback is called.
	if s := logs.String(); !strings.Contains(s, "Error reading file:") || !strings.Contains(s, "gone.csv") {
		t.Errorf("logged %q, want the unreadable gone.csv reported", s)
	}
	want := dialogRequest{Op: opOpenFiles, Options: browse.Options{
		Title:   "Open Log files",
		Filters: []browse.Filter{{Name: "Log files", Extensions: []string{"csv"}}},
	}}
	if req := request(); !reflect.DeepEqual(req, want) {
		t.Errorf("request %+v, want %+v", req, want)
	}
}

// SaveFile suggests the file name, offers the one extension and passes the
// chosen path to the callback.
func TestSaveFile(t *testing.T) {
	test.NewTempApp(t)
	dst := filepath.Join(t.TempDir(), "out.bin")
	request := fakeChild(t, replyJSON(t, dialogResponse{Paths: []string{dst}}))

	got := make(chan string, 1)
	SaveFile(func(s string) { got <- s }, "Binary file", "bin", "tuned.bin")
	if s := await(t, got); s != dst {
		t.Errorf("callback got %q, want %q", s, dst)
	}
	want := dialogRequest{Op: opSaveFile, Options: browse.Options{
		Title:   "Save Binary file",
		Name:    "tuned.bin",
		Filters: []browse.Filter{{Name: "Binary file", Extensions: []string{"bin"}}},
	}}
	if req := request(); !reflect.DeepEqual(req, want) {
		t.Errorf("request %+v, want %+v", req, want)
	}
}

// runChild runs RunFileChild in this process with req on stdin, and returns
// what it reported and the response it wrote to stdout.
func runChild(t *testing.T, req dialogRequest) (bool, dialogResponse) {
	t.Helper()
	dir := t.TempDir()
	in, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	inPath, outPath := filepath.Join(dir, "stdin"), filepath.Join(dir, "stdout")
	if err := os.WriteFile(inPath, in, 0o600); err != nil {
		t.Fatal(err)
	}
	stdin, err := os.Open(inPath)
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()

	prevIn, prevOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = stdin, stdout
	handled := RunFileChild()
	os.Stdin, os.Stdout = prevIn, prevOut

	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var resp dialogResponse
	if handled {
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("response %q: %v", out, err)
		}
	} else if len(out) != 0 {
		t.Errorf("wrote %q though it was not the child", out)
	}
	return handled, resp
}

// RunFileChild does nothing unless started as the dialog child; as the child
// it answers the request on stdin with the dialog's outcome on stdout.
func TestRunFileChild(t *testing.T) {
	noPortal(t)

	t.Setenv(dialogChildEnv, "")
	if handled, _ := runChild(t, dialogRequest{Op: opOpenFile}); handled {
		t.Fatal("RunFileChild handled a request without the child environment")
	}

	t.Setenv(dialogChildEnv, "1")
	handled, resp := runChild(t, dialogRequest{Op: "bogus"})
	if !handled {
		t.Fatal("RunFileChild did not handle the request as the child")
	}
	want := dialogResponse{Err: `unknown file dialog operation "bogus"`}
	if !reflect.DeepEqual(resp, want) {
		t.Errorf("response %+v, want %+v", resp, want)
	}

	_, resp = runChild(t, dialogRequest{Op: opFolder, Options: browse.Options{Title: "t"}})
	if resp.Cancelled || resp.Paths != nil || !strings.Contains(resp.Err, browse.ErrUnavailable.Error()) {
		t.Errorf("response %+v, want an ErrUnavailable failure that is not a cancel", resp)
	}
}

// A dialog child that cannot decode its request exits with an error instead
// of answering, which dialog then reports as a failed child.
func TestRunFileChildBadRequest(t *testing.T) {
	quickChildExit(t)
	cmd := exec.Command("/proc/self/exe")
	cmd.Env = append(os.Environ(), dialogChildEnv+"=1")
	cmd.Stdin = strings.NewReader("{not json")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Errorf("child exited with %v, want exit status 1", err)
	}
	if len(out) != 0 {
		t.Errorf("child answered %q, want nothing", out)
	}
	if !strings.Contains(stderr.String(), "error decoding request") {
		t.Errorf("child stderr %q, want the decode error", stderr.String())
	}
}

// logLines sends everything logged from now on to the returned channel, one
// value per log call.
func logLines(t *testing.T) <-chan string {
	t.Helper()
	ch := make(chan string, 16)
	prev := log.Writer()
	log.SetOutput(chanWriter(ch))
	t.Cleanup(func() { log.SetOutput(prev) })
	return ch
}

type chanWriter chan string

func (w chanWriter) Write(p []byte) (int, error) {
	w <- string(p)
	return len(p), nil
}

// A dialog that fails, or a chosen file that cannot be opened, is logged and
// the callback is not called. The cancel path logs nothing and has no other
// output, so it is not covered here.
func TestSelectFailuresAreLogged(t *testing.T) {
	missing := func(t *testing.T, names ...string) []string {
		dir := t.TempDir()
		var paths []string
		for _, n := range names {
			paths = append(paths, filepath.Join(dir, n))
		}
		return paths
	}
	for _, tc := range []struct {
		name   string
		paths  func(t *testing.T) []string // the dialog's answer, nil for a failed dialog
		run    func(called chan<- struct{})
		logged []string // one entry per expected log call
	}{
		{
			name:   "folder dialog failed",
			run:    func(c chan<- struct{}) { SelectFolder(func(string) { c <- struct{}{} }) },
			logged: []string{"File dialog: boom"},
		},
		{
			name:   "file dialog failed",
			run:    func(c chan<- struct{}) { SelectFile(func(fyne.URIReadCloser) { c <- struct{}{} }, "Bin", "bin") },
			logged: []string{"File dialog: boom"},
		},
		{
			name:   "files dialog failed",
			run:    func(c chan<- struct{}) { SelectFiles(func([]fyne.URIReadCloser) { c <- struct{}{} }, "Bin", "bin") },
			logged: []string{"File dialog: boom"},
		},
		{
			name:   "save dialog failed",
			run:    func(c chan<- struct{}) { SaveFile(func(string) { c <- struct{}{} }, "Bin", "bin", "x.bin") },
			logged: []string{"File dialog: boom"},
		},
		{
			name:   "file unreadable",
			paths:  func(t *testing.T) []string { return missing(t, "gone.bin") },
			run:    func(c chan<- struct{}) { SelectFile(func(fyne.URIReadCloser) { c <- struct{}{} }, "Bin", "bin") },
			logged: []string{"Error reading file: "},
		},
		{
			name:   "no file readable",
			paths:  func(t *testing.T) []string { return missing(t, "a.bin", "b.bin") },
			run:    func(c chan<- struct{}) { SelectFiles(func([]fyne.URIReadCloser) { c <- struct{}{} }, "Bin", "bin") },
			logged: []string{"Error reading file: ", "Error reading file: "},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			test.NewTempApp(t)
			reply := dialogResponse{Err: "boom"}
			if tc.paths != nil {
				reply = dialogResponse{Paths: tc.paths(t)}
			}
			fakeChild(t, replyJSON(t, reply))
			logs := logLines(t)

			called := make(chan struct{}, 1)
			tc.run(called)
			for _, want := range tc.logged {
				if got := await(t, logs); !strings.Contains(got, want) {
					t.Errorf("logged %q, want %q", got, want)
				}
			}
			// The goroutine returns right after its last log call. A callback
			// made after that call could slip past this check, but never
			// makes it fail spuriously.
			select {
			case <-called:
				t.Error("callback called after a failure")
			default:
			}
		})
	}
}

// startPortal runs a private session bus with a fake XDG desktop portal on it
// that answers every file dialog with uris, and points this process and the
// dialog children it starts at that bus.
func startPortal(t *testing.T, uris ...string) *fakePortal {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is not installed")
	}
	dir := t.TempDir()
	config := filepath.Join(dir, "bus.conf")
	conf := `<busconfig>
	<type>session</type>
	<listen>unix:dir=` + dir + `</listen>
	<policy context="default">
		<allow send_destination="*" eavesdrop="true"/>
		<allow eavesdrop="true"/>
		<allow own="*"/>
	</policy>
</busconfig>`
	if err := os.WriteFile(config, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(daemon, "--config-file="+config, "--nofork", "--print-address")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	address, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("dbus-daemon address: %v", err)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", strings.TrimSpace(address))

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	p := &fakePortal{conn: conn, uris: uris}
	if err := conn.Export(p, "/org/freedesktop/portal/desktop", "org.freedesktop.portal.FileChooser"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.RequestName("org.freedesktop.portal.Desktop", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	return p
}

// portalCall is what one file dialog asked the fake portal for.
type portalCall struct {
	Method      string
	Title       string
	Multiple    bool
	Directory   bool
	CurrentName string
}

type fakePortal struct {
	conn *dbus.Conn
	uris []string

	mu    sync.Mutex
	calls []portalCall
}

func (p *fakePortal) OpenFile(sender dbus.Sender, parent, title string, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	return p.answer("OpenFile", sender, title, options)
}

func (p *fakePortal) SaveFile(sender dbus.Sender, parent, title string, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	return p.answer("SaveFile", sender, title, options)
}

// answer records the call and accepts the dialog with p.uris, the way the
// real portal does: with a Response signal on the request object named after
// the caller and its handle token.
func (p *fakePortal) answer(method string, sender dbus.Sender, title string, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	call := portalCall{Method: method, Title: title}
	call.Multiple, _ = options["multiple"].Value().(bool)
	call.Directory, _ = options["directory"].Value().(bool)
	call.CurrentName, _ = options["current_name"].Value().(string)
	p.mu.Lock()
	p.calls = append(p.calls, call)
	p.mu.Unlock()

	token, _ := options["handle_token"].Value().(string)
	caller := strings.ReplaceAll(strings.TrimPrefix(string(sender), ":"), ".", "_")
	request := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + caller + "/" + token)
	results := map[string]dbus.Variant{"uris": dbus.MakeVariant(p.uris)}
	if err := p.conn.Emit(request, "org.freedesktop.portal.Request.Response", uint32(0), results); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return request, nil
}

// takeCalls returns the dialogs asked for since the last call.
func (p *fakePortal) takeCalls() []portalCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	calls := p.calls
	p.calls = nil
	return calls
}

// showDialog maps each operation to its portal dialog: a single file, several
// files, a file to save under the suggested name, or a folder. The chosen
// paths come back unchanged.
func TestShowDialogPortal(t *testing.T) {
	p := startPortal(t, "file:///logs/a.t7l", "file:///logs/b%20c.t7l")
	want := []string{"/logs/a.t7l", "/logs/b c.t7l"}
	for _, tc := range []struct {
		op    string
		call  portalCall
		paths []string
	}{
		{op: opOpenFile, call: portalCall{Method: "OpenFile", Title: "t"}, paths: want[:1]},
		{op: opOpenFiles, call: portalCall{Method: "OpenFile", Title: "t", Multiple: true}, paths: want},
		{op: opSaveFile, call: portalCall{Method: "SaveFile", Title: "t", CurrentName: "n.bin"}, paths: want[:1]},
		{op: opFolder, call: portalCall{Method: "OpenFile", Title: "t", Directory: true}, paths: want[:1]},
	} {
		paths, err := showDialog(tc.op, browse.Options{Title: "t", Name: "n.bin"})
		if err != nil {
			t.Errorf("%s: %v", tc.op, err)
		}
		if !reflect.DeepEqual(paths, tc.paths) {
			t.Errorf("%s: paths %q, want %q", tc.op, paths, tc.paths)
		}
		if calls := p.takeCalls(); !reflect.DeepEqual(calls, []portalCall{tc.call}) {
			t.Errorf("%s: portal asked %+v, want %+v", tc.op, calls, tc.call)
		}
	}
}

// End to end through the real dialog child: SelectFiles starts it, the child
// asks the portal for several files, and the callback gets readers for the
// files the user picked.
func TestSelectFilesThroughPortal(t *testing.T) {
	test.NewTempApp(t)
	quickChildExit(t)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.csv"), filepath.Join(dir, "b.csv")
	for path, data := range map[string]string{a: "A", b: "B"} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p := startPortal(t, "file://"+a, "file://"+b)

	got := make(chan []string, 1)
	SelectFiles(func(rc []fyne.URIReadCloser) {
		var files []string
		for _, r := range rc {
			data, _ := io.ReadAll(r)
			r.Close()
			files = append(files, r.URI().Path()+"="+string(data))
		}
		got <- files
	}, "Log files", "csv")

	if files, want := await(t, got), []string{a + "=A", b + "=B"}; !reflect.DeepEqual(files, want) {
		t.Errorf("callback got %q, want %q", files, want)
	}
	want := []portalCall{{Method: "OpenFile", Title: "Open Log files", Multiple: true}}
	if calls := p.takeCalls(); !reflect.DeepEqual(calls, want) {
		t.Errorf("portal asked %+v, want %+v", calls, want)
	}
}
