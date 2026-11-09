package lid

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The tests in this file use only injected power settings and a temporary
// journal. No system setting is read or written.

func watchRequest(deadline *time.Time, min int) Request {
	return Request{V: Protocol, ID: "session-1", Op: "enable", AdoptForeign: true, Deadline: deadline, MinPercent: min}
}

func enterWatch(t *testing.T, e *Engine, s *fakeSleep, deadline *time.Time) Response {
	t.Helper()
	s.v = 1
	r := e.Handle(1, watchRequest(deadline, 0))
	if !r.OK || r.State != "watching" {
		t.Fatalf("watch was not accepted: %+v", r)
	}
	return r
}

func storedJournal(t *testing.T, e *Engine) *Journal {
	t.Helper()
	j, err := e.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func noJournal(t *testing.T, e *Engine) {
	t.Helper()
	if _, err := e.Store.Load(); !os.IsNotExist(err) {
		t.Fatalf("journal retained: %v", err)
	}
}

func countWrites(writes []int, value int) int {
	n := 0
	for _, v := range writes {
		if v == value {
			n++
		}
	}
	return n
}

func advance(p *time.Time, d time.Duration) { *p = p.Add(d) }

// heartbeat stands in for the app's 5s ping when a test jumps the fake clock
// by more than a lease. Without it the lease, not the behaviour under test,
// would end the session.
func heartbeat(e *Engine, now *time.Time) { e.lastRequest = *now }

func TestWatchZeroWritesAndLeaseFields(t *testing.T) {
	e, s, _, now := fixture(t)
	start := *now
	enterWatch(t, e, s, nil)
	if len(s.writes) != 0 {
		t.Fatal("watch wrote the setting", s.writes)
	}
	j := storedJournal(t, e)
	if j.Writer != "foreign" || !j.Started.Equal(start) || j.Deadline != nil || j.MinPercent != 20 || j.Previous != 0 {
		t.Fatalf("incomplete watch journal: %+v", j)
	}
	if j.Rewrites != 0 || j.WindowCount != 0 || !j.LastRewrite.IsZero() {
		t.Fatal("watch consumed a rewrite budget", j)
	}
	// Regression: zero Started or lastRequest used to end the lease at the
	// very next tick (hard limit / 15s lease).
	advance(now, time.Second)
	e.Tick()
	if r := e.Handle(1, request("ping")); r.State != "watching" {
		t.Fatal("watch ended at the first tick", r)
	}
	storedJournal(t, e)
	if len(s.writes) != 0 {
		t.Fatal("watch wrote the setting later", s.writes)
	}
}

func TestWatchRenewalOnlyMovesDeadline(t *testing.T) {
	e, s, _, now := fixture(t)
	enterWatch(t, e, s, nil)
	before := storedJournal(t, e)
	advance(now, 10*time.Second)
	end := now.Add(2 * time.Hour)
	r := e.Handle(1, watchRequest(&end, 0))
	if !r.OK || r.State != "watching" {
		t.Fatal(r)
	}
	after := storedJournal(t, e)
	if after.Deadline == nil || !after.Deadline.Equal(end) || !after.Started.Equal(before.Started) ||
		after.Rewrites != before.Rewrites || after.WindowCount != before.WindowCount || after.Writer != "foreign" {
		t.Fatalf("renewal rewrote the lease: %+v", after)
	}
	if len(s.writes) != 0 {
		t.Fatal("renewal wrote the setting", s.writes)
	}
}

func TestOldClientCannotEnterWatch(t *testing.T) {
	e, s, _, _ := fixture(t)
	s.v = 1
	r := e.Handle(1, request("enable"))
	if r.OK || r.Code != "foreign-setting" {
		t.Fatal("old client was silently widened", r)
	}
	if len(s.writes) != 0 {
		t.Fatal(s.writes)
	}
	noJournal(t, e)
}

func TestTakeoverDeclaresOwnershipBeforeWriting(t *testing.T) {
	e, s, _, now := fixture(t)
	start := *now
	end := start.Add(4 * time.Hour)
	enterWatch(t, e, s, &end)
	advance(now, time.Hour)
	takeoverAt := *now
	heartbeat(e, now)
	var checked bool
	s.before = func(v int) {
		if v != 1 {
			return
		}
		checked = true
		j := storedJournal(t, e)
		if j.Writer != "ours" || !j.Started.Equal(takeoverAt) || j.Deadline == nil || !j.Deadline.Equal(end) {
			t.Fatalf("journal was not rewritten before Set(1): %+v", j)
		}
	}
	s.v = 0
	r := e.Handle(1, request("ping"))
	if !checked || !r.OK || r.State != "on-ours" || r.Value == nil || *r.Value != 1 || r.Detail != "adopted" {
		t.Fatalf("takeover failed: checked=%v %+v", checked, r)
	}
	if countWrites(s.writes, 1) != 1 || countWrites(s.writes, 0) != 0 {
		t.Fatal("takeover wrote more than once", s.writes)
	}
	j := storedJournal(t, e)
	if j.Writer != "ours" || !j.Started.Equal(takeoverAt) || j.Deadline == nil || !j.Deadline.Equal(end) || j.Rewrites != 1 {
		t.Fatalf("takeover moved the deadline or did not count: %+v", j)
	}
}

func TestTakeoverFailureKeepsOwnershipAndRetries(t *testing.T) {
	for _, failure := range []string{"write", "readback"} {
		t.Run(failure, func(t *testing.T) {
			e, s, _, now := fixture(t)
			enterWatch(t, e, s, nil)
			advance(now, time.Hour)
			heartbeat(e, now)
			s.v = 0
			if failure == "write" {
				s.setErr = true
			} else {
				s.badOne = true
			}
			e.Handle(1, request("ping"))
			j := storedJournal(t, e)
			if j.Writer != "ours" || e.state != "on-ours" || e.rewriteFails != 1 {
				t.Fatalf("declared ownership was dropped: %+v %s %d", j, e.state, e.rewriteFails)
			}
			if e.detail != "takeover-write-failed" && e.detail != "takeover-readback-failed" {
				t.Fatal("missing takeover failure detail", e.detail)
			}
			// A restart of an owned journal converges to 0 with no journal.
			s.setErr, s.badOne, s.v = false, false, 1
			if err := e.Start(); err != nil {
				t.Fatal(err)
			}
			if s.v != 0 {
				t.Fatal("restart did not restore", s.v)
			}
			noJournal(t, e)
		})
	}
}

// Budget exhaustion may only release an owned journal after a definite 0.
func TestBudgetExhaustionNeedsConfirmedZero(t *testing.T) {
	t.Run("confirmed-zero", func(t *testing.T) {
		e, s, _, now := fixture(t)
		enable(t, e)
		for i := range 3 {
			advance(now, RewriteGap)
			s.v = 0
			r := e.Handle(1, request("ping"))
			if !r.OK || r.State != "on-ours" {
				t.Fatal("rewrite ended the lease early", i, r)
			}
		}
		if countWrites(s.writes, 1) != 4 { // one enable plus three rewrites
			t.Fatal("unexpected write count", s.writes)
		}
		// The fourth 0 in the same window exhausts the budget.
		advance(now, RewriteGap)
		s.v = 0
		r := e.Handle(1, request("ping"))
		if r.State != "off" || r.Code != "contested" || e.code != "contested" {
			t.Fatal("budget exhaustion did not converge", r)
		}
		noJournal(t, e)
		if countWrites(s.writes, 1) != 4 || countWrites(s.writes, 0) != 0 {
			t.Fatal("contested wrote a value", s.writes)
		}
	})
	t.Run("readback-one", func(t *testing.T) {
		e, s, _, now := fixture(t)
		enable(t, e)
		j := *e.journal
		j.WindowAt, j.WindowCount, j.Rewrites = *now, RewriteWindowLimit, RewriteSessionLimit
		j.LastRewrite = now.Add(-RewriteGap)
		if err := e.Store.Save(j); err != nil {
			t.Fatal(err)
		}
		e.journal = &j
		// First read (the observation) is 0; the confirmation read is 1.
		e.Sleep = &readSequence{fakeSleep: s, values: []int{0, 1}}
		e.Handle(1, request("ping"))
		if e.state != "on-ours" || e.journal == nil || e.code == "contested" {
			t.Fatal("a 1 lost its owner", e.state, e.code)
		}
		storedJournal(t, e)
	})
	t.Run("read-failure", func(t *testing.T) {
		e, s, _, now := fixture(t)
		enable(t, e)
		j := *e.journal
		j.WindowAt, j.WindowCount, j.Rewrites = *now, RewriteWindowLimit, RewriteSessionLimit
		j.LastRewrite = now.Add(-RewriteGap)
		if err := e.Store.Save(j); err != nil {
			t.Fatal(err)
		}
		e.journal = &j
		e.Sleep = &readSequence{fakeSleep: s, values: []int{0, -1, -1}}
		e.Handle(1, request("ping"))
		if e.journal == nil || e.state != "on-ours" || e.readFailures != 1 {
			t.Fatal("a failed confirmation dropped the journal", e.state, e.readFailures)
		}
		storedJournal(t, e)
	})
}

type readSequence struct {
	*fakeSleep
	values []int
	err    error
}

func (s *readSequence) Read() (int, error) {
	if len(s.values) > 0 {
		v := s.values[0]
		s.values = s.values[1:]
		if v < 0 {
			return -1, errors.New("read")
		}
		return v, nil
	}
	return s.fakeSleep.Read()
}

func TestRewriteKeepsStartAndDeadline(t *testing.T) {
	e, s, _, now := fixture(t)
	start := *now
	end := start.Add(3 * time.Hour)
	r := request("enable")
	r.Deadline = &end
	if out := e.Handle(1, r); !out.OK {
		t.Fatal(out)
	}
	before := storedJournal(t, e)
	advance(now, time.Minute)
	heartbeat(e, now)
	s.v = 0
	r = request("ping")
	out := e.Handle(1, r)
	if out.Detail != "external-release" || out.State != "on-ours" || out.Rewrites != 1 {
		t.Fatalf("rewrite did not converge: %+v", out)
	}
	after := storedJournal(t, e)
	if !after.Started.Equal(before.Started) || after.Deadline == nil || !after.Deadline.Equal(end) || after.Rewrites != 1 {
		t.Fatalf("rewrite moved the 12h cap: %+v", after)
	}
	if countWrites(s.writes, 1) != 2 || countWrites(s.writes, 0) != 0 {
		t.Fatal("rewrite restored instead of writing 1", s.writes)
	}
}

type failingOneSleep struct {
	*fakeSleep
	fails int
}

func (s *failingOneSleep) Set(v int) error {
	if v == 1 && s.fails > 0 {
		s.fails--
		s.writes = append(s.writes, v)
		return errors.New("write")
	}
	return s.fakeSleep.Set(v)
}

func TestContinuousWriteFailuresEndOnlyAfterAConfirmedZero(t *testing.T) {
	e, s, _, now := fixture(t)
	enable(t, e)
	failing := &failingOneSleep{fakeSleep: s, fails: 3}
	e.Sleep = failing
	for i := range 3 {
		advance(now, RewriteGap)
		s.v = 0
		e.Handle(1, request("ping"))
		if e.state != "on-ours" || e.journal == nil || e.rewriteFails != i+1 {
			t.Fatal("failed rewrite dropped the liability", i, e.state, e.rewriteFails)
		}
		storedJournal(t, e)
	}
	advance(now, RewriteGap)
	s.v = 0
	r := e.Handle(1, request("ping"))
	if r.State != "off" || r.Code != "external-release-failed" {
		t.Fatal("continuous failure did not converge", r)
	}
	noJournal(t, e)
	stored := countWrites(s.writes, 0)
	if stored != 0 {
		t.Fatal("ending a confirmed 0 wrote another 0", s.writes)
	}
}

func TestRewriteGapOnlySuppressesTheSecondWrite(t *testing.T) {
	e, s, _, now := fixture(t)
	enable(t, e)
	s.v = 0
	e.Handle(1, request("ping"))
	advance(now, 5*time.Second)
	s.v = 0
	r := e.Handle(1, request("ping"))
	if r.State != "on-ours" || e.journal == nil || e.code == "contested" {
		t.Fatal("a short gap ended the lease", r)
	}
	if countWrites(s.writes, 1) != 2 {
		t.Fatal("gap wrote twice", s.writes)
	}
	storedJournal(t, e)
}

func TestRewriteSessionLimit(t *testing.T) {
	e, s, _, now := fixture(t)
	enable(t, e)
	for i := range RewriteSessionLimit {
		// A window longer than RewriteWindow resets the window counter but
		// never the per-lease total. Keep the 15s lease alive explicitly.
		advance(now, RewriteWindow+RewriteGap)
		e.lastRequest = *now
		s.v = 0
		out := e.Handle(1, request("ping"))
		if !out.OK || out.State != "on-ours" {
			t.Fatal("session limit hit too early", i, out)
		}
	}
	advance(now, RewriteWindow+RewriteGap)
	e.lastRequest = *now
	s.v = 0
	r := e.Handle(1, request("ping"))
	if r.State != "off" || r.Code != "contested" {
		t.Fatal("session limit did not stop rewrites", r)
	}
	noJournal(t, e)
	if countWrites(s.writes, 1) != RewriteSessionLimit+1 {
		t.Fatal("unexpected write count", s.writes)
	}
}

func TestObserveIntervalAndSingleReadPerPing(t *testing.T) {
	e, s, _, now := fixture(t)
	before := s.reads
	for range 10 {
		advance(now, ObserveInterval)
		e.Tick()
	}
	if got := s.reads - before; got != 10 {
		t.Fatal("1s ticks read", got, "times")
	}
	if e.state != "off" {
		t.Fatal("observation changed the idle state", e.state)
	}
	before = s.reads
	e.Handle(1, request("ping"))
	if got := s.reads - before; got != 1 {
		t.Fatal("a ping read", got, "times; tick and refresh must share lastObserve")
	}
}

// A foreign writer's 1 is never ours to clear, on any termination path.
func TestEndPathsRespectWriter(t *testing.T) {
	triggers := []string{"disable", "disconnect", "lease", "deadline", "hard-limit", "battery-low", "battery-unknown"}
	for _, mode := range []string{"watching", "on-ours"} {
		for _, trigger := range triggers {
			t.Run(mode+"/"+trigger, func(t *testing.T) {
				e, s, p, now := fixture(t)
				var deadline *time.Time
				if trigger == "deadline" {
					end := now.Add(30 * time.Second)
					deadline = &end
				}
				if mode == "watching" {
					enterWatch(t, e, s, deadline)
				} else {
					r := request("enable")
					r.Deadline = deadline
					if out := e.Handle(1, r); !out.OK {
						t.Fatal(out)
					}
				}
				switch trigger {
				case "disable":
					e.Handle(1, request("disable"))
				case "disconnect":
					e.Disconnect(1)
				case "lease":
					advance(now, Lease)
					e.Tick()
				case "deadline":
					*now = *deadline
					e.Tick()
				case "hard-limit":
					advance(now, HardLimit)
					e.Tick()
				case "battery-low":
					p.kind, p.p = "battery", 15
					for range 6 {
						advance(now, 5*time.Second)
						e.Handle(1, request("ping"))
					}
				case "battery-unknown":
					p.p = -1
					for range 18 {
						advance(now, 5*time.Second)
						e.Handle(1, request("ping"))
					}
				}
				noJournal(t, e)
				if e.state != "off" {
					t.Fatal("termination did not converge", e.state)
				}
				if mode == "watching" {
					if len(s.writes) != 0 || s.v != 1 {
						t.Fatal("a foreign 1 was touched", s.writes, s.v)
					}
				} else if s.v != 0 || countWrites(s.writes, 0) != 1 {
					t.Fatal("an owned 1 was not restored", s.writes, s.v)
				}
			})
		}
	}
}

func TestObservationRewritesInsteadOfRestoring(t *testing.T) {
	e, s, _, _ := fixture(t)
	enable(t, e)
	s.v = 0
	r := e.Handle(1, request("ping"))
	if r.State != "on-ours" || countWrites(s.writes, 1) != 2 || countWrites(s.writes, 0) != 0 {
		t.Fatal("readback mismatch still restored", r, s.writes)
	}
	storedJournal(t, e)
}

func TestReadFailuresOnlyCountForOwnedLeases(t *testing.T) {
	for _, mode := range []string{"watching", "on-ours"} {
		t.Run(mode, func(t *testing.T) {
			e, s, _, _ := fixture(t)
			if mode == "watching" {
				enterWatch(t, e, s, nil)
			} else {
				enable(t, e)
				s.v = 0 // let the restart's readback confirm 0
			}
			e.Sleep = &failingObservation{fakeSleep: s, failures: readFailureLimit}
			for range readFailureLimit {
				e.Handle(1, request("ping"))
			}
			if mode == "watching" {
				if e.state != "watching" || len(s.writes) != 0 || e.journal == nil {
					t.Fatal("watch was ended by read failures", e.state, s.writes)
				}
			} else if e.state != "off" || countWrites(s.writes, 0) != 1 {
				t.Fatal("owned read failures did not restore", e.state, s.writes)
			}
		})
	}
}

func TestStartSplitsOwnership(t *testing.T) {
	t.Run("ours", func(t *testing.T) {
		e, s, _, _ := fixture(t)
		if err := e.Store.Save(Journal{Writer: "ours"}); err != nil {
			t.Fatal(err)
		}
		s.v = 1
		if err := e.Start(); err != nil {
			t.Fatal(err)
		}
		if s.v != 0 || countWrites(s.writes, 0) != 1 {
			t.Fatal("owned journal was not restored", s.writes)
		}
		noJournal(t, e)
	})
	t.Run("foreign", func(t *testing.T) {
		e, s, _, _ := fixture(t)
		if err := e.Store.Save(Journal{Writer: "foreign"}); err != nil {
			t.Fatal(err)
		}
		s.v = 1
		if err := e.Start(); err != nil {
			t.Fatal(err)
		}
		if len(s.writes) != 0 || s.v != 1 || e.state != "on-foreign" {
			t.Fatal("foreign journal was modified", s.writes, s.v, e.state)
		}
		noJournal(t, e)
	})
	t.Run("corrupt-legacy", func(t *testing.T) {
		e, s, _, _ := fixture(t)
		path := e.Store.(FileJournal).Path
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{truncated"), 0600); err != nil {
			t.Fatal(err)
		}
		s.v = 1
		if err := e.Start(); err != nil {
			t.Fatal(err)
		}
		if s.v != 0 || countWrites(s.writes, 0) != 1 {
			t.Fatal("corrupt legacy journal was not treated as ours", s.writes)
		}
		noJournal(t, e)
	})
}

func TestCrashBoundariesConverge(t *testing.T) {
	for _, point := range []string{"save-then-write-fails", "write-then-readback-fails", "write-one-then-restart"} {
		t.Run(point, func(t *testing.T) {
			e, s, _, now := fixture(t)
			enterWatch(t, e, s, nil)
			advance(now, time.Hour)
			heartbeat(e, now)
			s.v = 0
			switch point {
			case "save-then-write-fails":
				s.setErr = true
			case "write-then-readback-fails":
				s.badOne = true
			case "write-one-then-restart":
				s.before = func(v int) {
					if v == 1 {
						s.v = 1
					}
				}
			}
			e.Handle(1, request("ping"))
			j := storedJournal(t, e)
			if j.Writer != "ours" {
				t.Fatal("ownership was not durable", j)
			}
			// Either the value is 0 with an owned journal that will restore on
			// restart, or it is 1 with an owned journal. Never a 1 without one.
			if s.v != 0 && s.v != 1 {
				t.Fatal("unexpected value", s.v)
			}
			s.setErr, s.badOne, s.before = false, false, nil
			if err := e.Start(); err != nil {
				t.Fatal(err)
			}
			if s.v != 0 {
				t.Fatal("restart did not converge to 0", s.v)
			}
			noJournal(t, e)
		})
	}
}

// The full calibration timeline: external writes at 09:50, 11:53, 13:50 and
// 21:13, with a forced-discharge segment whose charge crosses the limit.
func TestAlDenteCalibration(t *testing.T) {
	const (
		enableAt  = 300   // 09:55
		discharge = 2400  // 10:30
		takeover  = 7380  // 11:53
		recharge  = 10800 // 12:50
		hold      = 14400 // 13:50
		release   = 40980 // 21:13
		finish    = 41000
		step      = 5
	)
	for _, tc := range []struct {
		name    string
		adapter AdapterState
		start   string // "watching" or "owned"
	}{
		{"attached-watching", AdapterAttached, "watching"},
		{"absent-watching", AdapterAbsent, "watching"},
		{"absent-owned", AdapterAbsent, "owned"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, s, p, now := fixture(t)
			p.adapter = tc.adapter
			start := *now
			endedCode := ""
			if tc.start == "watching" {
				s.v = 1
			}
			for tick := int64(0); tick <= finish; tick += step {
				*now = start.Add(time.Duration(tick) * time.Second)
				switch tick {
				case 0:
					if tc.start == "watching" {
						e.Handle(1, request("ping"))
					} else if r := e.Handle(1, request("enable")); !r.OK {
						t.Fatal("initial enable failed", r)
					}
					continue
				case enableAt:
					if tc.start == "watching" && !e.Handle(1, watchRequest(nil, 0)).OK {
						t.Fatal("enable{adopt} failed")
					}
					continue
				case discharge:
					p.kind, p.p = "battery", 4
				case takeover:
					s.v = 0
				case recharge:
					p.kind, p.p = "ac", 80
				case hold:
					s.v = 1
				case release:
					s.v = 0
				}
				e.Handle(1, request("ping"))
				if endedCode == "" && e.journal == nil && (e.code == "battery-low" || e.code == "battery-unknown" || e.code == "contested") {
					endedCode = e.code
				}
			}
			switch tc.name {
			case "attached-watching":
				if countWrites(s.writes, 1) != 2 || countWrites(s.writes, 0) != 0 {
					t.Fatal("calibration wrote an unexpected value", s.writes)
				}
				if e.state != "on-ours" || e.code == "contested" || e.journal == nil {
					t.Fatal("calibration did not stay owned", e.state, e.code)
				}
				j := storedJournal(t, e)
				if !j.Started.Equal(start.Add(takeover*time.Second)) || j.Deadline != nil || j.Rewrites != 2 {
					t.Fatalf("takeover/rewrite moved the lease: %+v", j)
				}
			case "absent-watching":
				if len(s.writes) != 0 || endedCode != "battery-low" || e.journal != nil {
					t.Fatal("unplugged watch did not drop cleanly", s.writes, endedCode)
				}
			case "absent-owned":
				if endedCode != "battery-low" || countWrites(s.writes, 0) != 1 || s.v != 0 {
					t.Fatal("unplugged owned lease did not restore", s.writes, endedCode)
				}
				noJournal(t, e)
			}
		})
	}
}

func TestTugOfWarStopsAtContested(t *testing.T) {
	e, s, _, now := fixture(t)
	enable(t, e)
	for range 150 { // 5 minutes of external 0 writes, one every two seconds
		advance(now, 2*time.Second)
		s.v = 0
		e.Handle(1, request("ping"))
	}
	if e.code != "contested" || e.state != "off" {
		t.Fatal("tug of war did not degrade", e.state, e.code)
	}
	noJournal(t, e)
	before := len(s.writes)
	for range 20 {
		advance(now, 2*time.Second)
		s.v = 0
		e.Handle(1, request("ping"))
	}
	if len(s.writes) != before {
		t.Fatal("writes resumed after contested", s.writes)
	}
}
