package lid

import (
	"errors"
	"testing"
	"time"
)

func TestReleasedWatchCannotTakeOverOnTermination(t *testing.T) {
	for _, trigger := range []string{"disable", "disconnect", "shutdown", "lease", "deadline", "hard-limit", "battery-low", "battery-unknown"} {
		t.Run(trigger, func(t *testing.T) {
			engine, sleep, power, now := fixture(t)
			end := now.Add(time.Minute)
			enterWatch(t, engine, sleep, &end)
			sleep.v = 0
			switch trigger {
			case "disable":
				engine.Handle(1, request("disable"))
			case "disconnect":
				engine.Disconnect(1)
			case "shutdown":
				if err := engine.Shutdown(); err != nil {
					t.Fatal(err)
				}
			case "lease":
				advance(now, Lease)
				engine.Tick()
			case "deadline", "hard-limit":
				if trigger == "deadline" {
					*now = end
				} else {
					advance(now, HardLimit)
					engine.journal.Deadline = nil
				}
				heartbeat(engine, now)
				engine.Tick()
			case "battery-low", "battery-unknown":
				power.kind, power.p = "battery", 10
				if trigger == "battery-unknown" {
					power.p = -1
					engine.unknown = batteryUnknownLimit - 1
				}
				advance(now, BatteryInterval)
				heartbeat(engine, now)
				engine.Tick()
			}
			if len(sleep.writes) != 0 || sleep.v != 0 {
				t.Fatal("terminated watch acquired ownership", sleep.writes, sleep.v)
			}
			noJournal(t, engine)
			if trigger == "hard-limit" && engine.code != "max-duration" {
				t.Fatal("takeover renewed the expired hard limit", engine.code)
			}
		})
	}
}

func TestOwnedTerminationCannotRewriteFirst(t *testing.T) {
	for _, trigger := range []string{"disable", "lease", "hard-limit", "battery-low"} {
		t.Run(trigger, func(t *testing.T) {
			engine, sleep, power, now := fixture(t)
			enable(t, engine)
			sleep.v = 0
			switch trigger {
			case "disable":
				engine.Handle(1, request("disable"))
			case "lease":
				advance(now, Lease)
				engine.Tick()
			case "hard-limit":
				advance(now, HardLimit)
				heartbeat(engine, now)
				engine.Tick()
			case "battery-low":
				power.kind, power.p = "battery", 10
				advance(now, BatteryInterval)
				heartbeat(engine, now)
				engine.Tick()
			}
			if countWrites(sleep.writes, 1) != 1 || sleep.v != 0 {
				t.Fatal("termination wrote another 1", sleep.writes)
			}
			noJournal(t, engine)
		})
	}
}

func TestBudgetConfirmationFailuresAreBounded(t *testing.T) {
	engine, sleep, _, now := fixture(t)
	enable(t, engine)
	engine.journal.Rewrites = RewriteSessionLimit
	engine.Sleep = &readSequence{fakeSleep: sleep, values: []int{0, -1, 0, -1, 0, -1, 0}}
	var response Response
	for range readFailureLimit {
		advance(now, time.Second)
		response = engine.Handle(1, request("ping"))
	}
	if engine.journal != nil || countWrites(sleep.writes, 0) != 1 || engine.code != "pmset-failed" || response.Code != "pmset-failed" || response.Value == nil || *response.Value != 0 {
		t.Fatal("confirmation failures were reset by the initial 0", engine.readFailures, sleep.writes, engine.code)
	}
}

func TestObservationThrottleIncludesRequests(t *testing.T) {
	engine, sleep, _, now := fixture(t)
	advance(now, ObserveInterval)
	engine.Tick()
	before := sleep.reads
	for range 20 {
		engine.Handle(1, request("ping"))
	}
	if sleep.reads != before {
		t.Fatal("requests bypassed the shared 1s throttle", sleep.reads-before)
	}
}

type removalFailureStore struct {
	JournalStore
	fail      bool
	failSave  bool
	saveCalls int
}

func (store *removalFailureStore) Save(journal Journal) error {
	store.saveCalls++
	if store.failSave {
		return errors.New("save")
	}
	return store.JournalStore.Save(journal)
}

func (store *removalFailureStore) Remove() error {
	if store.fail {
		return errors.New("remove")
	}
	return store.JournalStore.Remove()
}

func TestReleasedOwnershipSurvivesJournalRemovalFailure(t *testing.T) {
	engine, sleep, power, now := fixture(t)
	enable(t, engine)
	store := &removalFailureStore{JournalStore: engine.Store, fail: true}
	engine.Store = store
	engine.journal.Rewrites = RewriteSessionLimit
	sleep.v = 0
	advance(now, ObserveInterval)
	engine.Handle(1, request("ping"))
	sleep.v = 1
	store.fail = false
	restarted := NewEngine(sleep, power, store)
	if err := restarted.Start(); err != nil {
		t.Fatal(err)
	}
	if sleep.v != 1 || countWrites(sleep.writes, 0) != 0 {
		t.Fatal("stale owned journal cleared a subsequent foreign 1", sleep.writes)
	}
	noJournal(t, restarted)
}

func TestCleanupRetriesCannotClearForeignValue(t *testing.T) {
	for _, mode := range []string{"watching", "owned", "contested"} {
		t.Run(mode, func(t *testing.T) {
			engine, sleep, _, now := fixture(t)
			if mode == "watching" {
				enterWatch(t, engine, sleep, nil)
			} else {
				enable(t, engine)
			}
			store := &removalFailureStore{JournalStore: engine.Store, fail: true}
			engine.Store = store
			if mode == "contested" {
				engine.journal.Rewrites = RewriteSessionLimit
				sleep.v = 0
				advance(now, ObserveInterval)
				engine.Handle(1, request("ping"))
			} else {
				if response := engine.Handle(1, request("disable")); response.OK {
					t.Fatal("failed cleanup reported success")
				}
			}
			if engine.journal == nil || !engine.cleanupPending || storedJournal(t, engine).Writer != "foreign" {
				t.Fatal("cleanup lost its durable ownership release")
			}
			before := len(sleep.writes)
			sleep.v = 1
			advance(now, ObserveInterval)
			engine.Tick()
			if len(sleep.writes) != before || sleep.v != 1 {
				t.Fatal("cleanup retried a system write", sleep.writes)
			}
			store.fail = false
			advance(now, ObserveInterval)
			engine.Tick()
			noJournal(t, engine)
			if len(sleep.writes) != before || engine.cleanupPending {
				t.Fatal("cleanup did not converge")
			}
		})
	}
}

func TestRenewalCannotRestartAnExpiredLease(t *testing.T) {
	engine, sleep, _, now := fixture(t)
	enterWatch(t, engine, sleep, nil)
	advance(now, HardLimit)
	heartbeat(engine, now)
	sleep.v = 0
	response := engine.Handle(1, watchRequest(nil, 5))
	if response.OK || response.Code != "max-duration" || len(sleep.writes) != 0 {
		t.Fatal("renewal restarted an expired watch", response, sleep.writes)
	}
	noJournal(t, engine)
}

func TestRewriteWriteErrorStillAcceptsConfirmedOne(t *testing.T) {
	engine, sleep, _, now := fixture(t)
	enable(t, engine)
	sleep.v = 0
	sleep.setErr = true
	sleep.before = func(value int) { sleep.v = value }
	advance(now, RewriteGap)
	response := engine.Handle(1, request("ping"))
	if !response.OK || response.State != "on-ours" || response.Code != "ok" || response.Value == nil || *response.Value != 1 || engine.rewriteFails != 0 || storedJournal(t, engine).Writer != "ours" {
		t.Fatal("successful readback was discarded after a write error", response, engine.rewriteFails)
	}
}

func TestBudgetSaveFailureStillRespectsRewriteGap(t *testing.T) {
	engine, sleep, _, now := fixture(t)
	enable(t, engine)
	store := &removalFailureStore{JournalStore: engine.Store, failSave: true}
	engine.Store = store
	sleep.v = 0
	advance(now, RewriteGap)
	engine.Handle(1, request("ping"))
	for range 9 {
		advance(now, ObserveInterval)
		engine.Handle(1, request("ping"))
	}
	if store.saveCalls != 1 || len(sleep.writes) != 1 {
		t.Fatal("failed journal saves bypassed the rewrite gap", store.saveCalls, sleep.writes)
	}
	advance(now, ObserveInterval)
	engine.Handle(1, request("ping"))
	if store.saveCalls != 2 {
		t.Fatal("rewrite did not retry after its gap", store.saveCalls)
	}
}

func TestCleanupSaveFailureDoesNotRepeatSystemWrites(t *testing.T) {
	engine, sleep, _, now := fixture(t)
	enable(t, engine)
	store := &removalFailureStore{JournalStore: engine.Store, failSave: true}
	engine.Store = store
	if response := engine.Handle(1, request("disable")); response.OK {
		t.Fatal("failed responsibility release reported success")
	}
	if !engine.cleanupPending || storedJournal(t, engine).Writer != "ours" || sleep.v != 0 {
		t.Fatal("failed responsibility release lost its retry", engine.cleanupPending, sleep.v)
	}
	sleep.v = 1
	advance(now, ObserveInterval)
	engine.Tick()
	store.failSave = false
	advance(now, ObserveInterval)
	engine.Tick()
	if countWrites(sleep.writes, 0) != 1 || sleep.v != 1 || engine.cleanupPending {
		t.Fatal("responsibility release retried a system write", sleep.writes)
	}
	noJournal(t, engine)
}
