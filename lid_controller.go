package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/ConteMan/cubbix/internal/lid"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

var errLidDenied = errors.New("LidDenied")
var errLidHelperUpdate = errors.New("LidHelperUpdate")

type lidServiceStatus int

const (
	svcUnavailable lidServiceStatus = iota
	svcNotRegistered
	svcEnabling
	svcRequiresApproval
	svcDenied
	svcBroken
	svcEnabled
	svcNotRunning
)

type lidService interface {
	Status() lidServiceStatus
	Register() error
	Unregister() error
	OpenSettings()
	HelperPath() (string, bool)
}
type lidConnection interface {
	Call(lid.Request) (lid.Response, error)
	Close() error
}
type sleepForcer interface{ Force() error }
type lidState int

const (
	lidOff lidState = iota
	lidEnabling
	lidOn
	lidStopping
	lidRestoreFailed
	lidHelperGone
	lidBlocked
	lidError
)

type lidSnapshot struct {
	state                          lidState
	reason                         string
	service                        lidServiceStatus
	online                         bool
	value                          int
	response                       lid.Response
	lastCode, hello, helperVersion string
	lastFailure                    *lid.Response
	install                        lidInstallDiagnostic
	stale, ours                    bool
}
type lidController struct {
	mu                 sync.Mutex
	published          atomic.Pointer[lidSnapshot]
	service            lidService
	dial               func() (lidConnection, error)
	read               func() (int, error)
	power              lid.PowerSource
	forcer             sleepForcer
	capable, supported bool
	unavailable        string
	marker             string
	connection         lidConnection
	snapshotValue      lidSnapshot
	deadline           time.Time
	now                func() time.Time
	wait               func(time.Duration)
}

func (c *lidController) snapshot() lidSnapshot {
	if c == nil {
		return lidSnapshot{state: lidOff, value: -1}
	}
	if c.mu.TryLock() {
		defer c.mu.Unlock()
		return c.snapshotValue
	}
	if s := c.published.Load(); s != nil {
		return *s
	}
	return lidSnapshot{state: lidEnabling, value: -1}
}
func (c *lidController) publish()           { copy := c.snapshotValue; c.published.Store(&copy) }
func (c *lidController) Available() bool    { return c != nil && c.capable && c.supported }
func (c *lidController) markerExists() bool { _, err := os.Stat(c.marker); return err == nil }
func (c *lidController) removeMarker() error {
	err := os.Remove(c.marker)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (c *lidController) writeMarker(id string) error {
	if err := os.MkdirAll(filepath.Dir(c.marker), 0700); err != nil {
		return err
	}
	data, _ := json.Marshal(struct {
		Started time.Time `json:"started"`
		ID      string    `json:"id"`
	}{c.now(), id})
	f, err := os.CreateTemp(filepath.Dir(c.marker), ".lid-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), c.marker)
}
func (c *lidController) connect() error {
	if c.connection != nil {
		if c.snapshotValue.response.Helper != lid.Version {
			return errLidHelperUpdate
		}
		return nil
	}
	conn, err := c.dial()
	if err != nil {
		c.snapshotValue.hello = "dial-failed"
		return err
	}
	r, err := conn.Call(lid.Request{Op: "hello"})
	if err != nil {
		c.snapshotValue.hello = "call-failed"
		conn.Close()
		return err
	}
	c.connection = conn
	c.snapshotValue.helperVersion = r.Helper
	c.snapshotValue.hello = "ok"
	c.apply(r)
	if r.Helper != lid.Version {
		c.snapshotValue.hello = "version-mismatch"
		c.snapshotValue.reason = "LidHelperUpdate"
		return errLidHelperUpdate
	}
	if !r.OK {
		c.snapshotValue.hello = "rejected"
		return errors.New("LidServiceBroken")
	}
	return nil
}
func (c *lidController) apply(r lid.Response) {
	s := &c.snapshotValue
	s.online = true
	s.response = r
	s.value = -1
	if r.Value != nil {
		s.value = *r.Value
	}
	// An unknown observation is not evidence that an owned lease ended.
	// Keep the last confirmed state and recovery marker until the helper
	// reports a terminal reason or a confirmed value/state transition.
	if s.state == lidOn && s.value != 0 && s.value != 1 && (r.Code == "ok" || r.Code == "pmset-failed") && (r.State == "on-ours" || r.State == "off") {
		return
	}
	if s.value == 0 {
		c.removeMarker()
		s.stale = false
		s.ours = false
	}
	switch r.State {
	case "on-foreign":
		c.removeMarker()
		s.state = lidBlocked
		s.reason = "LidForeign"
		s.ours = false
	case "restore-failed", "restoring":
		s.state = lidRestoreFailed
		s.reason = "LidRestoreFailed"
	case "on-ours":
		s.state = lidOn
		s.reason = ""
	case "off":
		s.state = lidOff
		s.reason = ""
	}
	switch r.Code {
	case "pmset-failed":
		if s.state != lidOn && s.state != lidRestoreFailed {
			s.reason = "LidEnableFailed"
		}
	case "battery-low":
		s.reason = "LidBatteryLow"
	case "battery-unknown":
		s.reason = "LidBatteryUnknown"
	case "max-duration":
		s.reason = "LidMaxDuration"
	}
}
func (c *lidController) lost() {
	if c.connection != nil {
		c.connection.Close()
		c.connection = nil
	}
	s := &c.snapshotValue
	s.online = false
	s.state = lidHelperGone
	s.reason = "LidHelperGone"
	c.readStale()
}
func (c *lidController) readStale() {
	value, err := c.read()
	s := &c.snapshotValue
	s.value = -1
	if err != nil {
		return
	}
	s.value = value
	if value == 0 {
		c.removeMarker()
		s.stale = false
		s.ours = false
		return
	}
	s.stale = value == 1
	s.ours = c.markerExists()
	if s.stale {
		if s.ours {
			s.reason = "LidStaleSetting"
		} else {
			s.reason = "LidForeign"
		}
	}
}

// Inspect is read-only, including startup and polling; it never installs.
func (c *lidController) Inspect() {
	if c == nil || !c.Available() {
		return
	}
	c.mu.Lock()
	defer func() { c.publish(); c.mu.Unlock() }()
	c.snapshotValue.service = c.service.Status()
	switch c.snapshotValue.service {
	case svcUnavailable:
		c.snapshotValue.state = lidBlocked
		c.snapshotValue.reason = "LidUnsupported"
	case svcRequiresApproval:
		c.snapshotValue.state = lidBlocked
		c.snapshotValue.reason = "LidAuthNeeded"
	case svcBroken:
		c.snapshotValue.state = lidBlocked
		c.snapshotValue.reason = "LidServiceBroken"
	case svcNotRunning:
		c.snapshotValue.state = lidBlocked
		c.snapshotValue.reason = "LidHelperNotRunning"
	}
	c.publish()
	if err := c.connect(); err != nil {
		c.readStale()
		c.snapshotValue.online = false
		if errors.Is(err, errLidHelperUpdate) {
			c.snapshotValue.state = lidBlocked
			c.snapshotValue.reason = "LidHelperUpdate"
		}
		if c.snapshotValue.reason == "" && c.snapshotValue.service != svcNotRegistered {
			c.snapshotValue.state = lidBlocked
			c.snapshotValue.reason = "LidHelperNotRunning"
		}
		return
	}
	r, err := c.connection.Call(lid.Request{Op: "ping"})
	if err != nil {
		c.lost()
		return
	}
	c.apply(r)
}
func (c *lidController) enable(end time.Time, min int, authorize bool) error {
	if !c.Available() {
		return errors.New("LidUnsupported")
	}
	c.mu.Lock()
	defer func() { c.publish(); c.mu.Unlock() }()
	s := &c.snapshotValue
	wasOn := s.state == lidOn
	s.state = lidEnabling
	c.publish()
	s.reason = ""
	s.service = c.service.Status()
	fail := func(key, code string) error {
		s.lastCode = code
		r := s.response
		s.lastFailure = &r
		if wasOn && s.state != lidRestoreFailed {
			if err := c.stopLocked(); err != nil {
				return err
			}
		}
		if s.state != lidRestoreFailed {
			s.state = lidBlocked
		}
		s.reason = key
		return errors.New(key)
	}
	if !c.supported {
		key := c.unavailable
		if key == "" {
			key = "LidUnsupported"
		}
		return fail(key, key)
	}
	if c.power != nil {
		kind, p, err := c.power.Source()
		if err != nil || p < 0 || p > 100 || kind == "battery" && p <= min {
			return fail("LidBatteryTooLow", "LidBatteryTooLow")
		}
	}
	// Refuse foreign values before presenting any authorization.
	if value, err := c.read(); err != nil {
		return fail("LidEnableFailed", "pmset-read-failed")
	} else if value == 1 && s.state != lidOn && c.connection == nil {
		c.readStale()
		if !s.ours {
			return fail("LidForeign", "LidForeign")
		}
	}
	if s.service == svcRequiresApproval {
		return fail("LidAuthNeeded", "LidAuthNeeded")
	}
	needsInstall := s.service == svcNotRegistered || s.service == svcBroken || s.service == svcNotRunning
	if s.service == svcUnavailable {
		return fail("LidUnsupported", "LidUnsupported")
	}
	if s.service == svcEnabled {
		if err := c.connect(); err != nil {
			needsInstall = true
		}
	}
	if needsInstall {
		if !authorize {
			if s.hello == "version-mismatch" {
				return fail("LidHelperUpdate", "helper-version-mismatch")
			}
			return fail("LidAuthNeeded", "LidAuthNeeded")
		}
		if c.connection != nil || c.markerExists() {
			if err := c.stopLocked(); err != nil {
				return err
			}
		}
		if c.connection != nil {
			c.connection.Close()
			c.connection = nil
		}
		if s.hello == "version-mismatch" {
			s.reason = "LidHelperUpdate"
			c.publish()
		}
		s.install = lidInstallDiagnostic{step: "authorize"}
		c.publish()
		err := c.service.Register()
		if err != nil {
			var failure *lidInstallFailure
			if errors.As(err, &failure) {
				s.install = failure.diagnostic
			} else {
				s.install.failure = "script-failed"
			}
		} else {
			zero := 0
			s.install = lidInstallDiagnostic{step: "confirm", exit: &zero}
		}
		// Status is always reread, including cancelled/failed authorization.
		s.service = c.service.Status()
		if s.service == svcRequiresApproval {
			return fail("LidAuthNeeded", "LidAuthNeeded")
		}
		if err != nil {
			if errors.Is(err, errLidDenied) {
				s.service = svcDenied
				return fail("LidDenied", "LidDenied")
			}
			return fail("LidServiceBroken", "LidServiceBroken")
		}
		if err := c.waitForHelper(); err != nil {
			s.install.step, s.install.failure = "hello", "helper-not-ready"
			return fail(err.Error(), err.Error())
		}
	}
	if err := c.connect(); err != nil {
		return fail("LidServiceBroken", "LidServiceBroken")
	}
	req := lid.Request{Op: "enable", MinPercent: min}
	if !end.IsZero() {
		utc := end.UTC()
		req.Deadline = &utc
	}
	r, err := c.connection.Call(req)
	if err != nil {
		c.lost()
		return fail(s.reason, "enable-call-failed")
	}
	c.apply(r)
	// A renewal already saved by the helper may have an unknown readback
	// during its grace period. Initial enable still requires confirmed 1.
	grace := wasOn && r.OK && r.State == "on-ours" && r.Code == "pmset-failed" && (r.Value == nil || *r.Value == -1)
	if !r.OK || r.State != "on-ours" || !grace && (r.Value == nil || *r.Value != 1) {
		key := "LidEnableFailed"
		switch r.Code {
		case "foreign-setting":
			key = "LidForeign"
		case "battery-low", "battery-unknown":
			key = "LidBatteryTooLow"
		case "restore-failed":
			key = "LidRestoreFailed"
		}
		code := "helper-" + r.Code
		if r.Code == "" || r.Code == "ok" {
			code = "helper-invalid-enable-response"
		}
		return fail(key, code)
	}
	if err = c.writeMarker(rand.Text()); err != nil {
		// Do not leave an unmarked lease if the user directory cannot persist it.
		if stopErr := c.stopLocked(); stopErr != nil {
			return fail("LidRestoreFailed", "marker-write-restore-failed")
		}
		return fail("LidEnableFailed", "marker-write-failed")
	}
	c.deadline = end
	s.state = lidOn
	s.reason = ""
	s.lastCode = ""
	s.lastFailure = nil
	return nil
}

// A successful installer exit is not proof that the daemon can serve requests.
// Only an authenticated hello from the expected helper version confirms readiness.
func (c *lidController) waitForHelper() error {
	until := c.now().Add(15 * time.Second)
	wait := c.wait
	if wait == nil {
		wait = time.Sleep
	}
	for {
		c.snapshotValue.service = c.service.Status()
		if c.snapshotValue.service == svcRequiresApproval {
			return errors.New("LidAuthNeeded")
		}
		err := c.connect()
		if err == nil {
			c.snapshotValue.service = svcEnabled
			return nil
		}
		if c.connection != nil {
			c.connection.Close()
			c.connection = nil
		}
		c.snapshotValue.online = false
		if !c.now().Before(until) {
			if err.Error() == "LidHelperUpdate" {
				return err
			}
			return errors.New("LidHelperNotRunning")
		}
		wait(50 * time.Millisecond)
	}
}

func (c *lidController) stopLocked() error {
	s := &c.snapshotValue
	if c.connection == nil {
		if !c.markerExists() && s.state != lidOn && s.state != lidRestoreFailed {
			return nil
		}
		if err := c.connect(); err != nil && !(errors.Is(err, errLidHelperUpdate) && c.connection != nil) {
			c.lost()
			return errors.New("LidRestoreFailed")
		}
	}
	s.state = lidStopping
	c.publish()
	r, err := c.connection.Call(lid.Request{Op: "disable", Reason: "user"})
	if err != nil {
		c.lost()
		return errors.New("LidRestoreFailed")
	}
	c.apply(r)
	if !r.OK || r.Value == nil || *r.Value != 0 {
		if r.State == "on-foreign" {
			return errors.New("LidForeign")
		}
		if r.State != "restore-failed" && r.State != "restoring" {
			s.state = lidError
			s.reason = "LidRestoreFailed"
			return errors.New(s.reason)
		}
		s.state = lidRestoreFailed
		s.reason = "LidRestoreFailed"
		return errors.New(s.reason)
	}
	if err = c.removeMarker(); err != nil {
		s.state = lidRestoreFailed
		s.reason = "LidRestoreFailed"
		return errors.New(s.reason)
	}
	s.state = lidOff
	return nil
}
func (c *lidController) stop() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer func() { c.publish(); c.mu.Unlock() }()
	return c.stopLocked()
}
func (c *lidController) canForce() bool {
	s := c.snapshot()
	if s.value != 1 || s.state == lidOn {
		return false
	}
	return s.online && s.state == lidRestoreFailed && (s.response.State == "restore-failed" || s.response.State == "restoring") || !s.online && s.ours
}
func (c *lidController) force() error {
	if c == nil || !c.Available() {
		return errors.New("LidUnsupported")
	}
	// Refresh immediately before the manual action; an online helper overrides markers.
	c.Inspect()
	if !c.canForce() {
		return errors.New("LidForeign")
	}
	c.mu.Lock()
	defer func() { c.publish(); c.mu.Unlock() }()
	if err := c.forcer.Force(); err != nil {
		return err
	}
	value, err := c.read()
	if err != nil || value != 0 {
		c.snapshotValue.reason = "LidForceRestoreFailed"
		return errors.New("LidForceRestoreFailed")
	}
	c.removeMarker()
	c.snapshotValue = lidSnapshot{state: lidOff, value: 0}
	return nil
}
func (c *lidController) close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer func() { c.publish(); c.mu.Unlock() }()
	if c.connection != nil {
		c.connection.Close()
		c.connection = nil
	}
}

// stopForQuit gives the helper two seconds. EOF and its durable journal remain
// responsible if the app exits while a bounded IPC operation is in flight.
func (c *lidController) stopForQuit() {
	if c == nil {
		return
	}
	done := make(chan struct{})
	go func() { _ = c.stop(); c.close(); close(done) }()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}
