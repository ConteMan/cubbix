package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/ConteMan/cubbix/internal/cli"
)

// cliTestApp is a panel test app whose hints are recorded instead of shown and
// whose clipboard refuses to be read.
func cliTestApp(t *testing.T) *application {
	t.Helper()
	a := panelTestApp(t)
	a.feedback.mode = func() string { return "hint" }
	a.feedback.cursorDisplay = func() mygo.Display { return mygo.Display{} }
	a.jsonReadClipboard = func() string {
		t.Fatal("the CLI read the clipboard")
		return ""
	}
	return a
}

// shortDir is a temp directory short enough for sun_path.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cbs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// testServer starts a CLI server on a short path with the app's own apply.
func testServer(t *testing.T, a *application) (*cliServer, string) {
	t.Helper()
	dir := shortDir(t)
	s := &cliServer{
		path:     filepath.Join(dir, cli.SocketName),
		slots:    make(chan struct{}, cli.MaxConcurrent),
		verify:   verifySameUser,
		dispatch: func(run func()) { run() },
		logf:     func(string, ...any) {},
	}
	s.apply = a.applyCLI
	if err := s.listen(); err != nil {
		t.Fatal(err)
	}
	go s.serve()
	t.Cleanup(s.stop)
	return s, s.path
}

func dial(t *testing.T, path string) *cli.Client {
	t.Helper()
	client, err := cli.Dial(path, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func do(t *testing.T, client *cli.Client, req cli.Request, body []byte) cli.Result {
	t.Helper()
	if req.V == 0 {
		req.V = cli.Protocol
	}
	if req.ID == "" {
		req.ID = "test"
	}
	result, err := client.Do(req, body, 3*time.Second)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return result
}

func TestCLIServerPermissions(t *testing.T) {
	a := cliTestApp(t)
	s, path := testServer(t, a)
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v %v", info.Mode(), err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v %v", info.Mode(), err)
	}
	result := do(t, dial(t, path), cli.Request{Op: cli.OpStatus}, nil)
	if !result.OK || result.State == nil {
		t.Fatalf("status = %+v", result)
	}
	if len(result.Lines) != 3 {
		t.Fatalf("lines = %v", result.Lines)
	}
	s.stop()
}

func TestCLIServerLineLimit(t *testing.T) {
	_, path := testServer(t, cliTestApp(t))
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(strings.Repeat("x", cli.MaxLine+1) + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 16)
	if n, _ := conn.Read(buf); n != 0 {
		t.Fatalf("the server answered an over-long line: %q", buf[:n])
	}
}

// rawRequest writes a request line and an optional body, closing the write side
// so a short body ends the read instead of waiting.
func rawRequest(t *testing.T, path string, req cli.Request, body []byte) cli.Response {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	data, err := cli.EncodeLine(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(append(data, body...)); err != nil {
		t.Fatal(err)
	}
	conn.(*net.UnixConn).CloseWrite()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var response cli.Response
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatalf("%q: %v", line, err)
	}
	return response
}

func TestCLIServerShortBody(t *testing.T) {
	a := cliTestApp(t)
	_, path := testServer(t, a)
	response := rawRequest(t, path, cli.Request{V: cli.Protocol, ID: "a", Op: cli.OpRun,
		Action: cli.ActionJSONNew, Bytes: 5}, []byte("abc"))
	if response.OK || response.Code != cli.CodeBadRequest {
		t.Fatalf("short body = %+v", response)
	}
	if a.awake.snapshot().Active {
		t.Fatal("a rejected request changed the state")
	}
}

func TestCLIServerOversizeBody(t *testing.T) {
	a := cliTestApp(t)
	_, path := testServer(t, a)
	response := rawRequest(t, path, cli.Request{V: cli.Protocol, ID: "a", Op: cli.OpRun,
		Action: cli.ActionJSONNew, Bytes: cli.MaxPayload + 1}, nil)
	if response.OK || response.Code != cli.CodeBadRequest {
		t.Fatalf("oversize = %+v", response)
	}
}

func TestCLIServerUntrustedPeer(t *testing.T) {
	dir := shortDir(t)
	s := &cliServer{
		path:     filepath.Join(dir, cli.SocketName),
		slots:    make(chan struct{}, cli.MaxConcurrent),
		verify:   func(*net.UnixConn) error { return errors.New("not our user") },
		dispatch: func(run func()) { run() },
		logf:     func(string, ...any) {},
	}
	s.apply = func(cli.Request, []byte) cli.Result {
		t.Error("an untrusted peer reached apply")
		return cli.Failure(cli.CodeInternal)
	}
	if err := s.listen(); err != nil {
		t.Fatal(err)
	}
	defer s.stop()
	go s.serve()
	conn, err := net.Dial("unix", s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// The verified rejection may close before the first write.
	_, _ = conn.Write([]byte("{\"v\":1,\"id\":\"a\",\"op\":\"status\"}\n"))
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 8)
	if n, _ := conn.Read(buf); n != 0 {
		t.Fatalf("an untrusted peer was answered: %q", buf[:n])
	}
}

func TestCLIServerBusyBeforeBody(t *testing.T) {
	dir := shortDir(t)
	started := make(chan struct{}, cli.MaxConcurrent)
	release := make(chan struct{})
	var applied int32
	var mu sync.Mutex
	s := &cliServer{
		path:     filepath.Join(dir, cli.SocketName),
		slots:    make(chan struct{}, cli.MaxConcurrent),
		verify:   verifySameUser,
		logf:     func(string, ...any) {},
		dispatch: func(run func()) { started <- struct{}{}; <-release; run() },
	}
	s.apply = func(cli.Request, []byte) cli.Result {
		mu.Lock()
		applied++
		mu.Unlock()
		return cli.NewResult(nil)
	}
	if err := s.listen(); err != nil {
		t.Fatal(err)
	}
	defer s.stop()
	go s.serve()

	var conns []net.Conn
	for i := 0; i < cli.MaxConcurrent; i++ {
		conn, err := net.Dial("unix", s.path)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		conns = append(conns, conn)
		if _, err := conn.Write([]byte("{\"v\":1,\"id\":\"a\",\"op\":\"status\"}\n")); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < cli.MaxConcurrent; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("the server did not take all its slots")
		}
	}
	// The ninth connection is answered busy without its request being read: it
	// never reaches apply, and its body is not consumed.
	ninth, err := net.Dial("unix", s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer ninth.Close()
	// The server may answer busy and close before this write lands, so a
	// failed write is expected; the busy line is still there to read.
	_, _ = ninth.Write([]byte("{\"v\":1,\"id\":\"n\",\"op\":\"run\",\"action\":\"json.new\",\"bytes\":4}" + "\n" + "body"))
	_ = ninth.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(ninth).ReadBytes('\n')
	if err != nil {
		t.Fatalf("busy read: %v", err)
	}
	var response cli.Response
	if err := json.Unmarshal(line, &response); err != nil || response.Code != cli.CodeBusy {
		t.Fatalf("ninth = %q", line)
	}
	mu.Lock()
	if applied != 0 {
		t.Fatalf("apply ran %d times while all slots were taken", applied)
	}
	mu.Unlock()
	close(release)
}

func TestCLIServerQuitting(t *testing.T) {
	a := cliTestApp(t)
	dir := shortDir(t)
	s := &cliServer{
		path:     filepath.Join(dir, cli.SocketName),
		slots:    make(chan struct{}, cli.MaxConcurrent),
		verify:   verifySameUser,
		dispatch: func(func()) {}, // the event loop is gone: RunOnMain drops it
		logf:     func(string, ...any) {},
	}
	s.apply = a.applyCLI
	if err := s.listen(); err != nil {
		t.Fatal(err)
	}
	defer s.stop()
	go s.serve()
	result := do(t, dial(t, s.path), cli.Request{Op: cli.OpStatus}, nil)
	if result.OK || result.Code != cli.CodeQuitting {
		t.Fatalf("quitting = %+v", result)
	}
}

// A leftover socket file is taken over; one another instance serves is left alone.
func TestCLIServerTakeover(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, cli.SocketName)
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	stale.Close()

	a := cliTestApp(t)
	s := &cliServer{
		path:     path,
		slots:    make(chan struct{}, cli.MaxConcurrent),
		verify:   verifySameUser,
		dispatch: func(run func()) { run() },
		logf:     func(string, ...any) {},
	}
	s.apply = a.applyCLI
	if err := s.listen(); err != nil {
		t.Fatalf("leftover socket was not taken over: %v", err)
	}
	defer s.stop()
	go s.serve()
	if result := do(t, dial(t, path), cli.Request{Op: cli.OpStatus}, nil); !result.OK {
		t.Fatalf("status after takeover = %+v", result)
	}

	second := &cliServer{path: path, slots: make(chan struct{}, 1), logf: func(string, ...any) {}}
	if err := second.listen(); err == nil {
		second.stop()
		t.Fatal("a second server bound the same socket")
	}
}

// Stopping must not remove a socket another instance bound meanwhile.
func TestCLIServerStopKeepsForeignInode(t *testing.T) {
	a := cliTestApp(t)
	s, path := testServer(t, a)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	other, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	s.stop()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stop removed someone else's socket: %v", err)
	}
}

func TestCLIServerStopRemovesOwnSocket(t *testing.T) {
	a := cliTestApp(t)
	s, path := testServer(t, a)
	s.stop()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket still there: %v", err)
	}
}

func TestApplyCLIAwake(t *testing.T) {
	a := cliTestApp(t)
	t.Cleanup(func() { a.awake.stop() })
	before := a.config
	result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "a", Op: cli.OpRun, Action: cli.ActionAwake, Arg: "1h30m"}, nil)
	if !result.OK || result.State == nil {
		t.Fatalf("awake = %+v", result)
	}
	if !a.awake.snapshot().Active || a.awake.snapshot().Session.Minutes != 90 {
		t.Fatalf("session = %+v", a.awake.snapshot())
	}
	if !strings.Contains(result.Message, "90") {
		t.Fatalf("message = %q", result.Message)
	}
	after := a.config
	if after.AwakeMinutes != before.AwakeMinutes || after.AwakeUntil != before.AwakeUntil || after.AwakeMode != before.AwakeMode {
		t.Fatalf("the CLI wrote the remembered setting: %+v", after)
	}
	if a.lidAuthorize {
		t.Fatal("the CLI allowed authorization")
	}
}

func TestApplyCLIAwakeStopAndMessage(t *testing.T) {
	a := cliTestApp(t)
	t.Cleanup(func() { a.awake.stop() })
	if result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "a", Op: cli.OpRun, Action: cli.ActionAwake, Arg: "30m"}, nil); !result.OK {
		t.Fatalf("start = %+v", result)
	}
	result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "b", Op: cli.OpRun, Action: cli.ActionAwakeStop}, nil)
	if !result.OK || a.awake.snapshot().Active {
		t.Fatalf("stop = %+v", result)
	}
	if result.Message != a.text(uiLocale(), "ToastAwakeOff") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestApplyCLIJSONNeverReadsClipboard(t *testing.T) {
	a := cliTestApp(t)
	var opened []string
	a.paletteOpenJSON = func(text string) { opened = append(opened, text) }
	result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "a", Op: cli.OpRun, Action: cli.ActionJSONNew, Bytes: 7},
		[]byte(`{"a":1}`))
	if !result.OK {
		t.Fatalf("json = %+v", result)
	}
	if len(opened) != 1 || opened[0] != `{"a":1}` {
		t.Fatalf("opened = %v", opened)
	}
	a.paletteOpenJSON = nil
}

func TestApplyCLIPaletteAndSettings(t *testing.T) {
	a := cliTestApp(t)
	if result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "a", Op: cli.OpRun, Action: cli.ActionPalette}, nil); !result.OK {
		t.Fatalf("palette = %+v", result)
	}
	if !a.paletteVisible() {
		t.Fatal("the palette is not visible")
	}
	a.hidePalette()
	a.page = 0
	if result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "b", Op: cli.OpRun, Action: cli.ActionSettings, Page: "shortcuts"}, nil); !result.OK {
		t.Fatalf("settings = %+v", result)
	}
	if a.page != 3 {
		t.Fatalf("page = %d", a.page)
	}
}

func TestApplyCLIDisabled(t *testing.T) {
	a := cliTestApp(t)
	a.config.AwakeEnabled = false
	result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "a", Op: cli.OpRun, Action: cli.ActionAwake, Arg: "30m"}, nil)
	if result.OK || result.Code != cli.CodeDisabled {
		t.Fatalf("disabled = %+v", result)
	}
	if a.awake.snapshot().Active {
		t.Fatal("a disabled tile started")
	}
	a.config.AwakeEnabled = true
	a.config.JSONEnabled = false
	if result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "b", Op: cli.OpRun, Action: cli.ActionJSONNew}, nil); result.Code != cli.CodeDisabled {
		t.Fatalf("json disabled = %+v", result)
	}
	a.config.JSONEnabled = true
	if result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "c", Op: cli.OpRun, Action: "eval"}, nil); result.Code != cli.CodeBadRequest {
		t.Fatalf("unknown action = %+v", result)
	}
}

// The toggle takes the shortcut's path, so a remembered lid preference starts a
// lid session. The URL branch would have answered LidAuthNeeded, because the
// request carries no minutes.
func TestApplyCLIToggleUsesShortcutPath(t *testing.T) {
	a, _, _, _, _ := lidTestApp(t)
	a.feedback.mode = func() string { return "hint" }
	a.feedback.cursorDisplay = func() mygo.Display { return mygo.Display{} }
	a.config.AwakeLid = true
	result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "a", Op: cli.OpRun, Action: cli.ActionAwakeToggle}, nil)
	if !result.OK {
		t.Fatalf("toggle = %+v", result)
	}
	if !a.awake.snapshot().Active || !a.awake.snapshot().Session.Lid {
		t.Fatalf("the lid preference was dropped: %+v", a.awake.snapshot())
	}
	if a.lidURL {
		t.Fatal("runCLIToggle left lidURL set")
	}
}

// The URL toggle keeps its own behavior; this pins it so nobody unifies it.
func TestURLToggleStillRefusesRememberedLid(t *testing.T) {
	a := cliTestApp(t)
	a.config.AwakeLid = true
	request, ok := parsePaletteURL("cubbix://awake/toggle", true, true, jsonFileLimit)
	if !ok {
		t.Fatal("the URL did not parse")
	}
	if request.Arg != "" || request.Follow != 0 {
		t.Fatalf("parsePaletteURL set the CLI-only fields: %+v", request)
	}
	if err := a.runURL(request); err == nil || err.Error() != "LidAuthNeeded" {
		t.Fatalf("URL toggle = %v", err)
	}
}

func TestAwakeLidGate(t *testing.T) {
	a := cliTestApp(t)
	now := time.Now()
	// No controller: unsupported, named as such instead of an empty error.
	if err := a.awakeLidGate(paletteURLRequest{HasLid: true, Lid: true}, awakeSession{Lid: true}, "cli"); err == nil || err.Error() != "LidUnsupported" {
		t.Fatalf("no controller = %v", err)
	}
	// A URL without minutes keeps the old answer.
	if err := a.awakeLidGate(paletteURLRequest{}, awakeSession{Lid: true}, "url"); err == nil || err.Error() != "LidAuthNeeded" {
		t.Fatalf("url gate = %v", err)
	}
	// Nothing wanted: no gate at all.
	if err := a.awakeLidGate(paletteURLRequest{}, awakeSession{}, "cli"); err != nil {
		t.Fatalf("no lid = %v", err)
	}

	b := cliTestApp(t)
	b.lid = testLidController(t, svcEnabled)
	if err := b.awakeLidGate(paletteURLRequest{HasLid: true, Lid: true}, awakeSession{Lid: true, Minutes: 90}, "cli"); err != nil {
		t.Fatalf("installed lid = %v", err)
	}
	if err := b.awakeLidGate(paletteURLRequest{HasLid: true, Lid: true}, awakeSession{Lid: true}, "cli"); err == nil || err.Error() != "LidNeedsMinutes" {
		t.Fatalf("no duration = %v", err)
	}
	if err := b.awakeLidGate(paletteURLRequest{HasLid: true, Lid: true}, awakeSession{Lid: true, Minutes: 0}, "cli"); err == nil || err.Error() != "LidNeedsMinutes" {
		t.Fatalf("unlimited = %v", err)
	}
	if err := b.awakeLidGate(paletteURLRequest{HasLid: true, Lid: true}, awakeSession{Lid: true, Minutes: 600}, "cli"); err == nil || err.Error() != "LidNeedsMinutes" {
		t.Fatalf("ten hours = %v", err)
	}
	follow := awakeSession{Lid: true, PID: 42}
	if err := b.awakeLidGate(paletteURLRequest{HasLid: true, Lid: true}, follow, "cli"); err != nil {
		t.Fatalf("follow = %v", err)
	}
	until := awakeSession{Lid: true, Until: "18:30", End: now.Add(2 * time.Hour)}
	if err := b.awakeLidGate(paletteURLRequest{HasLid: true, Lid: true}, until, "cli"); err != nil {
		t.Fatalf("until = %v", err)
	}
	late := awakeSession{Lid: true, Until: "23:59", End: now.Add(9 * time.Hour)}
	if err := b.awakeLidGate(paletteURLRequest{HasLid: true, Lid: true}, late, "cli"); err == nil || err.Error() != "LidNeedsMinutes" {
		t.Fatalf("late until = %v", err)
	}
	// The remembered preference needs no stated duration.
	if err := b.awakeLidGate(paletteURLRequest{HasLid: false}, awakeSession{Lid: true}, "cli"); err != nil {
		t.Fatalf("remembered lid = %v", err)
	}
	// Not installed: auth needed, never an authorization prompt.
	b.lid.service = &lidTestService{status: svcNotRegistered, after: svcNotRegistered}
	b.lid.snapshotValue.service = svcNotRegistered
	if err := b.awakeLidGate(paletteURLRequest{HasLid: true, Lid: true}, awakeSession{Lid: true, Minutes: 60}, "cli"); err == nil || err.Error() != "LidAuthNeeded" {
		t.Fatalf("not installed = %v", err)
	}
}

// The CLI never registers the helper: with the helper not installed the request
// is refused before any installation, which is what would ask for authorization.
func TestCLINeverRegistersTheHelper(t *testing.T) {
	a := cliTestApp(t)
	service := &lidTestService{status: svcNotRegistered, after: svcNotRegistered}
	a.lid = &lidController{
		service: service, capable: true, supported: true, now: time.Now,
		dial:          func() (lidConnection, error) { return nil, errors.New("gone") },
		read:          func() (int, error) { return 0, nil },
		power:         &lidTestPower{kind: "ac", percent: 80},
		marker:        filepath.Join(t.TempDir(), "marker"),
		snapshotValue: lidSnapshot{service: svcNotRegistered, value: -1},
	}
	t.Cleanup(func() { a.lid.close() })
	result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "a", Op: cli.OpRun, Action: cli.ActionAwake,
		Arg: "2h", Lid: boolPointer(true)}, nil)
	if result.OK || result.Code != "LidAuthNeeded" {
		t.Fatalf("a lid start without an installed helper: %+v", result)
	}
	if service.registers != 0 {
		t.Fatal("the CLI registered the helper: that asks for authorization")
	}
	if a.lidAuthorize {
		t.Fatal("the CLI allowed authorization")
	}
}

func boolPointer(value bool) *bool { return &value }

// testLidController is a controller that needs no helper process.
func testLidController(t *testing.T, status lidServiceStatus) *lidController {
	t.Helper()
	service := &lidTestService{status: status, after: status}
	controller := &lidController{
		service: service,
		dial:    func() (lidConnection, error) { return nil, errors.New("gone") },
		read:    func() (int, error) { return 0, nil },
		power:   &lidTestPower{kind: "ac", percent: 80},
		capable: true, supported: true,
		now:           time.Now,
		marker:        filepath.Join(t.TempDir(), "marker"),
		snapshotValue: lidSnapshot{service: status, value: -1},
	}
	t.Cleanup(func() { controller.close() })
	return controller
}

func TestCLIQuietByDefault(t *testing.T) {
	a := cliTestApp(t)
	t.Cleanup(func() { a.awake.stop() })
	if result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "a", Op: cli.OpRun, Action: cli.ActionAwake, Arg: "30m"}, nil); !result.OK {
		t.Fatalf("awake = %+v", result)
	}
	if a.feedback.message.Text != "" {
		t.Fatalf("the CLI showed a hint: %q", a.feedback.message.Text)
	}
	result := a.applyCLI(cli.Request{V: cli.Protocol, ID: "b", Op: cli.OpRun, Action: cli.ActionAwake, Arg: "30m", Notify: true}, nil)
	if !result.OK {
		t.Fatalf("awake = %+v", result)
	}
	if a.feedback.message.Text == "" {
		t.Fatal("--notify showed no hint")
	}
	if a.quiet {
		t.Fatal("quiet was left set")
	}
}

func TestLidAsyncQuietIsCaptured(t *testing.T) {
	for _, notify := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("notify=%t/fail=%t", notify, fail), func(t *testing.T) {
				a, _, power, svc, _ := lidTestApp(t)
				a.lidAsync = true
				a.feedback.mode = func() string { return "hint" }
				a.feedback.cursorDisplay = func() mygo.Display { return mygo.Display{} }
				if fail {
					power.kind, power.percent = "battery", 1
				}
				callbacks := make(chan func(), 1)
				a.urlDispatch = func(run func()) { callbacks <- run }
				sent := 0
				a.notifyBattery = func(string, string) { sent++ }
				a.config.AwakeLid = false
				yes := true
				results := make(chan cli.Result, 1)
				a.applyCLIAsync(cli.Request{Op: cli.OpRun, Action: cli.ActionAwake, Arg: "1m", Lid: &yes, Notify: notify}, nil, func(r cli.Result) { results <- r })
				select {
				case r := <-results:
					t.Fatalf("answered before completion: %+v", r)
				default:
				}
				if a.quiet {
					t.Fatal("quiet leaked")
				}
				select {
				case callback := <-callbacks:
					callback()
				case <-time.After(3 * time.Second):
					t.Fatal("lid callback did not arrive")
				}
				result := <-results
				if result.OK == fail || fail && result.Code != "LidBatteryTooLow" || result.State.Awake.Lid.State == "enabling" {
					t.Fatalf("final result: %+v", result)
				}
				if !fail && result.State.Awake.Lid.State != "on" {
					t.Fatal("success before lid on")
				}
				if svc.registers != 0 {
					t.Fatal("CLI authorized the helper")
				}
				if a.config.AwakeLid == fail {
					t.Fatalf("success preference = %t", a.config.AwakeLid)
				}
				if !notify && (sent != 0 || a.feedback.message.Text != "") {
					t.Fatal("async callback escaped quiet")
				}
				if notify && !fail && sent != 1 {
					t.Fatalf("notifications = %d", sent)
				}
				if notify && fail && a.feedback.message.Text == "" {
					t.Fatal("failure feedback missing")
				}
			})
		}
	}
}

func TestCLIErrorKey(t *testing.T) {
	cases := map[string]string{
		"ProcessEnded":        "ProcessEnded",
		"LidNeedsMinutes":     "LidNeedsMinutes",
		"LidDevBuild":         "LidDevBuild",
		"DurationError":       "DurationError",
		"keep awake disabled": "AwakeStartError",
		"anything else":       "AwakeStartError",
	}
	for message, want := range cases {
		if got := cliErrorKey(errors.New(message)); got != want {
			t.Fatalf("cliErrorKey(%q) = %q, want %q", message, got, want)
		}
	}
	if cliErrorKey(nil) != "" {
		t.Fatal("nil error named a code")
	}
}

func TestProcessByPID(t *testing.T) {
	process, ok := processByPID(os.Getpid())
	if !ok {
		t.Fatal("this process was not found")
	}
	want := processIdentity(os.Getpid())
	if want == "" {
		t.Skip("ps is not available")
	}
	if process.Identity != want {
		t.Fatalf("identity %q != processIdentity %q", process.Identity, want)
	}
	if process.Name == "" || process.PID != os.Getpid() {
		t.Fatalf("process = %+v", process)
	}
	if _, ok := processByPID(999999); ok {
		t.Fatal("a missing PID was found")
	}
	if _, ok := processByPID(0); ok {
		t.Fatal("PID 0 was found")
	}
	original := psOutput
	psOutput = func(...string) ([]byte, error) { return nil, errors.New("no ps") }
	if _, ok := processByPID(os.Getpid()); ok {
		t.Fatal("a failed ps reported a process")
	}
	psOutput = func(...string) ([]byte, error) { return []byte("Z+ Wed Nov  5 10:00:00 2026 codex\n"), nil }
	if _, ok := processByPID(42); ok {
		t.Fatal("a zombie was followed")
	}
	psOutput = func(...string) ([]byte, error) { return []byte("short\n"), nil }
	if _, ok := processByPID(42); ok {
		t.Fatal("a short ps line was followed")
	}
	psOutput = original
}

func TestCLIStateShapes(t *testing.T) {
	a := cliTestApp(t)
	t.Cleanup(func() { a.awake.stop() })
	state := a.cliState()
	if state.Awake.Active || state.Awake.Mode != "" || state.Awake.End != nil || state.Awake.RemainingSeconds != nil || state.Awake.Follow != nil {
		t.Fatalf("idle state = %+v", state.Awake)
	}
	if state.Tiles.Awake != a.config.AwakeEnabled || state.Version == "" {
		t.Fatalf("state = %+v", state)
	}

	session := a.selectedSession()
	session.Minutes = 30
	session.End = time.Now().Add(30 * time.Minute)
	if err := a.awake.start(session); err != nil {
		t.Fatal(err)
	}
	state = a.cliState()
	if !state.Awake.Active || state.Awake.Minutes != 30 || state.Awake.End == nil || state.Awake.RemainingSeconds == nil {
		t.Fatalf("timed state = %+v", state.Awake)
	}

	unlimited := awakeSession{Mode: "tasks"}
	if err := a.awake.start(unlimited); err != nil {
		t.Fatal(err)
	}
	state = a.cliState()
	if state.Awake.End != nil || state.Awake.RemainingSeconds != nil || state.Awake.Mode != "tasks" {
		t.Fatalf("unlimited state = %+v", state.Awake)
	}

	follow := awakeSession{Mode: "screen", PID: 1234, Name: "codex", Identity: "x"}
	if err := a.awake.start(follow); err != nil {
		t.Fatal(err)
	}
	state = a.cliState()
	if state.Awake.Follow == nil || state.Awake.Follow.PID != 1234 || state.Awake.Follow.Name != "codex" {
		t.Fatalf("follow state = %+v", state.Awake)
	}
}

func TestLidStateNames(t *testing.T) {
	want := map[lidState]string{
		lidOff: "off", lidEnabling: "enabling", lidOn: "on", lidStopping: "stopping",
		lidRestoreFailed: "restore-failed", lidHelperGone: "helper-gone",
		lidBlocked: "blocked", lidError: "error",
	}
	for state, name := range want {
		if got := lidStateName(state); got != name {
			t.Fatalf("%v = %q, want %q", state, got, name)
		}
	}
}

func TestSettingsHeightGeneralPage(t *testing.T) {
	token := tokens(false, "zh-CN")
	// Header card, one group title, three rows (language, login, CLI hint).
	want := int(token.SizePageHeader + token.Space8 + token.SizeGroupHeader + token.Space8 +
		3*token.SizeRowHeight + 2*token.StrokeHairline + 2*token.Space24)
	want = (want + int(token.Space4) - 1) / int(token.Space4) * int(token.Space4)
	if got := settingsNaturalHeight(token, settingsGeneral, true, false, false, true, false); got != want {
		t.Fatalf("general page height = %d, want %d", got, want)
	}
	// The window itself never drops below the shared floor.
	if got := settingsHeight(token, settingsGeneral, true, false, false, true, false); got != 388 {
		t.Fatalf("general window height = %d, want 388", got)
	}
}

// The palette's own view keeps working: the CLI must not have changed it.
func TestPaletteViewStillDraws(t *testing.T) {
	a := cliTestApp(t)
	a.palette = &paletteState{}
	tt := ui.NewTester(a.paletteView, 640, 400)
	tt.SetLocale("zh-CN")
	if !tt.HasText(a.text("zh-CN", "PaletteAll")) {
		t.Fatalf("palette view = %v", tt.Texts())
	}
}

func TestCLIServerPreservesNonSockets(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(fmt.Sprint(symlink), func(t *testing.T) {
			dir := shortDir(t)
			path := filepath.Join(dir, "cli.sock")
			target := filepath.Join(dir, "keep")
			if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			if symlink {
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			s := &cliServer{path: path}
			if err := s.listen(); err == nil {
				s.stop()
				t.Fatal("unsafe path accepted")
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "keep" {
				t.Fatalf("entry changed: %q %v", data, err)
			}
		})
	}
}

func TestCLIServerQueuedTimeoutAndShutdown(t *testing.T) {
	s, path := testServer(t, cliTestApp(t))
	callbacks := make(chan func(), 1)
	returned := make(chan struct{})
	// Set before connecting, when serve has not dispatched any request.
	s.actionTimeout = 20 * time.Millisecond
	s.dispatch = func(run func()) { callbacks <- run; <-returned }
	var applied atomic.Int32
	s.apply = func(cli.Request, []byte) cli.Result { applied.Add(1); return cli.NewResult(nil) }
	client := dial(t, path)
	result := do(t, client, cli.Request{Op: cli.OpStatus}, nil)
	if result.Code != cli.CodeBusy {
		t.Fatalf("timeout = %+v", result)
	}
	callback := <-callbacks
	callback() // a timed-out queued action must not run later
	if applied.Load() != 0 {
		t.Fatal("expired action executed")
	}
	if len(s.slots) != 1 {
		t.Fatal("blocked dispatch released its concurrency slot")
	}
	close(returned)
	s.stop()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestApplyCLIQuittingAndToggleNotify(t *testing.T) {
	a := cliTestApp(t)
	t.Cleanup(a.awake.stop)
	a.quitting = true
	if result := a.applyCLI(cli.Request{Op: cli.OpStatus}, nil); result.Code != cli.CodeQuitting {
		t.Fatalf("quitting = %+v", result)
	}
	a.quitting = false
	result := a.applyCLI(cli.Request{Op: cli.OpRun, Action: cli.ActionAwakeToggle, Notify: true}, nil)
	if !result.OK || a.feedback.message.Text == "" {
		t.Fatalf("toggle notification missing: %+v", result)
	}
}
