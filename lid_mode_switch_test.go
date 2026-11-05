package main

import (
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
	"github.com/ConteMan/cubbix/internal/lid"
)

type lidRecordingConnection struct {
	lidConnection
	ops []string
}

func (c *lidRecordingConnection) Call(r lid.Request) (lid.Response, error) {
	c.ops = append(c.ops, r.Op)
	return c.lidConnection.Call(r)
}

func TestLidIntentDuringAsyncStart(t *testing.T) {
	a, _, _, _, _ := lidTestApp(t)
	a.lidAsync = true
	callbacks := make(chan func(), 1)
	a.urlDispatch = func(run func()) { callbacks <- run }
	s := a.selectedSession()
	s.Lid = true
	err := a.startAwake(s)
	if err != nil {
		t.Fatal(err)
	}
	// Observe the same interval as the queued awakeChanged callback: the
	// ordinary assertion is running, but lid completion has not run yet.
	wanted := a.awake.snapshot().Session.Lid
	select {
	case run := <-callbacks:
		run()
	case <-time.After(time.Second):
		t.Fatal("missing lid completion")
	}
	if !wanted {
		t.Fatal("start erased the requested lid intent before completion")
	}
}

func TestLidReusingLeaseKeepsRequestedIntent(t *testing.T) {
	a, sleep, _, _, _ := lidTestApp(t)
	// The helper may already be on while the ordinary session's cached bit
	// is false. Reusing that lease must still publish the new selection.
	if err := a.awake.start(a.selectedSession()); err != nil {
		t.Fatal(err)
	}
	s := a.awake.snapshot().Session
	s.Lid = true
	if err := a.lid.enable(s.End, 20, false); err != nil {
		t.Fatal(err)
	}
	if err := a.startAwake(s); err != nil {
		t.Fatal(err)
	}
	if err := a.setMode("tasks"); err != nil {
		t.Fatal(err)
	}
	if !a.awake.snapshot().Session.Lid || sleep.value != 1 || len(sleep.writes) != 1 {
		t.Fatal("mode switch disabled the reused lease", a.awake.snapshot(), sleep.writes)
	}
}

func TestLidModeDurationAndTogglePreserveLease(t *testing.T) {
	for _, async := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync", true: "async"}[async], func(t *testing.T) {
			a, sleep, _, _, conn := lidTestApp(t)
			recording := &lidRecordingConnection{lidConnection: conn}
			a.lid.dial = func() (lidConnection, error) { return recording, nil }
			a.lidAsync = async
			callbacks := make(chan func(), 1)
			a.urlDispatch = func(run func()) { callbacks <- run }
			finish := func() {
				if a.lidBusy {
					select {
					case run := <-callbacks:
						run()
					case <-time.After(time.Second):
						t.Fatal("missing completion")
					}
				}
				state := a.awake.snapshot()
				if !state.Active || !state.Session.Lid || !a.config.AwakeLid || a.config.AwakeSession == nil || !a.config.AwakeSession.Lid || a.lid.snapshot().state != lidOn || sleep.value != 1 {
					t.Fatal("lost lid selection", state, a.lid.snapshot())
				}
			}
			a.toggleLid()
			intent := a.awake.snapshot().Session.Lid
			finish()
			if !intent {
				t.Fatal("lid switch erased intent before completion")
			}
			initialDeadline := a.lid.deadline
			recording.ops = nil
			for _, mode := range []int{1, 0, 1} {
				a.panelSetMode(mode)
				finish()
			}
			if len(recording.ops) != 0 || !a.lid.deadline.Equal(initialDeadline) {
				t.Fatal("mode switch touched helper lease", recording.ops)
			}
			// A shorter deadline leaves the helper alone; a longer one renews
			// once. Unlimited -> finite also needs no renewal.
			for _, minutes := range []int{30, 120, 0, 60} {
				recording.ops = nil
				a.panelSetDuration(minutes)
				finish()
				wantEnable := minutes == 120 || minutes == 0
				if wantEnable && (len(recording.ops) != 1 || recording.ops[0] != "enable") || !wantEnable && len(recording.ops) != 0 {
					t.Fatal("incorrect renewal", minutes, recording.ops)
				}
				if len(sleep.writes) != 1 {
					t.Fatal("duration rewrote sleep setting", sleep.writes)
				}
			}
			recording.ops = nil
			a.panelSetDuration(60)
			if len(recording.ops) != 0 {
				t.Fatal("active pill restarted lease")
			}
			a.toggleAwake()
			if a.awake.snapshot().Active || sleep.value != 0 || !a.config.AwakeLid {
				t.Fatal("stop lost remembered preference")
			}
			a.toggleAwake()
			finish()
			a.toggleLid()
			if a.awake.snapshot().Session.Lid || a.config.AwakeLid || sleep.value != 0 {
				t.Fatal("explicit switch off ignored")
			}
			a.panelSetMode(0)
			a.panelSetDuration(30)
			if a.awake.snapshot().Session.Lid || sleep.value != 0 {
				t.Fatal("ordinary selection reenabled lid")
			}
		})
	}
}

func TestLidExplicitCLIAndURLSelectionOverridesPreference(t *testing.T) {
	for _, source := range []string{"cli", "url"} {
		t.Run(source, func(t *testing.T) {
			a, sleep, _, _, _ := lidTestApp(t)
			a.toggleLid()
			no := false
			if source == "cli" {
				r := a.applyCLI(cli.Request{Op: cli.OpRun, Action: cli.ActionAwake, Arg: "30m", Lid: &no}, nil)
				if !r.OK {
					t.Fatal(r)
				}
			} else {
				req, ok := parsePaletteURL("cubbix://awake?minutes=30&lid=0", true, true, jsonFileLimit)
				if !ok {
					t.Fatal("valid URL rejected")
				}
				if err := a.runURL(req); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.setMode("tasks"); err != nil {
				t.Fatal(err)
			}
			if !a.awake.snapshot().Active || a.awake.snapshot().Session.Lid || sleep.value != 0 {
				t.Fatal("explicit off was not preserved", a.awake.snapshot())
			}
		})
	}
}
