package lid

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type failingObservation struct {
	*fakeSleep
	failures int
}

func (s *failingObservation) Read() (int, error) {
	if s.failures > 0 {
		s.failures--
		return -1, context.DeadlineExceeded
	}
	return s.fakeSleep.Read()
}

func TestOwnedLeaseReadFailuresAreBounded(t *testing.T) {
	for _, op := range []string{"ping", "enable"} {
		t.Run(op, func(t *testing.T) {
			e, sleep, _, now := fixture(t)
			enable(t, e)
			s := &failingObservation{fakeSleep: sleep, failures: 2}
			e.Sleep = s
			for range 2 {
				*now = now.Add(5 * time.Second)
				r := e.Handle(1, request(op))
				if !r.OK || r.State != "on-ours" || r.Code != "pmset-failed" || len(sleep.writes) != 1 || e.journal == nil {
					t.Fatal("transient read failure ended lease", r, sleep.writes)
				}
			}
			*now = now.Add(5 * time.Second)
			if r := e.Handle(1, request("ping")); !r.OK || r.Code != "ok" || r.Value == nil || *r.Value != 1 {
				t.Fatal("read did not recover", r)
			}
			// Success must reset the consecutive failure counter.
			s.failures = 3
			for i := 0; i < 3; i++ {
				*now = now.Add(5 * time.Second)
				r := e.Handle(1, request("ping"))
				if i < 2 && (r.State != "on-ours" || len(sleep.writes) != 1) {
					t.Fatal("counter did not reset", r, sleep.writes)
				}
				if i == 2 && (r.State != "off" || sleep.v != 0 || e.journal != nil || r.Code != "pmset-failed") {
					t.Fatal("persistent read failure was unbounded", r)
				}
			}
		})
	}
}

func TestPMSetReadTimeoutHasIPCHeadroom(t *testing.T) {
	if CallTimeout < 2*ReadTimeout+4*time.Second || CallTimeout >= Lease {
		t.Fatal("IPC budget must cover both enable reads and leave heartbeat headroom")
	}
	p := &PMSet{Run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < 4*time.Second {
			t.Error("read timeout is too short")
		}
		return nil, errors.New("injected")
	}}
	p.Read()
}

func TestPersistentReadFailureKeepsUnconfirmedRecoveryJournal(t *testing.T) {
	e, sleep, _, now := fixture(t)
	enable(t, e)
	e.Sleep = &failingObservation{fakeSleep: sleep, failures: 4}
	var r Response
	for i := range 3 {
		*now = now.Add(ObserveInterval)
		r = e.Handle(1, request("ping"))
		if i < 2 && (r.State != "on-ours" || r.Code != "pmset-failed") {
			t.Fatal("bounded read failures ended the lease early", r)
		}
	}
	if r.OK || r.State != "restore-failed" || r.Code != "restore-failed" || e.journal == nil {
		t.Fatal("unconfirmed recovery lost its liability", r)
	}
	if _, err := e.Store.Load(); err != nil {
		t.Fatal("durable journal was removed", err)
	}
}

func TestSlowObservationKeepsConnectionAndLease(t *testing.T) {
	e, sleep, _, _ := fixture(t)
	enable(t, e)
	first := true
	e.Sleep = &PMSet{Run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		if first {
			first = false
			select {
			case <-time.After(2200 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return []byte("SleepDisabled 1"), nil
	}}
	// Use only a short-lived local socket and injected reads/writes.
	dir, err := os.MkdirTemp("", "cbread")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "helper.sock")
	listener, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	server := &Server{Engine: e, Verify: func(*net.UnixConn) error { return nil }}
	go func() { done <- server.Serve(ctx, listener) }()
	var client *Client
	t.Cleanup(func() {
		// Restore the fake setter before the server's shutdown recovery.
		e.mu.Lock()
		e.Sleep = sleep
		e.mu.Unlock()
		cancel()
		if client != nil {
			client.Close()
		}
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	client, err = Dial(path, func(*net.UnixConn) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	r, err := client.Call(request("hello"))
	if err != nil || r.State != "on-ours" || r.Value == nil || *r.Value != 1 {
		t.Fatal("slow read disconnected the owner", r, err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(sleep.writes) != 1 || e.journal == nil {
		t.Fatal("slow observation restored the lease", sleep.writes)
	}
}
