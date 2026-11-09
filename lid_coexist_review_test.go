package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ConteMan/cubbix/internal/lid"
)

func TestLidTakeoverMarkerFailureEndsOnlyLid(t *testing.T) {
	app, sleep, _, service, _ := lidTestApp(t)
	sleep.value = 1
	session := app.selectedSession()
	session.Lid = true
	if err := app.startAwake(session); err != nil {
		t.Fatal(err)
	}
	child := app.awake.child
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	app.lid.marker = filepath.Join(blocked, "marker")
	sleep.value = 0
	app.pollLid()
	if sleep.value != 0 || app.lid.snapshot().state == lidOn || app.awake.snapshot().Session.Lid {
		t.Fatal("failed marker left an unmarked lid lease", app.lid.snapshot(), sleep.writes)
	}
	if !app.awake.snapshot().Active || app.awake.child != child || service.registers != 0 || app.lid.snapshot().lastCode != "marker-write-failed" {
		t.Fatal("marker failure replaced the ordinary session or lost its diagnosis", app.awake.snapshot(), app.awake.child == child, service.registers, app.lid.snapshot())
	}
}

func TestLidUnknownTakeoverDoesNotRearmForeignWatch(t *testing.T) {
	app, sleep, _, _, connection := lidTestApp(t)
	sleep.value = 1
	session := app.selectedSession()
	session.Lid = true
	if err := app.startAwake(session); err != nil {
		t.Fatal(err)
	}
	app.lid.apply(lid.Response{OK: true, Helper: lid.Version, State: "on-ours", Code: "pmset-failed"})
	if app.lid.snapshot().watching || app.lid.lastWatching {
		t.Fatal("owned takeover was still treated as foreign")
	}
	connection.gone = true
	app.pollLid()
	if app.lid.snapshot().rearmPending || app.awake.snapshot().Session.Lid {
		t.Fatal("unconfirmed owned lease was scheduled for rearm")
	}
}

func TestLidRearmStopsWithSession(t *testing.T) {
	app, sleep, _, service, connection := lidTestApp(t)
	sleep.value = 1
	session := app.selectedSession()
	session.Lid = true
	if err := app.startAwake(session); err != nil {
		t.Fatal(err)
	}
	connection.gone = true
	app.pollLid()
	if !app.lid.snapshot().rearmPending {
		t.Fatal("missing pending rearm")
	}
	if err := app.stopAwake(); err != nil {
		t.Fatal(err)
	}
	app.finishLidPoll()
	if app.lid.snapshot().rearmPending || app.lid.lastWatching || service.registers != 0 || len(sleep.writes) != 0 {
		t.Fatal("ended session retained a foreign rearm", app.lid.snapshot(), sleep.writes)
	}
}
