package lid

import (
	"errors"
	"os"
	"sync"
	"time"
)

type SleepSetter interface {
	Read() (int, error)
	Set(int) error
}
type PowerSource interface{ Source() (string, int, error) }
type Journal struct {
	Started    time.Time  `json:"started"`
	Deadline   *time.Time `json:"deadline"`
	MinPercent int        `json:"minPercent"`
	Previous   int        `json:"previous"`
}
type JournalStore interface {
	Load() (*Journal, error)
	Save(Journal) error
	Remove() error
}
type Engine struct {
	mu                                sync.Mutex
	Sleep                             SleepSetter
	Power                             PowerSource
	Store                             JournalStore
	Now                               func() time.Time
	journal                           *Journal
	owner                             uint64
	lastRequest, lastBattery, retryAt time.Time
	failures, unknown                 int
	state, code                       string
	detail                            string
	Logf                              func(string, ...any)
	logged                            *responseLogState
}

func NewEngine(s SleepSetter, p PowerSource, j JournalStore) *Engine {
	return &Engine{Sleep: s, Power: p, Store: j, Now: time.Now, state: "off", code: "ok"}
}

// Start recovers even a corrupt journal: its existence denotes our liability.
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	j, err := e.Store.Load()
	if errors.Is(err, os.ErrNotExist) {
		e.refresh()
		return nil
	}
	if err != nil && j == nil {
		return err
	}
	if j == nil {
		return errors.New("journal unavailable")
	}
	e.journal = j
	e.restore("ok")
	return nil
}

// Observation failures are transient; they must not overwrite a lease's
// termination code and remain stuck after the next successful read.
func (e *Engine) refresh() string {
	if e.journal != nil {
		return ""
	}
	value, err := e.Sleep.Read()
	if err != nil {
		e.state = "off"
		return "pmset-failed"
	}
	if value == 1 {
		e.state = "on-foreign"
		e.code = "foreign-setting"
	} else {
		e.state = "off"
		if e.code == "foreign-setting" {
			e.code = "ok"
		}
	}
	return ""
}
func (e *Engine) response(id string, ok bool, code string) Response {
	observed := e.refresh()
	if code == "" {
		code = e.code
		if observed != "" {
			code = observed
		}
	}
	r := Response{V: Protocol, ID: id, OK: ok, Protocol: Protocol, Helper: Version, State: e.state, Code: code, Detail: e.detail, MinPercent: 20, Power: "unknown"}
	if v, err := e.Sleep.Read(); err == nil && (v == 0 || v == 1) {
		r.Value = &v
	}
	kind, p, err := e.Power.Source()
	r.Power = kind
	if err == nil && p >= 0 && p <= 100 {
		r.Percent = &p
	}
	if e.journal != nil {
		r.MinPercent = e.journal.MinPercent
		deadline := e.effectiveDeadline()
		r.Deadline = &deadline
	}
	return r
}
func (e *Engine) effectiveDeadline() time.Time {
	hard := e.journal.Started.Add(HardLimit)
	if d := e.journal.Deadline; d != nil && d.Before(hard) {
		return *d
	}
	return hard
}
func (e *Engine) Handle(owner uint64, r Request) (out Response) {
	e.mu.Lock()
	defer e.mu.Unlock()
	defer func() {
		state := responseLogState{out.OK, out.Code, out.State, out.Detail, -1}
		if out.Value != nil {
			state.value = *out.Value
		}
		if (r.Op == "enable" || r.Op == "disable") && out.OK || e.logged == nil || *e.logged != state {
			LogResponse(e.Logf, r.Op, out)
		}
		e.logged = &state
	}()
	now := e.Now()
	if code := r.Validate(now); code != "" {
		return e.response(r.ID, false, code)
	}
	e.tick(now)
	if e.owner == owner {
		e.lastRequest = now
	}
	switch r.Op {
	case "enable":
		e.detail = ""
		if e.journal != nil {
			if e.state != "on-ours" {
				return e.response(r.ID, false, "restore-failed")
			}
			if e.owner != owner {
				return e.response(r.ID, false, "busy")
			}
			// Renewal never moves the start (the 12h limit cannot be extended).
			j := *e.journal
			j.Deadline = r.Deadline
			if err := e.Store.Save(j); err != nil {
				e.detail = "journal-save-failed"
				return e.response(r.ID, false, "pmset-failed")
			}
			e.journal = &j
			e.lastRequest = now
			v, err := e.Sleep.Read()
			if err != nil || v != 1 {
				e.detail = "renew-readback-failed"
				e.restore("pmset-failed")
				return e.response(r.ID, false, e.code)
			}
			return e.response(r.ID, true, "ok")
		}
		value, err := e.Sleep.Read()
		if err != nil {
			e.detail = "preflight-read-failed"
			return e.response(r.ID, false, "pmset-failed")
		}
		if value != 0 {
			return e.response(r.ID, false, "foreign-setting")
		}
		min := r.MinPercent
		if min == 0 {
			min = 20
		}
		kind, p, err := e.Power.Source()
		// Unknown capacity is rejected even on AC; this is the conservative rule.
		if err != nil || p < 0 || p > 100 || (kind != "ac" && kind != "battery") || (kind == "battery" && p <= min) {
			return e.response(r.ID, false, "battery-low")
		}
		j := Journal{Started: now, Deadline: r.Deadline, MinPercent: min, Previous: 0}
		if err = e.Store.Save(j); err != nil {
			e.detail = "journal-save-failed"
			return e.response(r.ID, false, "pmset-failed")
		}
		e.journal = &j
		e.owner = owner
		e.lastRequest = now
		e.lastBattery = now
		e.unknown = 0
		if err = e.Sleep.Set(1); err == nil {
			value, err = e.Sleep.Read()
			if err != nil || value != 1 {
				e.detail = "enable-readback-failed"
			}
		} else {
			e.detail = "enable-write-failed"
		}
		if err != nil || value != 1 {
			e.restore("pmset-failed")
			return e.response(r.ID, false, e.code)
		}
		e.state = "on-ours"
		e.code = "ok"
	case "disable":
		if e.journal != nil && e.owner != 0 && e.owner != owner {
			return e.response(r.ID, false, "busy")
		}
		if e.journal != nil {
			e.restore("ok")
		}
	}
	// Select the current code after response refreshes the observed setting.
	return e.response(r.ID, e.state != "restore-failed", "")
}

var backoff = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second}

func (e *Engine) restore(code string) {
	defer func() {
		if e.Logf != nil {
			e.Logf("restore code=%s state=%s detail=%s", e.code, e.state, e.detail)
		}
	}()
	if e.journal == nil {
		return
	}
	e.state = "restoring"
	err := e.Sleep.Set(0)
	if err == nil {
		var v int
		v, err = e.Sleep.Read()
		if err == nil && v != 0 {
			err = errors.New("restore readback")
		}
	}
	if err == nil {
		err = e.Store.Remove()
	}
	if err != nil {
		e.state = "restore-failed"
		e.code = "restore-failed"
		e.retryAt = e.Now().Add(backoff[min(e.failures, len(backoff)-1)])
		e.failures++
		return
	}
	e.journal = nil
	e.owner = 0
	e.state = "off"
	e.code = code
	e.failures = 0
	e.unknown = 0
}
func (e *Engine) Disconnect(owner uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.owner == owner {
		e.restore("ok")
	}
}

// Shutdown runs after request handlers stop. It also covers a failed startup
// recovery with no connected owner. A failed restore keeps the durable journal.
func (e *Engine) Shutdown() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.journal != nil {
		e.restore("ok")
	}
	if e.journal != nil {
		return errors.New("restore-failed")
	}
	return nil
}
func (e *Engine) Tick() { e.mu.Lock(); defer e.mu.Unlock(); e.tick(e.Now()) }
func (e *Engine) tick(now time.Time) {
	if e.journal == nil {
		return
	}
	if e.state == "restore-failed" {
		if !now.Before(e.retryAt) {
			e.restore("ok")
		}
		return
	}
	if !now.Before(e.effectiveDeadline()) {
		code := "ok"
		if !now.Before(e.journal.Started.Add(HardLimit)) {
			code = "max-duration"
		}
		e.restore(code)
		return
	}
	if now.Sub(e.lastRequest) >= Lease {
		e.restore("ok")
		return
	}
	if now.Sub(e.lastBattery) >= BatteryInterval {
		e.lastBattery = now
		kind, p, err := e.Power.Source()
		if err != nil || p < 0 || p > 100 || (kind != "ac" && kind != "battery") {
			e.unknown++
			if e.unknown >= batteryUnknownLimit {
				e.restore("battery-unknown")
			}
			return
		}
		e.unknown = 0
		if p <= e.journal.MinPercent {
			e.restore("battery-low")
		}
	}
}
