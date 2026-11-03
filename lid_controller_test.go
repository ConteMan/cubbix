package main

import (
	"errors"
	"github.com/ConteMan/cubbix/internal/lid"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type lidTestSleep struct {
	value    int
	failZero bool
	writes   []int
}

func (s *lidTestSleep) Read() (int, error) { return s.value, nil }
func (s *lidTestSleep) Set(v int) error {
	s.writes = append(s.writes, v)
	if v == 0 && s.failZero {
		return errors.New("failed")
	}
	s.value = v
	return nil
}

type lidTestPower struct {
	kind    string
	percent int
	err     error
}

func (p *lidTestPower) Source() (string, int, error) { return p.kind, p.percent, p.err }

type lidTestService struct {
	onRegister                    func()
	status                        lidServiceStatus
	registers, unregisters, reads int
	registerError                 error
	after                         lidServiceStatus
}

func (s *lidTestService) Status() lidServiceStatus { s.reads++; return s.status }
func (s *lidTestService) Register() error {
	s.registers++
	if s.onRegister != nil {
		s.onRegister()
	}
	s.status = s.after
	return s.registerError
}
func (s *lidTestService) Unregister() error          { s.unregisters++; return nil }
func (s *lidTestService) OpenSettings()              {}
func (s *lidTestService) HelperPath() (string, bool) { return "helper", s.status == svcEnabled }

type lidTestConnection struct {
	engine       *lid.Engine
	gone         bool
	helloVersion string
}

func (c *lidTestConnection) Call(r lid.Request) (lid.Response, error) {
	if c.gone {
		return lid.Response{}, errors.New("gone")
	}
	r.V = lid.Protocol
	r.ID = "test-client"
	out := c.engine.Handle(1, r)
	if c.helloVersion != "" {
		out.Helper = c.helloVersion
	}
	return out, nil
}
func (c *lidTestConnection) Close() error { return nil }

type lidTestForcer struct {
	sleep *lidTestSleep
	calls int
	fail  bool
}

func (f *lidTestForcer) Force() error {
	f.calls++
	if f.fail {
		return errors.New("cancelled")
	}
	return f.sleep.Set(0)
}
func lidTestApp(t *testing.T) (*application, *lidTestSleep, *lidTestPower, *lidTestService, *lidTestConnection) {
	t.Helper()
	a := panelTestApp(t)
	a.lidAuthorize = true
	a.notifyBattery = func(string, string) {}
	sleep := &lidTestSleep{}
	power := &lidTestPower{kind: "ac", percent: 80}
	svc := &lidTestService{status: svcEnabled, after: svcEnabled}
	engine := lid.NewEngine(sleep, power, lid.FileJournal{Path: filepath.Join(t.TempDir(), "root", "session.json")})
	engine.Start()
	conn := &lidTestConnection{engine: engine}
	a.lid = &lidController{service: svc, dial: func() (lidConnection, error) {
		if conn.gone {
			return nil, errors.New("gone")
		}
		return conn, nil
	}, read: sleep.Read, power: power, capable: true, supported: true, now: time.Now, marker: filepath.Join(filepath.Dir(a.path), "lid-restore-required"), snapshotValue: lidSnapshot{value: -1}}
	a.lid.forcer = &lidTestForcer{sleep: sleep}
	a.awake.beforeStop = func() { _ = a.lid.stop() }
	t.Cleanup(func() { a.awake.beforeStop = nil; a.lid.close() })
	return a, sleep, power, svc, conn
}
func TestLidEnableMemoryAndMarker(t *testing.T) {
	a, s, _, _, _ := lidTestApp(t)
	session := a.selectedSession()
	session.Lid = true
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	if s.value != 1 || a.lid.snapshot().state != lidOn || !a.config.AwakeLid || !a.lid.markerExists() || !a.config.AwakeSession.Lid {
		t.Fatal("enable was not confirmed")
	}
	if err := a.stopAwake(); err != nil {
		t.Fatal(err)
	}
	if s.value != 0 || a.lid.markerExists() || a.awake.snapshot().Active || !a.config.AwakeLid {
		t.Fatal("stop lifecycle")
	}
}
func TestLidEnableFailuresNeverRemember(t *testing.T) {
	for _, kind := range []string{"foreign", "battery", "approval", "denied", "gone"} {
		t.Run(kind, func(t *testing.T) {
			a, s, p, svc, conn := lidTestApp(t)
			switch kind {
			case "foreign":
				s.value = 1
			case "battery":
				p.kind = "battery"
				p.percent = 20
			case "approval":
				svc.status = svcRequiresApproval
			case "denied":
				svc.status = svcNotRegistered
				svc.after = svcNotRegistered
				svc.registerError = errLidDenied
			case "gone":
				conn.gone = true
			}
			session := a.selectedSession()
			session.Lid = true
			if err := a.startAwake(session); err == nil {
				t.Fatal("accepted")
			}
			if a.config.AwakeLid || a.lid.markerExists() || a.config.AwakeSession != nil && a.config.AwakeSession.Lid {
				t.Fatal("failed lease remembered")
			}
			if kind == "denied" && (svc.reads < 2 || a.lid.snapshot().service != svcDenied) {
				t.Fatal("status not reread", svc.reads)
			}
		})
	}
}
func TestLidRestoreFailureBlocksStopDisableAndUpdate(t *testing.T) {
	a, s, _, svc, _ := lidTestApp(t)
	session := a.selectedSession()
	session.Lid = true
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	s.failZero = true
	if err := a.stopAwake(); err == nil {
		t.Fatal("restore failure ignored")
	}
	if !a.lid.markerExists() || a.lid.snapshot().state != lidRestoreFailed || !a.lid.canForce() {
		t.Fatal("liability lost")
	}
	a.config.AwakeEnabled = false
	a.changeAwakeEnabled()
	if !a.config.AwakeEnabled || svc.unregisters > 0 {
		t.Fatal("unsafe unregister")
	}
	a.updates.mu.Lock()
	a.updates.state.Phase = updateReady
	a.updates.mu.Unlock()
	a.relaunch = func() { t.Fatal("unsafe relaunch") }
	a.restartUpdate()
	s.failZero = false
	if err := a.stopAwake(); err != nil {
		t.Fatal(err)
	}
	if a.lid.markerExists() {
		t.Fatal("marker retained")
	}
}
func TestLidStaleSettingAuthority(t *testing.T) {
	for _, online := range []bool{false, true} {
		for _, marker := range []bool{false, true} {
			for _, value := range []int{0, 1} {
				a, s, _, _, conn := lidTestApp(t)
				s.value = value
				conn.gone = !online
				if marker {
					if err := a.lid.writeMarker("stale"); err != nil {
						t.Fatal(err)
					}
				}
				a.lid.Inspect()
				want := !online && marker && value == 1
				if a.lid.canForce() != want {
					t.Fatalf("online=%v marker=%v value=%d snapshot=%+v", online, marker, value, a.lid.snapshot())
				}
				if value == 0 || online && value == 1 {
					if a.lid.markerExists() {
						t.Fatal("stale marker retained")
					}
				}
				if len(s.writes) != 0 {
					t.Fatal("startup wrote setting")
				}
			}
		}
	}
}
func TestLidForceRestoreAndForeignWithdrawal(t *testing.T) {
	a, s, _, _, conn := lidTestApp(t)
	s.value = 1
	conn.gone = true
	a.lid.writeMarker("stale")
	a.lid.Inspect()
	if !a.lid.canForce() {
		t.Fatal("missing recovery")
	}
	conn.gone = false
	a.lid.Inspect()
	if a.lid.canForce() || a.lid.markerExists() {
		t.Fatal("marker overrode online helper")
	}
	conn.gone = true
	a.lid.writeMarker("stale")
	a.lid.Inspect()
	if err := a.lid.force(); err != nil {
		t.Fatal(err)
	}
	if s.value != 0 || a.lid.markerExists() {
		t.Fatal("force readback")
	}
}
func TestLidCapabilitiesNeverConnect(t *testing.T) {
	for _, which := range []string{"dev", "desktop", "missing", "os12"} {
		a, _, _, _, _ := lidTestApp(t)
		a.lid.dial = func() (lidConnection, error) { t.Fatal("unavailable IPC"); return nil, nil }
		if which == "os12" {
			a.lid.supported = false
		} else {
			a.lid.capable = false
		}
		a.lid.Inspect()
		if a.lid.Available() {
			t.Fatal("available")
		}
		if a.lidCapable() != (which == "os12") {
			t.Fatal("row visibility")
		}
	}
}
func TestLidURLLimitsAndAuthorization(t *testing.T) {
	for _, raw := range []string{"cubbix://awake?lid=1", "cubbix://awake?lid=1&minutes=0", "cubbix://awake?lid=1&minutes=481", "cubbix://awake?lid=x&minutes=120", "cubbix://awake/stop?lid=x", "cubbix://awake/toggle?lid=1", "cubbix://awake/toggle?lid=1&minutes=481"} {
		if _, ok := parsePaletteURL(raw, true, true, jsonFileLimit); ok {
			t.Fatal(raw)
		}
	}
	a, s, _, svc, _ := lidTestApp(t)
	req, ok := parsePaletteURL("cubbix://awake?minutes=120&lid=1", true, true, jsonFileLimit)
	if !ok {
		t.Fatal("valid URL")
	}
	svc.status = svcNotRegistered
	if err := a.runURL(req); err == nil {
		t.Fatal("unapproved URL")
	}
	if a.awake.snapshot().Active || svc.registers != 0 || s.value != 0 {
		t.Fatal("URL side effect")
	}
	svc.status = svcEnabled
	a.config.FeedbackMode = "off"
	notified := 0
	a.notifyBattery = func(string, string) { notified++ }
	if err := a.runURL(req); err != nil {
		t.Fatal(err)
	}
	if notified != 1 || !a.awake.snapshot().Session.Lid || svc.registers != 0 {
		t.Fatal("missing forced notification")
	}
}
func TestLidModifierGrammar(t *testing.T) {
	for _, raw := range []string{"2h 合盖", "合盖 2h", "合盖", "2h lid", "盖 2h", "2h LID1"} {
		arg := parseAwakeArg(raw, time.Now())
		if !arg.Lid || arg.Error != "" {
			t.Fatal(raw, arg)
		}
	}
	if arg := parseAwakeArg("2h 合盖盖", time.Now()); arg.Error != "DurationError" {
		t.Fatal(arg)
	}
}
func TestLidCombinedLifecycle(t *testing.T) {
	for _, end := range []string{"toggle", "duration", "mode", "deadline", "process", "max-duration", "battery-low", "battery-unknown", "disconnect"} {
		t.Run(end, func(t *testing.T) {
			a, s, power, _, conn := lidTestApp(t)
			session := a.selectedSession()
			session.Lid = true
			if err := a.startAwake(session); err != nil {
				t.Fatal(err)
			}
			switch end {
			case "toggle":
				a.toggleLid()
				if !a.awake.snapshot().Active || a.config.AwakeLid {
					t.Fatal("toggle stopped ordinary lease")
				}
			case "duration":
				if err := a.setDuration(120); err != nil {
					t.Fatal(err)
				}
				if !a.lid.deadline.After(session.End) {
					t.Fatal("deadline not renewed")
				}
				if len(s.writes) != 1 {
					t.Fatal("renewal rewrote setting")
				}
				return
			case "mode":
				if err := a.setMode("tasks"); err != nil {
					t.Fatal(err)
				}
				if s.value != 1 || len(s.writes) != 1 {
					t.Fatal("mode touched lease")
				}
				return
			case "deadline", "process":
				a.awake.mu.Lock()
				if end == "deadline" {
					a.awake.state.Session.End = time.Now().Add(-time.Second)
				} else {
					a.awake.state.Session.PID = 42
					a.awake.state.Session.Identity = "gone"
					a.awake.alive = func(int, string) bool { return false }
				}
				a.awake.mu.Unlock()
				a.awake.poll()
			case "disconnect":
				conn.gone = true
				a.pollLid()
				if a.awake.snapshot().Active || !a.lid.markerExists() || !a.lid.canForce() {
					t.Fatal("disconnect liability")
				}
				return
			default:
				now := time.Now()
				conn.engine.Now = func() time.Time { return now }
				if end == "max-duration" {
					now = now.Add(lid.HardLimit)
					conn.engine.Tick()
					a.pollLid()
				} else {
					if end == "battery-low" {
						power.kind = "battery"
						power.percent = 15
					} else {
						power.percent = -1
					}
					samples := 6
					if end == "battery-unknown" {
						samples = 18
					}
					for range samples {
						now = now.Add(5 * time.Second)
						a.pollLid()
					}
				}
				if a.awake.snapshot().Active || a.lid.snapshot().response.Code != end {
					t.Fatalf("helper termination not propagated: %+v", a.lid.snapshot())
				}

			}
			if s.value != 0 || a.lid.markerExists() {
				t.Fatal("end not recovered")
			}
		})
	}
}
func TestLidConfigNormalization(t *testing.T) {
	for _, n := range []int{-1, 0, 10, 20, 30, 50, 999} {
		a := awakeTestApp(t)
		a.config.AwakeLidBatteryMin = n
		if err := saveConfig(a.path, a.config); err != nil {
			t.Fatal(err)
		}
		got, err := loadConfig(a.path)
		if err != nil {
			t.Fatal(err)
		}
		want := n
		if n != 10 && n != 20 && n != 30 {
			want = 20
		}
		if got.AwakeLidBatteryMin != want {
			t.Fatal(n, got.AwakeLidBatteryMin)
		}
	}
}
func TestLidMarkerPermissions(t *testing.T) {
	a, _, _, _, _ := lidTestApp(t)
	a.lid.writeMarker("test")
	info, err := os.Stat(a.lid.marker)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
}

func TestLidFailedModeRebuildRestores(t *testing.T) {
	a, s, _, _, _ := lidTestApp(t)
	session := a.selectedSession()
	session.Lid = true
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	a.awake.launch = func([]string) (awakeChild, error) { return nil, errors.New("launch") }
	if err := a.setMode("tasks"); err == nil {
		t.Fatal("expected failure")
	}
	if s.value != 0 || a.lid.markerExists() || a.awake.snapshot().Active {
		t.Fatal("failed rebuild kept lid lease")
	}
}
func TestLidRenewalFailureRestoresExistingLease(t *testing.T) {
	a, s, p, _, _ := lidTestApp(t)
	session := a.selectedSession()
	session.Lid = true
	a.startAwake(session)
	p.kind = "battery"
	p.percent = 10
	if err := a.setDuration(120); err == nil {
		t.Fatal("low battery renewal accepted")
	}
	if s.value != 0 || a.lid.markerExists() || a.awake.snapshot().Session.Lid {
		t.Fatal("failed renewal kept lease")
	}
}
func TestLidCancelRereadsApproval(t *testing.T) {
	a, _, _, svc, _ := lidTestApp(t)
	svc.status = svcNotRegistered
	svc.after = svcRequiresApproval
	svc.registerError = errLidDenied
	session := a.selectedSession()
	session.Lid = true
	if err := a.startAwake(session); err == nil || err.Error() != "LidAuthNeeded" {
		t.Fatal(err)
	}
	if a.lid.snapshot().service != svcRequiresApproval || svc.reads < 2 {
		t.Fatal("approval confused with denial")
	}
}
func TestLidDesktopIgnoresRememberedPreference(t *testing.T) {
	a, _, _, _, _ := lidTestApp(t)
	a.lid.capable = false
	a.config.AwakeLid = true
	if err := a.startAwake(a.selectedSession()); err != nil {
		t.Fatal(err)
	}
	if a.awake.snapshot().Session.Lid {
		t.Fatal("desktop lid lease")
	}
}
func TestLidStopDoesNotOfferForeignRecovery(t *testing.T) {
	a, s, _, _, _ := lidTestApp(t)
	s.value = 1
	a.lid.Inspect()
	if err := a.lid.stop(); err == nil {
		t.Fatal("foreign reported restored")
	}
	if a.lid.canForce() || len(s.writes) != 0 {
		t.Fatal("foreign recovery offered")
	}
	if !strings.Contains(a.awakeStatus("en", time.Now()), a.text("en", "LidForeign")) {
		t.Fatal("tray tooltip hid the external setting")
	}
}

func TestLidShortcutNeverInstalls(t *testing.T) {
	a, _, _, svc, _ := lidTestApp(t)
	a.lidAuthorize = false
	a.config.AwakeLid = true
	svc.status = svcNotRegistered
	a.shortcutToggleAwake()
	if svc.registers != 0 || a.lid.markerExists() {
		t.Fatal("shortcut started authorization")
	}
}

func TestLidSignatureFailureIsBroken(t *testing.T) {
	a, _, _, svc, _ := lidTestApp(t)
	svc.status = svcNotRegistered
	svc.after = svcNotRegistered
	svc.registerError = errors.New("signature failed")
	session := a.selectedSession()
	session.Lid = true
	if err := a.startAwake(session); err == nil || err.Error() != "LidServiceBroken" {
		t.Fatal("signature failure confused with cancellation", err)
	}
}
func TestLidHelperVersionUpdateReplacesConnection(t *testing.T) {
	a, s, _, svc, conn := lidTestApp(t)
	conn.helloVersion = "old-helper"
	svc.onRegister = func() { conn.helloVersion = "" }
	session := a.selectedSession()
	session.Lid = true
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	if svc.registers != 1 || s.value != 1 || a.lid.snapshot().response.Helper != lid.Version {
		t.Fatal("updated helper did not reconnect")
	}
}

// A new request owns its Lid choice, including an explicit off selection.
func TestLidNewRequestWithoutLidRestoresBeforeRestart(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, s, _, _, _ := lidTestApp(t)
			session := a.selectedSession()
			session.Lid = true
			if err := a.startAwake(session); err != nil {
				t.Fatal(err)
			}
			launch := a.awake.launch
			a.awake.launch = func(args []string) (awakeChild, error) {
				if s.value != 0 || a.lid.markerExists() || a.lid.snapshot().state != lidOff {
					t.Fatal("new assertion launched before confirmed lid recovery")
				}
				return launch(args)
			}
			off := false
			a.lidPending = &off
			if err := a.startAwakeWith(awakeArg{Kind: awakeArgMinutes, Minutes: 120}, "tasks"); err != nil {
				t.Fatal(err)
			}
			state := a.awake.snapshot()
			if !state.Active || state.Session.Lid || state.Session.Mode != "tasks" || state.Session.Minutes != 120 || a.config.AwakeSession == nil || a.config.AwakeSession.Lid {
				t.Fatal("new ordinary session inherited the old lid lease", state)
			}
			if s.value != 0 || a.lid.snapshot().state != lidOff || a.lid.markerExists() {
				t.Fatal("lid lease survived explicit off")
			}
			if strings.Contains(a.menubarPanelStatusLine(locale, time.Now()), a.text(locale, "R05Lid")) {
				t.Fatal("card status still advertises lid mode")
			}
			tt := panelView(a, locale)
			if !tt.HasText(a.text(locale, "R05LidHint")) {
				t.Fatal("card did not return to the ordinary-session hint")
			}
		})
	}
}

type lidIgnoreRestore struct{ *lidTestSleep }

func (s lidIgnoreRestore) Set(value int) error {
	if value == 0 {
		s.writes = append(s.writes, value)
		return nil
	}
	return s.lidTestSleep.Set(value)
}
func TestLidNewOrdinaryRequestRequiresConfirmedRecovery(t *testing.T) {
	for _, failure := range []string{"write", "readback"} {
		t.Run(failure, func(t *testing.T) {
			a, s, _, _, conn := lidTestApp(t)
			session := a.selectedSession()
			session.Lid = true
			if err := a.startAwake(session); err != nil {
				t.Fatal(err)
			}
			if failure == "write" {
				s.failZero = true
			} else {
				conn.engine.Sleep = lidIgnoreRestore{s}
			}
			before := a.awake.snapshot()
			launches := 0
			a.awake.launch = func([]string) (awakeChild, error) { launches++; return nil, errors.New("must not launch") }
			off := false
			a.lidPending = &off
			if err := a.startAwakeWith(awakeArg{Kind: awakeArgMinutes, Minutes: 120}, "tasks"); err == nil {
				t.Fatal("unconfirmed recovery accepted")
			}
			after := a.awake.snapshot()
			if launches != 0 || !after.Active || after.Session != before.Session || a.config.AwakeMode != before.Session.Mode {
				t.Fatal("failed recovery replaced the existing session")
			}
			if s.value != 1 || !a.lid.markerExists() || a.lid.snapshot().state != lidRestoreFailed {
				t.Fatal("recovery liability was lost")
			}
		})
	}
}
func TestLidOrdinaryRequestLeavesForeignSettingAlone(t *testing.T) {
	a, s, _, _, _ := lidTestApp(t)
	s.value = 1
	a.lid.Inspect()
	if err := a.startAwake(awakeSession{Mode: "tasks", Minutes: 60, End: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if !a.awake.snapshot().Active || a.awake.snapshot().Session.Lid || len(s.writes) != 0 || s.value != 1 {
		t.Fatal("ordinary request changed a foreign setting")
	}
}
