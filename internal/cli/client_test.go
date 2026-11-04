package cli

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSocketPath(t *testing.T) {
	short := filepath.Join("/tmp", "cubbix-test")
	if got := SocketPath(short); got != filepath.Join(short, SocketName) {
		t.Fatalf("short path = %q", got)
	}
	long := filepath.Join("/tmp", strings.Repeat("d", 120))
	got := SocketPath(long)
	want := "/tmp/cubbix-cli-" + itoa(os.Getuid()) + ".sock"
	if got != want {
		t.Fatalf("fallback = %q, want %q", got, want)
	}
	// The fallback must not follow $TMPDIR: a terminal and a process started by
	// open can disagree about it.
	if temp := os.TempDir(); temp != "" && strings.Contains(got, temp) {
		t.Fatalf("fallback %q embeds TMPDIR %q", got, temp)
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

// listener starts a socket server that hands each accepted connection on.
// shortPath is a socket path short enough for sun_path.
func shortPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cbs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func listener(t *testing.T, path string) *net.UnixListener {
	t.Helper()
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

func TestVerifyServer(t *testing.T) {
	dir := t.TempDir()

	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyServer(nil, plain); CodeOf(err) != CodeRefused {
		t.Fatalf("a plain file was accepted: %v", err)
	}

	open := filepath.Join(dir, "open.sock")
	ln := listener(t, open)
	if err := os.Chmod(open, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := VerifyServer(nil, open); CodeOf(err) != CodeRefused {
		t.Fatalf("a world-writable socket was accepted: %v", err)
	}
	ln.Close()

	good := filepath.Join(dir, "good.sock")
	listener(t, good)
	if err := os.Chmod(good, 0o600); err != nil {
		t.Fatal(err)
	}
	client, server := unixPairAt(t, good)
	defer client.Close()
	if err := VerifyServer(server, good); err != nil {
		t.Fatalf("our own socket was refused: %v", err)
	}
}

// unixPairAt connects to a listening path and returns both ends.
func unixPairAt(t *testing.T, path string) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path + ".pair", Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	accepted := make(chan *net.UnixConn, 1)
	go func() {
		conn, err := ln.AcceptUnix()
		if err == nil {
			accepted <- conn
		}
	}()
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path + ".pair", Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	server := <-accepted
	t.Cleanup(func() { server.Close() })
	return client, server
}

func TestDialRequiresOurOwnSocket(t *testing.T) {
	path := shortPath(t)
	ln := listener(t, path)
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()
	if _, err := Dial(path, time.Second); CodeOf(err) != CodeRefused {
		t.Fatalf("dial to a world-writable socket = %v", err)
	}
}

// The tool never writes a request, let alone a body, to a socket it has not
// verified: the check runs inside Dial, before Do can be called.
func TestDialSendsNothingToAnUnverifiedServer(t *testing.T) {
	path := shortPath(t)
	ln := listener(t, path)
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	read := make(chan int, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			read <- -1
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 64)
		n, _ := conn.Read(buf)
		read <- n
	}()
	if _, err := Dial(path, time.Second); CodeOf(err) != CodeRefused {
		t.Fatalf("dial = %v", err)
	}
	if n := <-read; n > 0 {
		t.Fatalf("the tool wrote %d bytes to an unverified server", n)
	}
}

func TestDoRoundTrip(t *testing.T) {
	path := shortPath(t)
	ln := listener(t, path)
	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		line, _ := reader.ReadBytes('\n')
		var req Request
		_ = json.Unmarshal(line, &req)
		body := make([]byte, req.Bytes)
		if req.Bytes > 0 {
			_, _ = reader.Read(body)
		}
		response := Response{V: Protocol, ID: req.ID, OK: true, Message: string(body)}
		data, _ := EncodeLine(response)
		_, _ = conn.Write(data)
	}()
	client, err := Dial(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.Do(Request{V: Protocol, ID: "abc", Op: OpRun, Action: ActionJSONNew, Bytes: 3}, []byte("hey"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Message != "hey" {
		t.Fatalf("result = %+v", result)
	}
}

func TestDoRejectsForeignAnswer(t *testing.T) {
	path := shortPath(t)
	ln := listener(t, path)
	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = bufio.NewReader(conn).ReadBytes('\n')
		data, _ := EncodeLine(Response{V: Protocol, ID: "other", OK: true})
		_, _ = conn.Write(data)
	}()
	client, err := Dial(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Do(Request{V: Protocol, ID: "abc", Op: OpStatus}, nil, 2*time.Second); CodeOf(err) != CodeInternal {
		t.Fatalf("foreign answer = %v", err)
	}
}

func TestDoTimesOut(t *testing.T) {
	path := shortPath(t)
	ln := listener(t, path)
	go func() {
		conn, err := ln.AcceptUnix()
		if err == nil {
			time.Sleep(2 * time.Second)
			conn.Close()
		}
	}()
	client, err := Dial(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Do(Request{V: Protocol, ID: "abc", Op: OpStatus}, nil, 200*time.Millisecond); CodeOf(err) != CodeTimeout {
		t.Fatalf("silence = %v", err)
	}
}

func TestDialClassifies(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "none.sock")
	_, err := Dial(missing, time.Second)
	if CodeOf(err) != CodeStale || !CanLaunch(err) {
		t.Fatalf("missing socket = %v (launch %v)", err, CanLaunch(err))
	}
	if CanLaunch(&Error{Code: CodeRefused}) {
		t.Fatal("a refused socket must not start the app")
	}
}
