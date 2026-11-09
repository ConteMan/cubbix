package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
	"github.com/ConteMan/cubbix/internal/lid"
)

// scriptedLid returns canned helper responses while recording every request.
// All other operations still go through the injected engine.
type scriptedLid struct {
	lidConnection
	byOp     map[string]lid.Response
	requests []lid.Request
}

func (c *scriptedLid) Call(r lid.Request) (lid.Response, error) {
	c.requests = append(c.requests, r)
	if out, ok := c.byOp[r.Op]; ok {
		return out, nil
	}
	return c.lidConnection.Call(r)
}

func (c *scriptedLid) lastOp(op string) (lid.Request, bool) {
	for i := len(c.requests) - 1; i >= 0; i-- {
		if c.requests[i].Op == op {
			return c.requests[i], true
		}
	}
	return lid.Request{}, false
}

func intPtr(v int) *int { return &v }

// The helper's state is the only source of the watching flag.
func TestLidApplyMapsCoexistStates(t *testing.T) {
	for _, tc := range []struct {
		name         string
		state, code  string
		detail       string
		value        int
		wantState    lidState
		wantWatching bool
		wantReason   string
	}{
		{"watching", "watching", "ok", "", 1, lidOn, true, ""},
		{"adopted", "on-ours", "ok", "adopted", 1, lidOn, false, ""},
		{"external-release", "on-ours", "ok", "external-release", 1, lidOn, false, ""},
		{"contested", "off", "contested", "", 0, lidOff, false, "LidContested"},
		{"external-release-failed", "off", "external-release-failed", "", 0, lidOff, false, "LidExternalReleaseFailed"},
		{"on-foreign", "on-foreign", "foreign-setting", "", 1, lidBlocked, false, "LidForeign"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _, _, _ := lidTestApp(t)
			if err := a.lid.writeMarker("stale"); err != nil {
				t.Fatal(err)
			}
			a.lid.apply(lid.Response{OK: true, Helper: lid.Version, State: tc.state, Code: tc.code, Detail: tc.detail, Value: intPtr(tc.value)})
			s := a.lid.snapshot()
			if s.state != tc.wantState || s.watching != tc.wantWatching || s.reason != tc.wantReason {
				t.Fatalf("snapshot = %+v", s)
			}
			if tc.wantWatching && a.lid.markerExists() {
				t.Fatal("watching kept a recovery marker")
			}
			if (tc.code == "contested" || tc.code == "external-release-failed") && (s.lastCode != tc.code || a.lid.markerExists()) {
				t.Fatal("termination code or marker was lost", s.lastCode)
			}
		})
	}
}

// A successful watch is a success without a marker; a confirmed 1 is a
// success with one. An unmodified foreign setting is still a failure.
func TestLidEnableWatchingSuccessConditions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		reply        lid.Response
		wantErr      string
		wantWatching bool
		wantMarker   bool
	}{
		{"watching", lid.Response{OK: true, State: "watching", Code: "ok", Value: intPtr(1)}, "", true, false},
		{"ours", lid.Response{OK: true, State: "on-ours", Code: "ok", Value: intPtr(1)}, "", false, true},
		{"foreign", lid.Response{OK: false, State: "on-foreign", Code: "foreign-setting", Value: intPtr(1)}, "LidForeign", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _, svc, conn := lidTestApp(t)
			reply := tc.reply
			reply.Helper = lid.Version
			a.lid.dial = func() (lidConnection, error) {
				return &scriptedLid{lidConnection: conn, byOp: map[string]lid.Response{"enable": reply}}, nil
			}
			if err := a.lid.writeMarker("stale"); err != nil {
				t.Fatal(err)
			}
			err := a.lid.enable(time.Time{}, 20, false)
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr) {
				t.Fatal("unexpected enable result", err)
			}
			s := a.lid.snapshot()
			if tc.wantWatching != s.watching || a.lid.markerExists() != tc.wantMarker || svc.registers != 0 {
				t.Fatalf("marker/authorization: %+v registers=%d marker=%v", s, svc.registers, a.lid.markerExists())
			}
		})
	}
}

// Stopping a watching lease does not require a readback of 0: the 1 belongs
// to somebody else and must not be cleared.
func TestLidStopWatchingAcceptsForeignValue(t *testing.T) {
	a, s, _, svc, conn := lidTestApp(t)
	s.value = 1
	scripted := &scriptedLid{lidConnection: conn, byOp: map[string]lid.Response{
		"enable":  {OK: true, Helper: lid.Version, State: "watching", Code: "ok", Value: intPtr(1)},
		"disable": {OK: true, Helper: lid.Version, State: "off", Code: "ok", Value: intPtr(1)},
	}}
	a.lid.dial = func() (lidConnection, error) { return scripted, nil }
	if err := a.lid.enable(time.Time{}, 20, false); err != nil {
		t.Fatal(err)
	}
	if err := a.lid.stop(); err != nil {
		t.Fatal("watching stop was rejected", err)
	}
	snap := a.lid.snapshot()
	if snap.state != lidOff || snap.watching || a.lid.markerExists() || snap.value != 1 || len(s.writes) != 0 || svc.registers != 0 {
		t.Fatal("watching stop touched the foreign setting", snap, s.writes)
	}
}

// An authorized helper update must not be stopped by another tool's 1.
func TestLidHelperUpdateIsNotBlockedByForeignSetting(t *testing.T) {
	for _, online := range []bool{false, true} {
		t.Run(map[bool]string{true: "helper-online", false: "helper-offline"}[online], func(t *testing.T) {
			a, s, _, svc, conn := lidTestApp(t)
			s.value = 1
			conn.helloVersion = "old-helper"
			svc.onRegister = func() { conn.helloVersion = "" }
			if !online {
				conn.gone = true
			} else {
				a.lid.Inspect()
			}
			err := a.lid.enable(time.Time{}, 20, true)
			if online {
				if err != nil || svc.registers != 1 || !a.lid.snapshot().watching || len(s.writes) != 0 {
					t.Fatal("update did not reach Register", err, svc.registers, a.lid.snapshot(), s.writes)
				}
			} else if err == nil || err.Error() != "LidForeign" || svc.registers != 0 {
				t.Fatal("offline helper offered authorization", err, svc.registers)
			}
		})
	}
}

// Regression for the two-round reconnect: the disconnect round must not clear
// Session.Lid, and the online round must reattach with adoptForeign and no
// authorization. The helper restart drops the foreign journal first.
func TestLidRearmSurvivesDisconnectRound(t *testing.T) {
	a, s, p, svc, conn := lidTestApp(t)
	session := a.selectedSession()
	session.Lid = true
	s.value = 1
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	if !a.lid.snapshot().watching || !a.awake.snapshot().Session.Lid {
		t.Fatal("watching session was not established")
	}
	// Round one: the helper disappears. finishLidPoll must not clear the flag.
	conn.gone = true
	conn.requests = nil
	a.pollLid()
	if !a.lid.snapshot().rearmPending || !a.awake.snapshot().Session.Lid {
		t.Fatal("disconnect round cleared the lid intent", a.lid.snapshot(), a.awake.snapshot())
	}
	// The helper restarts: Engine.Start drops the foreign journal and reports
	// on-foreign. The old sleep and the restarted one agree on the value 1.
	restartedSleep := &lidTestSleep{value: 1}
	restarted := lid.NewEngine(restartedSleep, p, conn.engine.Store)
	if err := restarted.Start(); err != nil {
		t.Fatal(err)
	}
	conn.engine, conn.gone, conn.requests = restarted, false, nil
	// Round two: the online app reattaches exactly once.
	a.pollLid()
	enableReq, found := (func() (lid.Request, bool) {
		for _, r := range conn.requests {
			if r.Op == "enable" {
				return r, true
			}
		}
		return lid.Request{}, false
	})()
	if !found || !enableReq.AdoptForeign {
		t.Fatal("rearm did not send enable{adoptForeign}", conn.requests)
	}
	if !a.awake.snapshot().Session.Lid || !a.lid.snapshot().watching || a.lid.snapshot().rearmPending {
		t.Fatal("rearm did not restore the watching lease", a.lid.snapshot(), a.awake.snapshot())
	}
	if svc.registers != 0 || len(s.writes) != 0 || a.lid.markerExists() {
		t.Fatal("rearm authorized or wrote a value", svc.registers, s.writes)
	}
}

// Giving up after four leases: an unattended rearm must not hang forever.
func TestLidRearmGivesUpAfterBound(t *testing.T) {
	a, _, _, _, _ := lidTestApp(t)
	session := a.selectedSession()
	session.Lid = false
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	a.setSessionLid(true)
	now := time.Now()
	a.lid.now = func() time.Time { return now }
	a.lid.snapshotValue.rearmPending = true
	a.lid.rearmSince = now
	a.lid.snapshotValue.online = false
	a.lid.snapshotValue.state = lidHelperGone
	now = now.Add(lidRearmLimit + time.Second)
	a.finishLidPoll()
	if a.lid.snapshot().rearmPending || a.awake.snapshot().Session.Lid {
		t.Fatal("expired rearm kept the lid intent")
	}
}

func TestLidMarkerFollowsOwnership(t *testing.T) {
	a, s, _, svc, conn := lidTestApp(t)
	session := a.selectedSession()
	session.Lid = true
	s.value = 1
	if err := a.lid.writeMarker("stale"); err != nil {
		t.Fatal(err)
	}
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	if !a.lid.snapshot().watching || a.lid.markerExists() {
		t.Fatal("entering watching kept a stale marker")
	}
	// The external tool releases the 1: the helper takes over and the next
	// poll marks the now-owned 1 as our recovery liability.
	s.value = 0
	a.pollLid()
	if !a.lid.markerExists() || a.lid.snapshot().watching {
		t.Fatal("takeover did not restore the marker")
	}
	// Another release is observed while the write-back gap is open. A
	// confirmed 0 has no recovery liability, so the marker is cleared; the
	// next confirmed 1 (our own write) puts it back without authorizing.
	s.value = 0
	a.pollLid()
	if svc.registers != 0 || a.lid.snapshot().state != lidOn || a.lid.markerExists() {
		t.Fatal("gap-limited observation authorized or changed ownership")
	}
	s.value = 1
	a.pollLid()
	if !a.lid.markerExists() || svc.registers != 0 {
		t.Fatal("confirmed 1 did not restore the marker")
	}
	a.lid.apply(lid.Response{OK: true, Helper: lid.Version, State: "off", Code: "contested", Value: intPtr(0)})
	if a.lid.markerExists() {
		t.Fatal("contested kept a recovery marker")
	}
	_ = conn
}

func TestLidWatchingStopLeavesNoMarker(t *testing.T) {
	a, s, _, svc, conn := lidTestApp(t)
	s.value = 1
	session := a.selectedSession()
	session.Lid = true
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	if err := a.lid.stop(); err != nil {
		t.Fatal(err)
	}
	if a.lid.markerExists() || s.value != 1 || len(s.writes) != 0 || svc.registers != 0 {
		t.Fatal("watching end touched the foreign 1", s.writes)
	}
	_ = conn
}

func TestCLIWatchingAndDiagnostics(t *testing.T) {
	a, s, _, svc, conn := lidTestApp(t)
	a.config.AwakeLidBatteryMin = 5
	s.value = 1
	yes := true
	req := cli.Request{V: cli.Protocol, ID: "cli-1", Op: cli.OpRun, Action: cli.ActionAwake, Arg: "2h", Lid: &yes}
	result := a.applyCLI(req, nil)
	if !result.OK || result.State.Awake.Lid.State != "watching" || result.Message != a.text(uiLocale(), "LidWatching") {
		t.Fatalf("CLI did not report watching: %+v", result)
	}
	if svc.registers != 0 || a.lid.markerExists() || len(s.writes) != 0 {
		t.Fatal("CLI authorized or wrote a value")
	}
	state := a.cliState()
	if state.Awake.Lid.BatteryMin != 5 || state.Awake.Lid.State != "watching" {
		t.Fatal("batteryMin or state missing", state.Awake.Lid)
	}
	// cliState is a pure snapshot: it never probes the helper.
	a.lid.snapshotValue.adapter = "yes"
	a.lid.snapshotValue.rewrites = 2
	before := len(conn.requests)
	state = a.cliState()
	if state.Awake.Lid.Adapter != "yes" || state.Awake.Lid.Rewrites != 2 || len(conn.requests) != before {
		t.Fatal("status diagnostics probed the helper", state.Awake.Lid)
	}
	if !strings.Contains(strings.TrimSpace(a.cliStatusLines("en", state)[0]), strings.TrimSpace(a.text("en", "R05LidHeld"))) {
		t.Fatal("human status line omitted the watching suffix")
	}
	// Without an installed helper the request is refused, never installed.
	b, _, _, bSvc, _ := lidTestApp(t)
	bSvc.status, bSvc.after = svcNotRegistered, svcNotRegistered
	b.lid.snapshotValue.service = svcNotRegistered
	denied := b.applyCLI(req, nil)
	if denied.OK || denied.Code != "LidAuthNeeded" || bSvc.registers != 0 {
		t.Fatal("uninstalled CLI offered authorization", denied)
	}
}

func TestFivePercentThreshold(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{5, 5}, {10, 10}, {20, 20}, {30, 30}, {0, 20}, {15, 20}, {40, 20}, {999, 20}} {
		a := awakeTestApp(t)
		a.config.AwakeLidBatteryMin = tc.in
		if err := saveConfig(a.path, a.config); err != nil {
			t.Fatal(err)
		}
		got, err := loadConfig(a.path)
		if err != nil || got.AwakeLidBatteryMin != tc.want {
			t.Fatal(tc.in, got.AwakeLidBatteryMin, err)
		}
	}
	valid := lid.Request{V: lid.Protocol, ID: "x", Op: "enable", MinPercent: 5}
	if valid.Validate(time.Now()) != "" {
		t.Fatal("5% rejected by the helper protocol")
	}
	valid.MinPercent = 4
	if valid.Validate(time.Now()) == "" {
		t.Fatal("4% accepted by the helper protocol")
	}
	for i, want := range lidBatteryLevels {
		if got := lidBatteryIndex(want); got != i {
			t.Fatal("segment index", want, got)
		}
	}
	if lidBatteryIndex(15) != 2 {
		t.Fatal("unknown value did not fall back to 20%")
	}
	tk := tokens(false, "en")
	segmentWidth := (lidBatteryTrackWidth(tk) - 2*tk.Space2) / float32(len(lidBatteryLevels))
	if segmentWidth < 3*tk.FontSizeBody {
		t.Fatal("four segments are too narrow", segmentWidth)
	}
}

func TestLidCoexistLocaleKeys(t *testing.T) {
	a := awakeTestApp(t)
	for _, locale := range []string{"zh-CN", "en"} {
		for _, key := range []string{"LidWatching", "R05LidHeld", "LidContested", "LidExternalReleaseFailed", "LidBatteryHint", "LidBatteryFiveHint"} {
			if value := a.text(locale, key); value == "" || strings.Contains(value, "%!") {
				t.Fatalf("%s: missing %s", locale, key)
			}
		}
	}
	if _, err := os.Stat("design/locales/zh-CN.json"); err != nil {
		t.Fatal(err)
	}
}
