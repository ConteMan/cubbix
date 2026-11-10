package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
	qrlib "github.com/ConteMan/cubbix/internal/qr"
)

// qrStubConn answers one `cubbix qr` request with a fixed result.
type qrStubConn struct {
	result cli.Result
	err    error
	req    cli.Request
	body   []byte
}

func (c *qrStubConn) Do(req cli.Request, body []byte, _ time.Duration) (cli.Result, error) {
	c.req, c.body = req, append([]byte(nil), body...)
	return c.result, c.err
}

func (c *qrStubConn) Close() error { return nil }

func qrEnv(t *testing.T, result cli.Result, stdin string, terminal bool) (*env, *bytes.Buffer, *bytes.Buffer, *qrStubConn) {
	t.Helper()
	stub := &qrStubConn{result: result}
	e, stdout, stderr, _ := testEnv(t, func(string, time.Duration) (conn, error) { return stub, nil })
	e.stdin = func() (io.Reader, bool) { return strings.NewReader(stdin), terminal }
	return e, stdout, stderr, stub
}

// qrOKResult is the metadata the app answers with for text that fits.
func qrOKResult(text string) cli.Result {
	code, err := qrlib.Encode(text)
	if err != nil {
		panic(err)
	}
	result := cli.NewResult(nil)
	result.QR = &cli.QRResult{Version: code.Version(), ECC: code.ECC(), Modules: code.Modules(), Bytes: code.Bytes}
	return result
}

func TestCLIQRValidate(t *testing.T) {
	inv, err := parse([]string{"qr", "https://example.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.command != "qr" || inv.textArg != "https://example.com/a" || inv.hasPNG {
		t.Fatalf("qr = %+v", inv)
	}
	inv, err = parse([]string{"qr", "--png", "out.png", "--text", "hello", "--json"})
	if err != nil || inv.png != "out.png" || !inv.hasPNG || inv.body.text != "hello" || !inv.flags.json {
		t.Fatalf("qr options = %+v %v", inv, err)
	}
	inv, err = parse([]string{"help", "qr"})
	if err != nil || inv.helpTopic != "qr" {
		t.Fatalf("help qr = %+v %v", inv, err)
	}
	for _, args := range [][]string{
		{"qr", "--notify"},
		{"qr", "--blank"},
		{"qr", "--to", "zh"},
		{"qr", "--minutes", "5"},
		{"qr", "--page", "qr"},
		{"qr", "one", "two"},
		{"qr", "--png="},
	} {
		if _, err := parse(args); err == nil {
			t.Fatalf("%v was accepted", args)
		} else if _, ok := err.(*usageError); !ok {
			t.Fatalf("%v: not a usage error: %v", args, err)
		}
	}
	// A body frame is allowed, and nothing else is.
	request := cli.Request{V: cli.Protocol, ID: "q", Op: cli.OpRun, Action: cli.ActionQR, Bytes: 3}
	if code := request.Validate(cli.MaxPayload); code != "" {
		t.Fatalf("Validate = %q", code)
	}
	for _, bad := range []cli.Request{
		{V: cli.Protocol, ID: "q", Op: cli.OpRun, Action: cli.ActionQR, Bytes: 1, Arg: "x"},
		{V: cli.Protocol, ID: "q", Op: cli.OpRun, Action: cli.ActionQR, Bytes: 1, Notify: true},
		{V: cli.Protocol, ID: "q", Op: cli.OpRun, Action: cli.ActionQR, Bytes: 1, Page: "qr"},
		{V: cli.Protocol, ID: "q", Op: cli.OpRun, Action: cli.ActionQR, Bytes: -1},
	} {
		if code := bad.Validate(cli.MaxPayload); code == "" {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

func TestCLIQROutput(t *testing.T) {
	text := "https://example.com/orders/2026"
	code, err := qrlib.Encode(text)
	if err != nil {
		t.Fatal(err)
	}
	// The default is the block drawing with a quiet zone of four modules.
	e, stdout, stderr, stub := qrEnv(t, qrOKResult(text), "", true)
	if exit := run([]string{"qr", text}, e); exit != exitOK {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
	if stub.req.Action != cli.ActionQR || stub.req.Bytes != len(text) || string(stub.body) != text {
		t.Fatalf("request = %+v body=%q", stub.req, stub.body)
	}
	if got := stdout.String(); got != code.Matrix(qrlib.QuietZone) {
		t.Fatalf("drawing differs:\n%s", got)
	}
	// The app answers only metadata: the drawing is made here, after the
	// answer, so nothing is printed for a refusal.
	refusedEnv, refusedOut, _, _ := qrEnv(t, cli.Failure("disabled"), "", true)
	if exit := run([]string{"qr", text}, refusedEnv); exit != exitRejected {
		t.Fatalf("disabled exit = %d", exit)
	}
	if refusedOut.Len() != 0 {
		t.Fatalf("a refusal printed a drawing: %q", refusedOut.String())
	}
	// --png writes the file and prints nothing.
	dir := t.TempDir()
	path := filepath.Join(dir, "code.png")
	e, stdout, stderr, _ = qrEnv(t, qrOKResult(text), "", true)
	if exit := run([]string{"qr", text, "--png", path}, e); exit != exitOK {
		t.Fatalf("--png exit = %d, stderr = %q", exit, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("--png printed %q", stdout.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || !bytes.HasPrefix(data, []byte("\x89PNG")) {
		t.Fatalf("not a PNG: %d bytes", len(data))
	}
	// --json prints one metadata line, and png is only the path.
	e, stdout, _, _ = qrEnv(t, qrOKResult(text), "", true)
	if exit := run([]string{"qr", text, "--png", path, "--json"}, e); exit != exitOK {
		t.Fatalf("--json exit = %d", exit)
	}
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("--json printed %d lines", len(lines))
	}
	var printed cli.Result
	if err := json.Unmarshal([]byte(lines[0]), &printed); err != nil {
		t.Fatal(err)
	}
	if printed.QR == nil || printed.QR.Version == 0 || printed.QR.ECC != "M" || printed.QR.Modules == 0 || printed.QR.Bytes != len(text) {
		t.Fatalf("qr metadata = %+v", printed.QR)
	}
	if printed.QR.PNG != path || printed.Bytes != 0 || len(lines[0]) > cli.MaxLine {
		t.Fatalf("json line = %q", lines[0])
	}
	// A refused answer keeps the code and writes no file.
	refusedPath := filepath.Join(dir, "refused.png")
	e, _, _, _ = qrEnv(t, cli.Failure("QRTooLong"), "", true)
	if exit := run([]string{"qr", text, "--png", refusedPath}, e); exit != exitRejected {
		t.Fatalf("too-long exit = %d", exit)
	}
	if _, err := os.Stat(refusedPath); !os.IsNotExist(err) {
		t.Fatal("a refused answer wrote a file")
	}
}

func TestCLIQRExitCodes(t *testing.T) {
	text := "hello"
	cases := []struct {
		name   string
		result cli.Result
		want   int
	}{
		{"ok", qrOKResult(text), exitOK},
		{"disabled", cli.Failure("disabled"), exitRejected},
		{"too long", cli.Failure("QRTooLong"), exitRejected},
		{"busy", cli.Failure(cli.CodeBusy), exitRejected},
	}
	for _, c := range cases {
		e, _, _, _ := qrEnv(t, c.result, "", true)
		if got := run([]string{"qr", text}, e); got != c.want {
			t.Fatalf("%s: exit = %d, want %d", c.name, got, c.want)
		}
	}
	// A usage mistake never reaches the app.
	e, _, stderr, _ := qrEnv(t, qrOKResult(text), "", true)
	if got := run([]string{"qr"}, e); got != exitUsage {
		t.Fatalf("empty text exit = %d, stderr = %q", got, stderr.String())
	}
	// A pipe is the text.
	e, stdout, _, stub := qrEnv(t, qrOKResult("piped"), "piped", false)
	if got := run([]string{"qr"}, e); got != exitOK || string(stub.body) != "piped" || stdout.Len() == 0 {
		t.Fatalf("piped exit = %d body=%q", got, stub.body)
	}
}

// TestCLIQRPage pins the fourth place the page name has to appear: the
// settings whitelist. Without it `cubbix settings --page qr` exits 2.
func TestCLIQRPage(t *testing.T) {
	inv, err := parse([]string{"settings", "--page", "qr"})
	if err != nil || inv.page != "qr" || inv.command != "settings" {
		t.Fatalf("settings --page qr = %+v %v", inv, err)
	}
}
