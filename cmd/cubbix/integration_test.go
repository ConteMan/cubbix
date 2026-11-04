package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
)

// binPath is the built tool every case runs, so the tests exercise the real
// binary and a stub server: no app, no window, no launch.
var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cubbix-cli-test")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "cubbix")
	build := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := build.CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// stub serves the protocol on path, answering with what handle returns. A nil
// answer means "say nothing", which the tool reports as a timeout.
func stub(t *testing.T, path string, handle func(cli.Request, []byte) *cli.Response) {
	t.Helper()
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var once sync.Once
	go func() {
		for {
			conn, err := ln.AcceptUnix()
			if err != nil {
				return
			}
			go func(conn *net.UnixConn) {
				defer conn.Close()
				reader := bufio.NewReaderSize(conn, cli.MaxLine+1)
				line, err := reader.ReadBytes('\n')
				if err != nil {
					return
				}
				var req cli.Request
				if err := json.Unmarshal(line, &req); err != nil {
					return
				}
				body := make([]byte, req.Bytes)
				if req.Bytes > 0 {
					if _, err := io.ReadFull(reader, body); err != nil {
						return
					}
				}
				once.Do(func() {})
				response := handle(req, body)
				if response == nil {
					time.Sleep(2 * time.Second)
					return
				}
				response.V = cli.Protocol
				response.ID = req.ID
				data, err := cli.EncodeLine(*response)
				if err != nil {
					return
				}
				_, _ = conn.Write(data)
			}(conn)
		}
	}()
}

func tempSocket(t *testing.T) string {
	t.Helper()
	// A short directory: sun_path is 104 bytes, and a test name is not.
	dir, err := os.MkdirTemp("", "cbs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

// runCLI runs the tool with a clean environment except the stubs.
func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	command := exec.Command(binPath, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v", err)
	}
	return stdout.String(), stderr.String(), code
}

func TestIntegrationStatus(t *testing.T) {
	path := tempSocket(t)
	state := &cli.State{Version: "0.4.0", Tiles: cli.Tiles{Awake: true}, JSON: cli.JSONState{Windows: 2}}
	stub(t, path, func(req cli.Request, _ []byte) *cli.Response {
		if req.Op != cli.OpStatus {
			return &cli.Response{OK: false, Code: cli.CodeBadRequest}
		}
		return &cli.Response{OK: true, Lines: []string{"第一行", "JSON 窗口 2", "Cubbix 0.4.0"}, State: state}
	})
	stdout, _, code := runCLI(t, "status", "--socket", path)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if lines := strings.Split(strings.TrimSpace(stdout), "\n"); len(lines) != 3 {
		t.Fatalf("stdout = %q", stdout)
	}
	stdout, stderr, code := runCLI(t, "awake", "status", "--socket", path)
	if code != 0 || stderr != "" || stdout != "第一行\n" {
		t.Fatalf("awake status = %q %q %d", stdout, stderr, code)
	}
}

func TestIntegrationJSONOutput(t *testing.T) {
	path := tempSocket(t)
	stub(t, path, func(cli.Request, []byte) *cli.Response {
		return &cli.Response{OK: true, Message: "hello", State: &cli.State{Version: "0.4.0"}}
	})
	stdout, _, code := runCLI(t, "awake", "on", "--minutes", "60", "--json", "--socket", path)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
		t.Fatalf("more than one line: %q", stdout)
	}
	var result cli.Result
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("%q: %v", stdout, err)
	}
	if !result.OK || result.Message != "hello" || result.State == nil {
		t.Fatalf("result = %+v", result)
	}
}

// Every action code maps to the documented exit status.
func TestIntegrationCodes(t *testing.T) {
	cases := []struct {
		code string
		exit int
	}{
		{cli.CodeBadRequest, 1},
		{cli.CodeProtocolMismatch, 1},
		{cli.CodeUntrustedPeer, 1},
		{cli.CodeBusy, 4},
		{cli.CodeQuitting, 3},
		{cli.CodeDisabled, 4},
		{"LidAuthNeeded", 4},
		{"LidNeedsMinutes", 4},
		{"ProcessEnded", 4},
		{"AwakeStartError", 4},
	}
	for _, scenario := range cases {
		t.Run(scenario.code, func(t *testing.T) {
			path := tempSocket(t)
			stub(t, path, func(cli.Request, []byte) *cli.Response {
				return &cli.Response{OK: false, Code: scenario.code, Message: "message for " + scenario.code}
			})
			stdout, stderr, code := runCLI(t, "awake", "off", "--socket", path)
			if code != scenario.exit {
				t.Fatalf("%s exit = %d, want %d (%q)", scenario.code, code, scenario.exit, stderr)
			}
			if stdout != "" || !strings.Contains(stderr, "message for "+scenario.code) {
				t.Fatalf("stdout %q stderr %q", stdout, stderr)
			}
		})
	}
}

func TestIntegrationTimeout(t *testing.T) {
	path := tempSocket(t)
	stub(t, path, func(cli.Request, []byte) *cli.Response { return nil })
	_, stderr, code := runCLI(t, "status", "--timeout", "1s", "--socket", path)
	if code != 5 {
		t.Fatalf("exit = %d (%q)", code, stderr)
	}
	if !strings.Contains(stderr, "did not answer") {
		t.Fatalf("stderr = %q", stderr)
	}
}

// --no-launch must answer at once and never start the app: the unit test proves
// open is not called, and this checks the status and the promptness.
func TestIntegrationNoLaunch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.sock")
	start := time.Now()
	_, stderr, code := runCLI(t, "awake", "on", "--no-launch", "--socket", path)
	if code != 3 {
		t.Fatalf("exit = %d (%q)", code, stderr)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("--no-launch waited %v: it launched somehow", elapsed)
	}
}

func TestIntegrationStaleSocket(t *testing.T) {
	path := tempSocket(t)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	ln.SetUnlinkOnClose(false)
	ln.Close() // the file stays, nobody serves it
	stdout, stderr, code := runCLI(t, "status", "--json", "--no-launch", "--socket", path)
	if code != 3 {
		t.Fatalf("exit = %d (%q)", code, stderr)
	}
	var result cli.Result
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("%q: %v", stdout, err)
	}
	if result.Code != cli.CodeStale {
		t.Fatalf("code = %q", result.Code)
	}
}

func TestIntegrationRefusedSocket(t *testing.T) {
	path := tempSocket(t)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLI(t, "status", "--json", "--socket", path)
	if code != 3 {
		t.Fatalf("exit = %d (%q)", code, stderr)
	}
	var result cli.Result
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("%q: %v", stdout, err)
	}
	if result.Code != cli.CodeRefused {
		t.Fatalf("code = %q", result.Code)
	}
}

func TestIntegrationBody(t *testing.T) {
	path := tempSocket(t)
	bodies := make(chan string, 1)
	stub(t, path, func(req cli.Request, body []byte) *cli.Response {
		bodies <- string(body)
		return &cli.Response{OK: true}
	})
	if _, stderr, code := runCLI(t, "json", "new", "--text", `{"a":1}`, "--socket", path); code != 0 {
		t.Fatalf("exit = %d (%q)", code, stderr)
	}
	select {
	case body := <-bodies:
		if body != `{"a":1}` {
			t.Fatalf("body = %q", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no request")
	}
}

func TestIntegrationLargeBody(t *testing.T) {
	if testing.Short() {
		t.Skip("64 MiB body")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "big.json")
	handle, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(handle, zeroReader{}, cli.MaxPayload); err != nil {
		t.Fatal(err)
	}
	handle.Close()
	path := tempSocket(t)
	var received int
	stub(t, path, func(req cli.Request, body []byte) *cli.Response {
		received = len(body)
		return &cli.Response{OK: true}
	})
	if _, stderr, code := runCLI(t, "json", "new", "--file", file, "--timeout", "30s", "--socket", path); code != 0 {
		t.Fatalf("exit = %d (%q)", code, stderr)
	}
	if received != cli.MaxPayload {
		t.Fatalf("received %d bytes", received)
	}
	// One byte more is refused before anything is sent.
	tooBig := filepath.Join(dir, "too-big.json")
	handle, err = os.Create(tooBig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(handle, zeroReader{}, cli.MaxPayload+1); err != nil {
		t.Fatal(err)
	}
	handle.Close()
	if _, stderr, code := runCLI(t, "json", "new", "--file", tooBig, "--socket", path); code != 2 {
		t.Fatalf("oversized exit = %d (%q)", code, stderr)
	}
}

// zeroReader never blocks, so the big file is written without a source.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}
