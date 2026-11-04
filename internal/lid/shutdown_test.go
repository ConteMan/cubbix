package lid

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShutdownRestoresWithoutConnectionAndKeepsFailedJournal(t *testing.T) {
	for _, fail := range []bool{false, true} {
		e, s, _, _ := fixture(t)
		enable(t, e)
		s.zeroErr = fail
		// Startup recovery and a disconnected failed restore have no owner.
		e.owner = 0
		if err := e.Shutdown(); (err != nil) != fail {
			t.Fatal(err)
		}
		if fail {
			if _, err := e.Store.Load(); err != nil || s.v != 1 {
				t.Fatal("lost pending recovery", err)
			}
			s.zeroErr = false
			if err := e.Shutdown(); err != nil {
				t.Fatal(err)
			}
		}
		if s.v != 0 {
			t.Fatal("not restored")
		}
		absent(t, e)
	}
}

func TestServerCancellationWaitsForRestoreAndClosesIdlePeers(t *testing.T) {
	e, s, _, _ := fixture(t)
	dir, err := os.MkdirTemp("/tmp", "lid-stop-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	l, err := Listen(filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := Server{Engine: e, Verify: func(*net.UnixConn) error { return nil }}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, l) }()
	client, err := Dial(filepath.Join(dir, "s"), func(*net.UnixConn) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if r, err := client.Call(request("enable")); err != nil || !r.OK {
		t.Fatal(r, err)
	}
	restoring, release := make(chan struct{}), make(chan struct{})
	// Synchronize fake mutation with the server through the engine mutex.
	e.mu.Lock()
	s.before = func(v int) {
		if v == 0 {
			close(restoring)
			<-release
		}
	}
	e.mu.Unlock()
	cancel()
	select {
	case <-restoring:
	case <-time.After(time.Second):
		t.Fatal("restore not started")
	}
	select {
	case err := <-done:
		t.Fatal("returned before recovery", err)
	default:
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown hung")
	}
	if s.v != 0 {
		t.Fatal("setting not restored")
	}
	absent(t, e)
}
