package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
)

// This subprocess only hosts the production socket server. No application,
// window, shortcut, privileged helper or launch service is initialized.
func TestCLISocketProcessHelper(t *testing.T) {
	path := os.Getenv("CUBBIX_REVIEW_SOCKET")
	if path == "" {
		t.Skip("socket subprocess only")
	}
	s := &cliServer{
		path: path, slots: make(chan struct{}, cli.MaxConcurrent),
		verify: verifySameUser, dispatch: func(run func()) { run() },
		apply: func(cli.Request, []byte) cli.Result { return cli.NewResult(nil) },
		logf:  func(string, ...any) {},
	}
	if err := s.listen(); err != nil {
		fmt.Println("occupied")
		return
	}
	go s.serve()
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin) // the parent controls graceful shutdown
	s.stop()
}

func TestCLISocketTwoProcesses(t *testing.T) {
	path := filepath.Join(shortDir(t), "cli.sock")
	type process struct {
		cmd   *exec.Cmd
		input io.WriteCloser
	}
	start := func() (*process, string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCLISocketProcessHelper$")
		cmd.Env = append(os.Environ(), "CUBBIX_REVIEW_SOCKET="+path)
		input, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		output, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		p := &process{cmd, input}
		t.Cleanup(func() { input.Close(); _ = cmd.Wait() })
		line, err := bufio.NewReader(output).ReadString('\n')
		if err != nil {
			t.Fatalf("helper startup: %v", err)
		}
		return p, line
	}
	first, line := start()
	if line != "ready\n" {
		t.Fatalf("first: %q", line)
	}
	_, line = start()
	if line != "occupied\n" {
		t.Fatalf("second: %q", line)
	}
	result := do(t, dial(t, path), cli.Request{Op: cli.OpStatus}, nil)
	if !result.OK {
		t.Fatalf("first server displaced: %+v", result)
	}
	first.input.Close()
	if err := first.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("first cleanup: %v", err)
	}
	third, line := start()
	if line != "ready\n" {
		t.Fatalf("takeover: %q", line)
	}
	if result := do(t, dial(t, path), cli.Request{Op: cli.OpStatus}, nil); !result.OK {
		t.Fatalf("takeover: %+v", result)
	}
	third.input.Close()
	if err := third.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
