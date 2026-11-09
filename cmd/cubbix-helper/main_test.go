package main

import (
	"context"
	"github.com/ConteMan/cubbix/internal/lid"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

type signalSleep struct {
	value    int
	restored string
}

func (s *signalSleep) Read() (int, error) { return s.value, nil }
func (s *signalSleep) Set(v int) error {
	s.value = v
	if v == 0 {
		return os.WriteFile(s.restored, []byte("restored"), 0600)
	}
	return nil
}

type signalPower struct{}

func (signalPower) Sample() (lid.PowerSample, error) {
	return lid.PowerSample{Kind: "ac", Percent: 80}, nil
}

// Only this isolated test child receives SIGTERM; it has fake power operations,
// a temporary socket and journal, and no production helper startup path.
func TestHelperSignalFixture(t *testing.T) {
	dir := os.Getenv("CUBBIX_TEST_SIGNAL_DIR")
	if dir == "" {
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()
	stop := helperExitWatchdog(ctx, time.Second, func() { os.Exit(7) })
	defer stop()
	sleep := &signalSleep{restored: filepath.Join(dir, "restored")}
	engine := lid.NewEngine(sleep, signalPower{}, lid.FileJournal{Path: filepath.Join(dir, "journal", "session")})
	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}
	if r := engine.Handle(1, lid.Request{V: lid.Protocol, ID: "test", Op: "enable"}); !r.OK {
		t.Fatal(r)
	}
	l, err := lid.Listen(filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ready"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	server := lid.Server{Engine: engine, Verify: func(*net.UnixConn) error { return nil }}
	if err := server.Serve(ctx, l); err != nil {
		t.Fatal(err)
	}
}

func TestHelperSIGTERMRestoresBeforeProcessExit(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "lid-signal-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHelperSignalFixture$")
	cmd.Env = append(os.Environ(), "CUBBIX_TEST_SIGNAL_DIR="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	until := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			break
		}
		if time.Now().After(until) {
			t.Fatal("child not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "restored")); err != nil {
		t.Fatal("exited before restoration", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "journal", "session")); !os.IsNotExist(err) {
		t.Fatal("journal retained after confirmed restore", err)
	}
}

func TestHelperExitWatchdogBoundsStuckShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan struct{})
	stop := helperExitWatchdog(ctx, 20*time.Millisecond, func() { close(exited) })
	defer stop()
	select {
	case <-exited:
		t.Fatal("exited before cancellation")
	default:
	}
	cancel()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("watchdog did not bound shutdown")
	}
}
