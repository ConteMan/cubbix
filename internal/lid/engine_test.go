package lid

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSleep struct {
	v               int
	writes          []int
	setErr, zeroErr bool
	badOne, badZero bool
	before          func(int)
}

func (s *fakeSleep) Read() (int, error) { return s.v, nil }
func (s *fakeSleep) Set(v int) error {
	if s.before != nil {
		s.before(v)
	}
	s.writes = append(s.writes, v)
	if s.setErr || v == 0 && s.zeroErr {
		return errors.New("write")
	}
	if v == 1 && s.badOne || v == 0 && s.badZero {
		return nil
	}
	s.v = v
	return nil
}

type fakePower struct {
	kind string
	p    int
	err  error
}

func (p *fakePower) Source() (string, int, error) { return p.kind, p.p, p.err }
func fixture(t *testing.T) (*Engine, *fakeSleep, *fakePower, *time.Time) {
	t.Helper()
	s := &fakeSleep{}
	p := &fakePower{kind: "ac", p: 80}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(s, p, FileJournal{filepath.Join(t.TempDir(), "journal", "session.json")})
	e.Now = func() time.Time { return now }
	e.Start()
	return e, s, p, &now
}
func request(op string) Request { return Request{V: Protocol, ID: "session-1", Op: op} }
func enable(t *testing.T, e *Engine) {
	t.Helper()
	if r := e.Handle(1, request("enable")); !r.OK || r.State != "on-ours" {
		t.Fatalf("enable: %+v", r)
	}
}
func absent(t *testing.T, e *Engine) {
	t.Helper()
	if _, err := e.Store.Load(); !os.IsNotExist(err) {
		t.Fatalf("journal retained: %v", err)
	}
}
func TestJournalBeforeWriteAndReadback(t *testing.T) {
	for _, failure := range []string{"none", "write", "one", "rollback-write", "rollback-read"} {
		t.Run(failure, func(t *testing.T) {
			e, s, _, _ := fixture(t)
			s.before = func(v int) {
				if v == 1 {
					j, err := e.Store.Load()
					if err != nil || j.Previous != 0 || j.MinPercent != 20 {
						t.Fatalf("journal before Set: %+v %v", j, err)
					}
				}
			}
			switch failure {
			case "write":
				s.setErr = true
			case "one":
				s.badOne = true
			case "rollback-write":
				s.badOne = true
				s.zeroErr = true
			case "rollback-read":
				s.badOne = true
				s.badZero = true
				s.before = func(v int) {
					if v == 0 {
						s.v = 1
					}
				}

			}
			r := e.Handle(1, request("enable"))
			if failure == "none" {
				if !r.OK {
					t.Fatal(r)
				}
				e.Handle(1, request("disable"))
				absent(t, e)
			} else if failure == "one" {
				if r.OK || r.Code != "pmset-failed" {
					t.Fatal(r)
				}
				absent(t, e)
			} else {
				if r.OK || r.State != "restore-failed" {
					t.Fatal(r)
				}
				if _, err := e.Store.Load(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
func TestForeignNeverWritten(t *testing.T) {
	e, s, _, _ := fixture(t)
	s.v = 1
	r := e.Handle(1, request("enable"))
	if r.Code != "foreign-setting" {
		t.Fatal(r)
	}
	e.Handle(1, request("disable"))
	if len(s.writes) != 0 {
		t.Fatal(s.writes)
	}
}
func TestPowerGates(t *testing.T) {
	for _, v := range []struct {
		kind string
		p    int
		ok   bool
	}{{"battery", 80, true}, {"battery", 20, false}, {"battery", 15, false}, {"battery", -1, false}, {"ac", 1, true}, {"ac", -1, false}} {
		t.Run(v.kind+stringID(uint64(v.p+1)), func(t *testing.T) {
			e, s, p, _ := fixture(t)
			p.kind = v.kind
			p.p = v.p
			r := e.Handle(1, request("enable"))
			if r.OK != v.ok {
				t.Fatal(r)
			}
			if !v.ok {
				absent(t, e)
				if len(s.writes) > 0 {
					t.Fatal(s.writes)
				}
			}
		})
	}
}
func TestRecoveryTriggers(t *testing.T) {
	for _, trigger := range []string{"deadline", "eof", "lease", "disable", "battery", "unknown", "startup", "boot", "hard-limit"} {
		t.Run(trigger, func(t *testing.T) {
			e, s, p, now := fixture(t)
			r := request("enable")
			end := now.Add(10 * time.Second)
			if trigger == "deadline" {
				r.Deadline = &end
			}
			if out := e.Handle(1, r); !out.OK {
				t.Fatal(out)
			}
			switch trigger {
			case "deadline":
				*now = end
				e.Tick()
			case "eof":
				e.Disconnect(1)
			case "lease":
				*now = now.Add(Lease)
				e.Tick()
			case "disable":
				e.Handle(1, request("disable"))
			case "startup", "boot":
				e.Start()
			case "hard-limit":
				*now = now.Add(HardLimit)
				e.Tick()
			case "battery", "unknown":
				for i := 0; i < 18; i++ {
					*now = now.Add(5 * time.Second)
					if trigger == "battery" {
						p.kind = "battery"
						p.p = 15
					} else {
						p.p = -1
					}
					e.Handle(1, request("ping"))
				}
			}
			if s.v != 0 {
				t.Fatal(s.v)
			}
			absent(t, e)
			if trigger == "hard-limit" && e.code != "max-duration" || trigger == "unknown" && e.code != "battery-unknown" {
				t.Fatal(e.code)
			}
		})
	}
}
func TestRetryAndIdempotentRecovery(t *testing.T) {
	e, s, _, now := fixture(t)
	enable(t, e)
	s.zeroErr = true
	r := e.Handle(1, request("disable"))
	if r.State != "restore-failed" {
		t.Fatal(r)
	}
	s.zeroErr = false
	*now = now.Add(time.Second)
	e.Tick()
	absent(t, e)
	if s.v != 0 {
		t.Fatal(s.v)
	}
	for _, value := range []int{0, 1} {
		e.Store.Save(Journal{})
		s.v = value
		s.writes = nil
		e.Start()
		if len(s.writes) != 1 || s.writes[0] != 0 {
			t.Fatal(s.writes)
		}
		absent(t, e)
	}
}
func TestCrashBoundaries(t *testing.T) {
	for _, point := range []string{"before-journal", "after-journal", "after-enable", "before-restore", "after-restore", "after-readback"} {
		t.Run(point, func(t *testing.T) {
			e, s, _, _ := fixture(t)
			if point != "before-journal" {
				e.Store.Save(Journal{})
			}
			if point == "after-enable" || point == "before-restore" {
				s.v = 1
			}
			e.Start()
			if s.v != 0 {
				t.Fatal(s.v)
			}
			absent(t, e)
			if point != "before-journal" && (len(s.writes) != 1 || s.writes[0] != 0) {
				t.Fatal(s.writes)
			}
		})
	}
}
func TestHardLimitRenewalAndOwnership(t *testing.T) {
	e, _, _, now := fixture(t)
	end := now.Add(48 * time.Hour)
	r := request("enable")
	r.Deadline = &end
	out := e.Handle(1, r)
	if out.Deadline == nil || !out.Deadline.Equal(now.Add(HardLimit)) {
		t.Fatal(out)
	}
	if out = e.Handle(2, r); out.Code != "busy" {
		t.Fatal(out)
	}
	e.Disconnect(2)
	if e.journal == nil {
		t.Fatal("non-owner ended lease")
	}
	*now = now.Add(time.Second)
	e.Handle(1, r)
	if e.journal.Started.Equal(*now) {
		t.Fatal("renewal reset cap")
	}
	e.Disconnect(1)
	enable(t, e)
	e.Handle(1, request("disable"))
	enable(t, e)
}
func TestUnknownSamplesAndUnplug(t *testing.T) {
	for _, reset := range []bool{false, true} {
		e, s, p, now := fixture(t)
		enable(t, e)
		p.kind = "battery"
		for i := 1; i <= 4; i++ {
			if i == 3 && reset {
				p.p = 80
			} else {
				p.p = -1
			}
			for n := 0; n < 6; n++ {
				*now = now.Add(5 * time.Second)
				e.Handle(1, request("ping"))
			}
			if i < 3 && s.v != 1 {
				t.Fatal("early recovery")
			}
		}
		if reset && s.v != 1 {
			t.Fatal("counter did not reset")
		}
		if !reset && (s.v != 0 || e.code != "battery-unknown") {
			t.Fatal(e.code)
		}
	}
}
func TestProtocolValidation(t *testing.T) {
	now := time.Now()
	for _, op := range []string{"hello", "status", "enable", "disable", "ping"} {
		r := request(op)
		if r.Validate(now) != "" {
			t.Fatal(op)
		}
	}
	cases := []Request{request("shell"), {V: 2, ID: "x", Op: "hello"}, {V: 1, ID: "a\nb", Op: "status"}}
	for _, minutes := range []int{-1, MaxMinutes + 1} {
		r := request("enable")
		end := now.Add(time.Duration(minutes) * time.Minute)
		r.Deadline = &end
		cases = append(cases, r)
	}
	r := request("enable")
	r.MinPercent = 9
	cases = append(cases, r)
	for _, r := range cases {
		if r.Validate(now) == "" {
			t.Fatal(r)
		}
	}
}
func TestCapacityValidation(t *testing.T) {
	for _, v := range [][3]int64{{4000, 5000, 80}, {1, 0, -1}, {-1, 100, -1}, {101, 100, -1}, {1, -1, -1}, {20, 100, 20}} {
		p, err := CapacityPercent(v[0], v[1])
		if p != int(v[2]) || (err != nil) != (v[2] < 0) {
			t.Fatal(v, p, err)
		}
	}
}
func TestTemporarySocketAndUntrustedPeer(t *testing.T) {
	e, s, _, _ := fixture(t)
	dir, err := os.MkdirTemp("/tmp", "lid-peer-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "peer.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	server := Server{Engine: e, Verify: func(c *net.UnixConn) error {
		if strings.HasSuffix(c.LocalAddr().String(), "never") {
			return nil
		}
		return ErrUntrusted
	}}
	go func() { done <- server.Serve(ctx, l) }()
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Now().Add(time.Second))
	json.NewEncoder(c).Encode(request("enable"))
	var b [1]byte
	if _, err = c.Read(b[:]); err == nil {
		t.Fatal("untrusted peer accepted")
	}
	c.Close()
	cancel()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if len(s.writes) != 0 {
		t.Fatal(s.writes)
	}
}
func TestMalformedTransport(t *testing.T) {
	for _, payload := range []string{"{broken}\n", strings.Repeat("x", MaxLine+1) + "\n", `{"v":2,"id":"x","op":"hello"}` + "\n"} {
		e, _, _, _ := fixture(t)
		path := filepath.Join(t.TempDir(), "s")
		l, err := Listen(path)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		server := Server{Engine: e, Verify: func(*net.UnixConn) error { return nil }}
		go func() { done <- server.Serve(ctx, l) }()
		c, _ := net.Dial("unix", path)
		c.SetDeadline(time.Now().Add(time.Second))
		c.Write([]byte(payload))
		var resp Response
		json.NewDecoder(c).Decode(&resp)
		c.Close()
		cancel()
		<-done
		if resp.OK {
			t.Fatal(resp)
		}
	}
}

func TestUnplugKeepsJournalAndLease(t *testing.T) {
	e, s, p, now := fixture(t)
	enable(t, e)
	before, err := e.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	p.kind = "battery"
	for range 6 {
		*now = now.Add(5 * time.Second)
		e.Handle(1, request("ping"))
	}
	after, err := e.Store.Load()
	if err != nil || !before.Started.Equal(after.Started) || s.v != 1 || len(s.writes) != 1 {
		t.Fatal("unplug changed the lease")
	}
}
func TestJournalWriteFailureNeverSetsSleep(t *testing.T) {
	e, s, _, _ := fixture(t)
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	e.Store = FileJournal{filepath.Join(path, "session")}
	r := e.Handle(1, request("enable"))
	if r.OK || len(s.writes) > 0 {
		t.Fatal("journal failure changed setting")
	}
}
func TestAllRetryBackoffs(t *testing.T) {
	e, s, _, now := fixture(t)
	enable(t, e)
	s.zeroErr = true
	e.Handle(1, request("disable"))
	for _, delay := range backoff {
		if !e.retryAt.Equal(now.Add(delay)) {
			t.Fatal("retry delay", e.retryAt, delay)
		}
		*now = e.retryAt
		e.Tick()
	}
	s.zeroErr = false
	*now = e.retryAt
	e.Tick()
	absent(t, e)
}

func TestServerRejectCannotEvictLiveLease(t *testing.T) {
	e, _, _, _ := fixture(t)
	dir, err := os.MkdirTemp("/tmp", "lid-owner-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "s")
	listener, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var accepted atomic.Int32
	server := Server{Engine: e, Verify: func(*net.UnixConn) error {
		if accepted.Add(1) == 2 {
			return ErrUntrusted
		}
		return nil
	}}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	verify := func(*net.UnixConn) error { return nil }
	first, err := Dial(path, verify)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if r, err := first.Call(request("enable")); err != nil || !r.OK {
		t.Fatal(r, err)
	}
	rejected, err := Dial(path, verify)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rejected.Call(request("enable")); err == nil {
		t.Fatal("untrusted connection accepted")
	}
	rejected.Close()
	if r, err := first.Call(request("ping")); err != nil || r.State != "on-ours" {
		t.Fatal("untrusted peer evicted owner", r, err)
	}
	second, err := Dial(path, verify)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if r, err := second.Call(request("enable")); err != nil || r.Code != "busy" {
		t.Fatal("second owner accepted", r, err)
	}
	first.Close()
	deadline := time.Now().Add(time.Second)
	for {
		r, err := second.Call(request("enable"))
		if err != nil {
			t.Fatal(err)
		}
		if r.OK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owner did not transfer", r)
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestUnsafeJournalCannotOverwriteForeignSetting(t *testing.T) {
	e, s, _, _ := fixture(t)
	dir := filepath.Dir(e.Store.(FileJournal).Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	s.v = 1
	if err := e.Start(); err == nil {
		t.Fatal("unsafe directory accepted")
	}
	if len(s.writes) != 0 || s.v != 1 {
		t.Fatal("unproven journal changed external setting")
	}
}
func TestTrustedCorruptJournalStillRecovers(t *testing.T) {
	e, s, _, _ := fixture(t)
	path := e.Store.(FileJournal).Path
	os.MkdirAll(filepath.Dir(path), 0700)
	if err := os.WriteFile(path, []byte("{truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	s.v = 1
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if s.v != 0 {
		t.Fatal("owned liability ignored")
	}
	absent(t, e)
}
