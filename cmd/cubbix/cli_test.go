package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
)

// testEnv is a tool environment that never touches a socket or the app.
func testEnv(t *testing.T, dial func(string, time.Duration) (conn, error)) (*env, *bytes.Buffer, *bytes.Buffer, *[]string) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	launched := &[]string{}
	return &env{
		stdout:     stdout,
		stderr:     stderr,
		dial:       dial,
		executable: func() (string, error) { return "/tmp/Cubbix.app/Contents/Resources/bin/cubbix", nil },
		launch: func(target string, byBundleID bool) error {
			if byBundleID {
				target = "bundle:" + target
			}
			*launched = append(*launched, target)
			return nil
		},
		stdin: func() (io.Reader, bool) { return strings.NewReader(""), true },
		sleep: func(time.Duration) {},
	}, stdout, stderr, launched
}

func TestParseAwakeOn(t *testing.T) {
	cases := []struct {
		args []string
		arg  string
	}{
		{[]string{"awake", "on", "--minutes", "90"}, "1h30m"},
		{[]string{"awake", "on", "--minutes=45"}, "45m"},
		{[]string{"awake", "on", "--minutes", "0"}, "∞"},
		{[]string{"awake", "on", "--minutes", "525659"}, "8760h59m"},
		{[]string{"awake", "on", "--until", "18:30"}, "until 18:30"},
		{[]string{"awake", "on", "--follow", "1234"}, ""},
		{[]string{"awake", "on"}, ""},
	}
	for _, scenario := range cases {
		inv, err := parse(scenario.args)
		if err != nil {
			t.Fatalf("%v: %v", scenario.args, err)
		}
		if inv.command != "awake.on" || inv.awake.arg != scenario.arg {
			t.Fatalf("%v = %q (arg %q)", scenario.args, inv.command, inv.awake.arg)
		}
	}
}

func TestParseAwakeOptions(t *testing.T) {
	inv, err := parse([]string{"awake", "on", "--follow", "42", "--mode", "tasks", "--lid", "--notify", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.awake.follow != 42 || inv.awake.mode != "tasks" || inv.awake.lid == nil || !*inv.awake.lid {
		t.Fatalf("options = %+v", inv.awake)
	}
	if !inv.flags.notify || !inv.flags.json {
		t.Fatalf("flags = %+v", inv.flags)
	}
	inv, err = parse([]string{"awake", "on", "--no-lid"})
	if err != nil || inv.awake.lid == nil || *inv.awake.lid {
		t.Fatalf("--no-lid = %+v %v", inv.awake.lid, err)
	}
	inv, err = parse([]string{"awake", "on"})
	if err != nil || inv.awake.lid != nil {
		t.Fatalf("default lid = %+v %v", inv.awake.lid, err)
	}
}

func TestParseUsageErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"awake"},
		{"awake", "on", "--minutes", "90", "--until", "18:30"},
		{"awake", "on", "--follow", "9", "--minutes", "5"},
		{"awake", "on", "--lid", "--no-lid"},
		{"awake", "on", "--minutes", "525660"},
		{"awake", "on", "--minutes", "-1"},
		{"awake", "on", "--minutes", "abc"},
		{"awake", "on", "--until", "99:99"},
		{"awake", "on", "--follow", "0"},
		{"awake", "on", "--follow", "-3"},
		{"awake", "on", "--mode", "lid"},
		{"awake", "off", "--minutes", "30"},
		{"awake", "toggle", "--mode", "tasks"},
		{"awake", "status", "--notify"},
		{"status", "--notify"},
		{"awake", "status", "--minutes", "5"},
		{"json", "new", "--minutes", "5"},
		{"json"},
		{"palette", "--text", "x"},
		{"settings", "--page", "bogus"},
		{"version", "--json"},
		{"help", "--json"},
		{"awake", "on", "--timeout", "0s"},
		{"awake", "on", "--timeout", "61s"},
		{"awake", "on", "--unknown", "x"},
		{"unknown"},
	}
	for _, args := range cases {
		if _, err := parse(args); err == nil {
			t.Fatalf("%v was accepted", args)
		} else if _, ok := err.(*usageError); !ok {
			t.Fatalf("%v: not a usage error: %v", args, err)
		}
	}
}

// pageNumber is the app's page index for a page name, -1 for an unknown one.
func pageNumber(page string) int {
	switch page {
	case "", "general":
		return 0
	case "about":
		return 1
	case "tiles":
		return 2
	case "shortcuts":
		return 3
	}
	return -1
}

func TestParseSettingsPages(t *testing.T) {
	for page, number := range map[string]int{"general": 0, "about": 1, "tiles": 2, "shortcuts": 3} {
		inv, err := parse([]string{"settings", "--page", page})
		if err != nil {
			t.Fatal(err)
		}
		if got := pageNumber(inv.page); got != number {
			t.Fatalf("%s = %d", page, got)
		}
	}
	inv, err := parse([]string{"settings"})
	if err != nil || inv.page != "general" {
		t.Fatalf("default page = %q %v", inv.page, err)
	}
	if got := pageNumber("bogus"); got != -1 {
		t.Fatalf("an unknown page mapped to %d", got)
	}
}

// A usage mistake still prints exactly one JSON object when --json is given.
func TestUsageErrorJSON(t *testing.T) {
	e, stdout, stderr, _ := testEnv(t, func(string, time.Duration) (conn, error) {
		t.Fatal("a usage mistake must not connect")
		return nil, nil
	})
	code := run([]string{"awake", "on", "--minutes", "90", "--until", "18:30", "--json"}, e)
	if code != exitUsage {
		t.Fatalf("exit = %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	line := strings.TrimSpace(stdout.String())
	var result cli.Result
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("%q: %v", line, err)
	}
	if result.OK || result.Code != cli.CodeUsage || result.Message == "" || result.State != nil {
		t.Fatalf("result = %+v", result)
	}
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("more than one line: %q", line)
	}
}

func TestUsageErrorHuman(t *testing.T) {
	e, stdout, stderr, _ := testEnv(t, nil)
	if code := run([]string{"awake", "on", "--minutes", "90", "--until", "18:30"}, e); code != exitUsage {
		t.Fatalf("exit = %d", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "--minutes, --until and --follow are mutually exclusive.") {
		t.Fatalf("stdout %q stderr %q", stdout.String(), stderr.String())
	}
}

// launchApp prefers the bundle this tool belongs to, then the bundle id.
func TestLaunchOrder(t *testing.T) {
	e, _, _, launched := testEnv(t, nil)
	launchApp(e)
	if len(*launched) != 1 || (*launched)[0] != "/tmp/Cubbix.app" {
		t.Fatalf("launched = %v", *launched)
	}
	*launched = nil
	e.launch = func(target string, byBundleID bool) error {
		if !byBundleID {
			return errors.New("no such bundle")
		}
		*launched = append(*launched, "bundle:"+target)
		return nil
	}
	launchApp(e)
	if len(*launched) != 1 || (*launched)[0] != "bundle:"+appBundleID {
		t.Fatalf("fallback = %v", *launched)
	}
}

// connect keeps launching only for "not running": a socket this user cannot open
// must not start a second copy.
func TestConnectRefusesToLaunchOnRefused(t *testing.T) {
	refused := &cli.Error{Code: cli.CodeRefused, Msg: "nope"}
	e, _, _, launched := testEnv(t, func(string, time.Duration) (conn, error) { return nil, refused })
	inv, err := parse([]string{"awake", "on", "--socket", filepath.Join(t.TempDir(), "s.sock")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connect(e, inv); cli.CodeOf(err) != cli.CodeRefused {
		t.Fatalf("err = %v", err)
	}
	if len(*launched) != 0 {
		t.Fatalf("launched = %v", *launched)
	}
}

func TestVersionNeedsNoSocket(t *testing.T) {
	e, stdout, _, _ := testEnv(t, func(string, time.Duration) (conn, error) {
		t.Fatal("version must not connect")
		return nil, nil
	})
	if code := run([]string{"version"}, e); code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout.String(), "/tmp/Cubbix.app") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestBundleOf(t *testing.T) {
	bundle, ok := bundleOf("/Applications/Cubbix.app/Contents/Resources/bin/cubbix")
	if !ok || bundle != "/Applications/Cubbix.app" {
		t.Fatalf("bundle = %q %v", bundle, ok)
	}
	if _, ok := bundleOf("/usr/local/bin/cubbix"); ok {
		t.Fatal("a bare binary has no bundle")
	}
}

func TestPlistVersion(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "Cubbix.app")
	if err := os.MkdirAll(filepath.Join(bundle, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict>
	<key>CFBundleShortVersionString</key>
	<string>0.4.0</string>
	</dict></plist>`
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte(plist), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := plistVersion(bundle); got != "0.4.0" {
		t.Fatalf("version = %q", got)
	}
	if got := plistVersion(filepath.Join(dir, "absent.app")); got != "unknown" {
		t.Fatalf("missing version = %q", got)
	}
}

func TestReadOnlyStatusNeverLaunches(t *testing.T) {
	for _, command := range [][]string{{"status"}, {"awake", "status"}} {
		e, _, _, launched := testEnv(t, func(string, time.Duration) (conn, error) { return nil, &cli.Error{Code: cli.CodeStale} })
		if code := run(command, e); code != exitUnavail || len(*launched) != 0 {
			t.Fatalf("status launched: %v %d %v", command, code, *launched)
		}
	}
}

func TestBodyExplicitEmptyAndBlank(t *testing.T) {
	for _, args := range [][]string{{"json", "new", "--blank"}, {"json", "new", "--text="}, {"json", "new", "--text=", "--file=missing"}} {
		inv, err := parse(args)
		if err != nil {
			t.Fatal(err)
		}
		e, _, _, _ := testEnv(t, nil)
		e.stdin = func() (io.Reader, bool) { t.Fatal("explicit empty input read stdin"); return nil, false }
		req, body, err := buildRequest(inv, e)
		if err != nil || len(body) != 0 || !req.Blank {
			t.Fatalf("%v = %+v %q %v", args, req, body, err)
		}
	}
}

func TestParseDevtoolsCommands(t *testing.T) {
	inv, err := parse([]string{"ts"})
	if err != nil || inv.command != "ts" || inv.hasTextArg {
		t.Fatalf("ts = %+v %v", inv, err)
	}
	// A negative number is ts's text, not an unknown option.
	inv, err = parse([]string{"ts", "-1"})
	if err != nil || !inv.hasTextArg || inv.textArg != "-1" {
		t.Fatalf("ts -1 = %+v %v", inv, err)
	}
	inv, err = parse([]string{"uuid"})
	if err != nil || inv.command != "uuid" || inv.uuidVersion != 0 || inv.hasCount || inv.upper || inv.plain {
		t.Fatalf("uuid = %+v %v", inv, err)
	}
	inv, err = parse([]string{"uuid", "3", "--v7", "--upper", "--plain"})
	if err != nil || inv.count != 3 || !inv.hasCount || inv.uuidVersion != 7 || !inv.upper || !inv.plain {
		t.Fatalf("uuid options = %+v %v", inv, err)
	}
	inv, err = parse([]string{"uuid", "-n", "100", "--v4"})
	if err != nil || inv.count != 100 || inv.uuidVersion != 4 {
		t.Fatalf("uuid -n = %+v %v", inv, err)
	}
	for _, args := range [][]string{
		{"uuid", "--v4", "--v7"},
		{"uuid", "0"},
		{"uuid", "101"},
		{"uuid", "3", "-n", "2"},
		{"uuid", "-n", "0"},
		{"uuid", "abc"},
		{"uuid", "--text", "x"},
		{"uuid", "--page", "qr"},
		{"ts", "--notify"},
		{"ts", "--text", "x"},
		{"ts", "a", "b"},
	} {
		if _, err := parse(args); err == nil {
			t.Fatalf("%v was accepted", args)
		} else if _, ok := err.(*usageError); !ok {
			t.Fatalf("%v: not a usage error: %v", args, err)
		}
	}
}

// TestTSLongTextIsUsage bounds the ts argument before any connection: the app's
// MaxArg is 128 bytes, and an over-long text fails here rather than remotely.
func TestTSLongTextIsUsage(t *testing.T) {
	e, stdout, stderr, _ := testEnv(t, func(string, time.Duration) (conn, error) {
		t.Fatal("an over-long ts text connected")
		return nil, nil
	})
	if code := run([]string{"ts", strings.Repeat("1", cli.MaxArg+1)}, e); code != exitUsage {
		t.Fatalf("exit = %d", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "128") {
		t.Fatalf("stdout %q stderr %q", stdout.String(), stderr.String())
	}
}
