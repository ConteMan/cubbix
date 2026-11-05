package main

import (
	"context"
	"testing"

	"github.com/ConteMan/cubbix/internal/lid"
)

type lidFailingObservation struct {
	*lidTestSleep
	failures int
}

func (s *lidFailingObservation) Read() (int, error) {
	if s.failures > 0 {
		s.failures--
		return -1, context.DeadlineExceeded
	}
	return s.lidTestSleep.Read()
}

func TestLidReadGraceAcrossRenewalAndPolling(t *testing.T) {
	for _, mode := range []string{"screen", "tasks"} {
		t.Run(mode, func(t *testing.T) {
			a, sleep, _, _, conn := lidTestApp(t)
			a.config.AwakeMode = mode
			a.toggleLid()
			s := &lidFailingObservation{lidTestSleep: sleep, failures: 2}
			conn.engine.Sleep = s
			if err := a.setDuration(120); err != nil {
				t.Fatal("renewal rejected a transient observation", err)
			}
			a.pollLid()
			if !a.awake.snapshot().Session.Lid || len(sleep.writes) != 1 || a.lid.snapshot().state != lidOn {
				t.Fatal("grace period ended the lease")
			}
			a.pollLid()
			if a.lid.snapshot().response.Code != "ok" || a.lid.snapshot().value != 1 {
				t.Fatal("read recovery was not reflected")
			}
			child := a.awake.child
			s.failures = 3
			for range 3 {
				a.pollLid()
			}
			if !a.awake.snapshot().Active || a.awake.snapshot().Session.Lid || a.awake.child != child || sleep.value != 0 || a.lid.markerExists() {
				t.Fatal("persistent failure did not degrade only lid", a.awake.snapshot())
			}
		})
	}
}

func TestLidUnknownPollKeepsBothAssertions(t *testing.T) {
	for _, state := range []string{"on-ours", "off"} {
		for _, code := range []string{"ok", "pmset-failed"} {
			t.Run(state+"/"+code, func(t *testing.T) {
				a, sleep, _, _, conn := lidTestApp(t)
				a.toggleLid()
				before := a.awake.snapshot()
				child := a.awake.child
				unknown := -1
				// Inject an old or delayed helper's inconclusive observation.
				a.lid.apply(lid.Response{OK: true, State: state, Code: code, Value: &unknown, Helper: lid.Version})
				a.finishLidPoll()
				if a.awake.snapshot() != before || a.awake.child != child || sleep.value != 1 || len(sleep.writes) != 1 || !a.lid.markerExists() {
					t.Fatal("one unknown observation stopped an assertion", a.awake.snapshot(), sleep.writes)
				}
				// The next successful ping converges without a new enable.
				a.lid.connection = conn
				a.pollLid()
				if !a.awake.snapshot().Session.Lid || a.lid.snapshot().value != 1 || len(sleep.writes) != 1 {
					t.Fatal("read recovery changed the lease")
				}
			})
		}
	}
}

func TestLidFailureOnlyDegradesLid(t *testing.T) {
	for _, mode := range []string{"screen", "tasks"} {
		for _, failure := range []string{"off", "disconnect", "restore-failed"} {
			t.Run(mode+"/"+failure, func(t *testing.T) {
				a, sleep, _, _, conn := lidTestApp(t)
				a.config.AwakeMode = mode
				a.toggleLid()
				before := a.awake.snapshot()
				child := a.awake.child
				switch failure {
				case "disconnect":
					conn.gone = true
				case "off":
					conn.engine.Disconnect(1)
				case "restore-failed":
					sleep.failZero = true
					conn.engine.Disconnect(1)
				}
				a.pollLid()
				after := a.awake.snapshot()
				before.Session.Lid = false
				if after != before || a.awake.child != child || a.config.AwakeSession == nil || a.config.AwakeSession.Lid || !a.config.AwakeLid {
					t.Fatal("lid failure stopped or replaced caffeinate", after)
				}
				if failure != "off" && !a.lid.markerExists() {
					t.Fatal("unconfirmed recovery lost its marker")
				}
			})
		}
	}
}
