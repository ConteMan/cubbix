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

// PowerSource is a single power sample. The adapter signal is part of the
// sample so the helper and the app preflight share one Plugged rule.
type PowerSource interface{ Sample() (PowerSample, error) }

type Journal struct {
	Started    time.Time  `json:"started"`
	Deadline   *time.Time `json:"deadline"`
	MinPercent int        `json:"minPercent"`
	Previous   int        `json:"previous"`

	// Writer records who wrote the 1. Empty means a journal from helper
	// revision 4 or earlier; those only ever wrote their own 1, so an empty
	// value always means "ours" (see the upgrade notes in the design).
	Writer      string    `json:"writer,omitempty"` // "ours" | "foreign"
	Rewrites    int       `json:"rewrites,omitempty"`
	WindowAt    time.Time `json:"windowAt,omitempty"`
	WindowCount int       `json:"windowCount,omitempty"`
	LastRewrite time.Time `json:"lastRewrite,omitempty"`
}

type JournalStore interface {
	Load() (*Journal, error)
	Save(Journal) error
	Remove() error
}

type Engine struct {
	mu      sync.Mutex
	Sleep   SleepSetter
	Power   PowerSource
	Store   JournalStore
	Now     func() time.Time
	Logf    func(string, ...any)
	logged  *responseLogState
	journal *Journal
	owner   uint64
	// lastObserve is shared by tick and refresh: one ping observes once.
	lastRequest, lastBattery, retryAt, lastObserve time.Time
	lastValue                                      int
	lastCode                                       string
	failures, unknown, readFailures                int
	rewriteFails                                   int
	state, code, detail                            string
}

func NewEngine(s SleepSetter, p PowerSource, j JournalStore) *Engine {
	return &Engine{Sleep: s, Power: p, Store: j, Now: time.Now, state: "off", code: "ok", lastValue: -1}
}

// writerOf is the single ownership decision. An absent or empty Writer is an
// older helper's journal, which always described our own 1.
func (e *Engine) writerOf() string {
	if e.journal != nil && e.journal.Writer == "foreign" {
		return "foreign"
	}
	return "ours"
}

// Start recovers even a corrupt journal: its existence denotes our liability.
// A journal that names a foreign writer is only an intention record; dropping
// it never touches the system setting.
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	j, err := e.Store.Load()
	if errors.Is(err, os.ErrNotExist) {
		e.lastObserve = time.Time{}
		e.observe(e.Now(), true)
		return nil
	}
	if err != nil && j == nil {
		return err
	}
	if j == nil {
		return errors.New("journal unavailable")
	}
	e.journal = j
	if e.writerOf() == "foreign" {
		e.dropWatch("ok")
		e.lastObserve = time.Time{}
		e.observe(e.Now(), true)
		return nil
	}
	e.restore("ok")
	return nil
}

// setObservation keeps the response value in step with a confirmed write or
// restore. refresh is rate limited, so it must not report a stale value.
func (e *Engine) setObservation(value int, code string) {
	e.lastValue, e.lastCode = value, code
}

// observe is the only read path used by the 1s tick, request handling and
// refresh. A request forces one observation; the timer and refresh share
// lastObserve so a single ping reads once and consumes at most one rewrite
// budget slot.
func (e *Engine) observe(now time.Time, force bool) (int, string) {
	if !force && now.Sub(e.lastObserve) < ObserveInterval {
		return e.lastValue, e.lastCode
	}
	e.lastObserve = now
	value, err := e.Sleep.Read()
	if err != nil || value != 0 && value != 1 {
		value = -1
	}
	value, code := e.observeValue(value, now)
	e.setObservation(value, code)
	return value, code
}

// observeValue is the single value dispatcher. Only a definite 0 can start a
// rewrite or release an owned journal; a 1 keeps the liability, and a failed
// read keeps it through the bounded readFailureLimit counter.
func (e *Engine) observeValue(value int, now time.Time) (int, string) {
	if e.state == "restore-failed" {
		// The bounded retry loop owns this state until it converges.
		return value, ""
	}
	if value == 0 || value == 1 {
		e.readFailures = 0
	}
	if e.journal == nil {
		switch value {
		case 1:
			e.state, e.code = "on-foreign", "foreign-setting"
		case 0:
			e.state = "off"
			if e.code == "foreign-setting" {
				e.code = "ok"
			}
		default:
			e.state = "off"
			return -1, "pmset-failed"
		}
		return value, ""
	}
	if e.writerOf() == "foreign" {
		if value == 0 {
			return e.takeover(now)
		}
		if value < 0 {
			return -1, "pmset-failed"
		}
		return value, ""
	}
	switch value {
	case 1:
		e.detail = ""
		e.state = "on-ours"
		return 1, ""
	case 0:
		return e.rewrite(now)
	default:
		e.readFailures++
		if e.readFailures >= readFailureLimit {
			e.detail = "poll-read-failed"
			e.restore("pmset-failed")
			if e.journal == nil {
				return 0, "pmset-failed"
			}
			return -1, "restore-failed"
		}
		return -1, "pmset-failed"
	}
}

func (e *Engine) refresh() (int, string) { return e.observe(e.Now(), false) }

func (e *Engine) response(id string, ok bool, code string) Response {
	value, observed := e.refresh()
	if code == "" {
		code = e.code
		if observed != "" {
			code = observed
		}
	}
	return e.responseValue(id, ok, code, value)
}

// Mutation paths pass their confirmed readback, avoiding a redundant read
// that could fail after the write has already been verified successfully.
func (e *Engine) responseValue(id string, ok bool, code string, value int) Response {
	r := Response{V: Protocol, ID: id, OK: ok && e.state != "restore-failed", Protocol: Protocol, Helper: Version, State: e.state, Code: code, Detail: e.detail, MinPercent: 20, Power: "unknown"}
	if value == 0 || value == 1 {
		r.Value = &value
	}
	sample, err := e.Power.Sample()
	r.Power = sample.Kind
	r.Adapter = AdapterLabel(sample.Adapter)
	if err == nil && sample.Percent >= 0 && sample.Percent <= 100 {
		r.Percent = &sample.Percent
	}
	if e.journal != nil {
		r.MinPercent = e.journal.MinPercent
		r.Rewrites = e.journal.Rewrites
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
		// An invalid request must never observe (and therefore never write).
		return e.responseValue(r.ID, false, code, e.lastValue)
	}
	// A bounded request in flight is not an idle/disconnected owner. Do not
	// charge its pmset execution time against the next heartbeat's lease.
	defer func() {
		if e.journal != nil && e.owner == owner {
			e.lastRequest = e.Now()
		}
	}()
	e.tick(now, true)
	if e.owner == owner {
		e.lastRequest = now
	}
	switch r.Op {
	case "enable":
		e.detail = ""
		if e.journal != nil {
			if e.state != "on-ours" && e.state != "watching" {
				return e.response(r.ID, false, "restore-failed")
			}
			if e.owner != owner {
				return e.response(r.ID, false, "busy")
			}
			// Renewal never moves the start, never writes the setting and
			// never resets the rewrite budget.
			j := *e.journal
			j.Deadline = r.Deadline
			if err := e.Store.Save(j); err != nil {
				e.detail = "journal-save-failed"
				return e.response(r.ID, false, "pmset-failed")
			}
			e.journal = &j
			e.lastRequest = now
			// A saved renewal owns the same lease. Observe once using the
			// polling grace period rather than restoring on a single error.
			return e.response(r.ID, true, "")
		}
		value, err := e.Sleep.Read()
		if err != nil {
			e.detail = "preflight-read-failed"
			return e.response(r.ID, false, "pmset-failed")
		}
		min := r.MinPercent
		if min == 0 {
			min = 20
		}
		sample, sampleErr := e.Power.Sample()
		// Unknown capacity is rejected even on AC; this is the conservative rule.
		low := sampleErr != nil || sample.Percent < 0 || sample.Percent > 100 ||
			!Plugged(sample) && sample.Percent <= min
		if value != 0 {
			if !r.AdoptForeign || value != 1 {
				return e.response(r.ID, false, "foreign-setting")
			}
			if low {
				return e.response(r.ID, false, "battery-low")
			}
			return e.watch(owner, r, now, min)
		}
		if low {
			return e.response(r.ID, false, "battery-low")
		}
		j := Journal{Started: now, Deadline: r.Deadline, MinPercent: min, Previous: 0, Writer: "ours"}
		if err = e.Store.Save(j); err != nil {
			e.detail = "journal-save-failed"
			return e.response(r.ID, false, "pmset-failed")
		}
		e.journal = &j
		e.owner = owner
		e.lastRequest = now
		e.lastBattery = now
		e.lastObserve = now
		e.unknown = 0
		e.readFailures = 0
		e.rewriteFails = 0
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
		e.readFailures = 0
		e.setObservation(1, "")
		return e.responseValue(r.ID, true, "ok", value)
	case "disable":
		if e.journal != nil && e.owner != 0 && e.owner != owner {
			return e.response(r.ID, false, "busy")
		}
		if e.journal != nil {
			e.end("ok")
			if e.journal == nil {
				value, err := e.Sleep.Read()
				if err != nil || value != 0 && value != 1 {
					value = -1
				}
				return e.responseValue(r.ID, true, e.code, value)
			}
		}
	}
	// Select the current code after response refreshes the observed setting.
	return e.response(r.ID, e.state != "restore-failed", "")
}

// watch registers the intention to keep the lid closed without writing a
// single byte to the system setting: the current 1 belongs to another tool.
// All lease fields must be set; zero Started/lastRequest would end the lease
// at the first tick (hard limit and 15s lease respectively).
func (e *Engine) watch(owner uint64, r Request, now time.Time, min int) Response {
	j := Journal{Started: now, Deadline: r.Deadline, MinPercent: min, Previous: 0, Writer: "foreign"}
	if err := e.Store.Save(j); err != nil {
		e.detail = "journal-save-failed"
		return e.response(r.ID, false, "pmset-failed")
	}
	e.journal = &j
	e.owner = owner
	e.lastRequest = now
	e.lastBattery = now
	e.lastObserve = now
	e.unknown = 0
	e.readFailures = 0
	e.rewriteFails = 0
	e.state = "watching"
	e.code = "ok"
	e.detail = ""
	e.setObservation(1, "")
	return e.responseValue(r.ID, true, "ok", 1)
}

// takeover declares ownership before writing. The journal is saved first, so
// a crash after Set(1) always leaves somebody responsible for the 1.
func (e *Engine) takeover(now time.Time) (int, string) {
	if !e.allowRewrite(now) {
		// The observation confirmed 0. The foreign 1 is gone; dropping our
		// intention never writes a value.
		e.dropWatch("contested")
		return 0, ""
	}
	j := *e.journal
	j.Writer = "ours"
	j.Started = now
	if err := e.Store.Save(j); err != nil {
		e.detail = "journal-save-failed"
		return 0, "pmset-failed"
	}
	e.journal = &j
	return e.writeOne(now, true)
}

// rewrite is only reached after a definite read of 0. Budget exhaustion and
// repeated write failures both end the lease, but only after another read
// confirms the value is still 0: a 1 must keep its journal.
func (e *Engine) rewrite(now time.Time) (int, string) {
	if e.rewriteFails >= RewriteFailLimit {
		return e.finishRewrite("external-release-failed")
	}
	if !e.rewriteGapOpen(now) {
		// A definite 0 was read, but the minimum spacing has not elapsed.
		// Keep the owned journal and retry on a later observation; this is
		// not a conflict and must not consume or end the lease.
		return 0, ""
	}
	if !e.rewriteBudgetOpen(now) {
		return e.finishRewrite("contested")
	}
	return e.writeOne(now, false)
}

func (e *Engine) finishRewrite(code string) (int, string) {
	value, err := e.Sleep.Read()
	if err != nil || value != 0 && value != 1 {
		// Unconfirmed: keep the journal and fall back to the read limit.
		e.readFailures++
		if e.readFailures >= readFailureLimit {
			e.detail = "poll-read-failed"
			e.restore("pmset-failed")
			return -1, "restore-failed"
		}
		return -1, "pmset-failed"
	}
	if value == 1 {
		// Our last write (or another tool) left a 1. Never drop its owner.
		e.state, e.code, e.detail = "on-ours", "ok", ""
		return 1, ""
	}
	e.dropWatch(code)
	return 0, ""
}

// rewriteGapOpen keeps two observations within RewriteGap from writing twice.
func (e *Engine) rewriteGapOpen(now time.Time) bool {
	j := e.journal
	return j != nil && (j.LastRewrite.IsZero() || now.Sub(j.LastRewrite) >= RewriteGap)
}

// rewriteBudgetOpen is a pure decision. It does not mutate the journal;
// writeOne persists the budget fields after this returns true.
func (e *Engine) rewriteBudgetOpen(now time.Time) bool {
	j := e.journal
	if j == nil {
		return false
	}
	if !j.WindowAt.IsZero() && now.Sub(j.WindowAt) < RewriteWindow && j.WindowCount >= RewriteWindowLimit {
		return false
	}
	if j.Rewrites >= RewriteSessionLimit {
		return false
	}
	return e.rewriteFails < RewriteFailLimit
}

func (e *Engine) allowRewrite(now time.Time) bool {
	return e.rewriteGapOpen(now) && e.rewriteBudgetOpen(now)
}

// writeOne persists ONLY budget fields (never Started or Deadline) before
// writing, then reads back. Takeover and rewrite share this write path; only
// takeover has already changed Writer and Started.
func (e *Engine) writeOne(now time.Time, takingOver bool) (int, string) {
	j := *e.journal
	j.Rewrites++
	if j.WindowAt.IsZero() || now.Sub(j.WindowAt) >= RewriteWindow {
		j.WindowAt = now
		j.WindowCount = 0
	}
	j.WindowCount++
	j.LastRewrite = now
	if err := e.Store.Save(j); err != nil {
		e.detail = "journal-save-failed"
		e.rewriteFails++
		e.state = "on-ours"
		return 0, "pmset-failed"
	}
	e.journal = &j
	err := e.Sleep.Set(1)
	value := 0
	if err == nil {
		value, err = e.Sleep.Read()
		if err != nil {
			value = -1
		}
	} else {
		value = 0
	}
	if err == nil && value == 1 {
		e.state, e.code, e.detail = "on-ours", "ok", ""
		e.rewriteFails = 0
		if takingOver {
			e.detail = "adopted"
		} else {
			e.detail = "external-release"
		}
		return 1, ""
	}
	e.rewriteFails++
	e.state, e.code = "on-ours", "ok"
	if takingOver {
		if err != nil {
			e.detail = "takeover-write-failed"
		} else {
			e.detail = "takeover-readback-failed"
		}
	} else {
		e.detail = "rewrite-failed"
	}
	return value, "pmset-failed"
}

var backoff = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second}

// restore is the only path that writes 0. It is reached only for a journal
// whose writer is ours (or an older journal with no writer field).
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
		// The 0 was never confirmed. Keep the last known value so the app can
		// still see an unconfirmed liability and offer its recovery entry.
		e.setObservation(e.lastValue, "restore-failed")
		return
	}
	e.journal = nil
	e.owner = 0
	e.state = "off"
	e.code = code
	e.failures = 0
	e.unknown = 0
	e.readFailures = 0
	e.rewriteFails = 0
	e.setObservation(0, "")
}

// end sends every termination path through the ownership split. A foreign
// writer's 1 is never ours to clear.
func (e *Engine) end(code string) {
	if e.journal != nil && e.writerOf() == "foreign" {
		e.dropWatch(code)
		return
	}
	e.restore(code)
}

// dropWatch releases a lease WITHOUT calling Sleep.Set. It is used both for a
// foreign writer and for an owned lease whose value has just been confirmed 0.
func (e *Engine) dropWatch(code string) {
	if err := e.Store.Remove(); err != nil {
		e.detail = "journal-remove-failed"
	}
	e.journal = nil
	e.owner = 0
	e.state = "off"
	e.code = code
	if e.detail != "journal-remove-failed" {
		e.detail = ""
	}
	e.failures = 0
	e.unknown = 0
	e.readFailures = 0
	e.rewriteFails = 0
}

func (e *Engine) Disconnect(owner uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.owner == owner {
		e.end("ok")
	}
}

// Shutdown runs after request handlers stop. It also covers a failed startup
// recovery with no connected owner. A failed restore keeps the durable journal.
func (e *Engine) Shutdown() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.journal != nil {
		e.end("ok")
	}
	if e.journal != nil {
		return errors.New("restore-failed")
	}
	return nil
}

func (e *Engine) Tick() { e.mu.Lock(); defer e.mu.Unlock(); e.tick(e.Now(), false) }

func (e *Engine) tick(now time.Time, force bool) {
	// Observation rides the existing 1s server tick, so it does not depend on
	// the app being online while the lid is closed.
	e.observe(now, force)
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
		e.end(code)
		return
	}
	if now.Sub(e.lastRequest) >= Lease {
		e.end("ok")
		return
	}
	if now.Sub(e.lastBattery) >= BatteryInterval {
		e.lastBattery = now
		sample, err := e.Power.Sample()
		if err != nil || sample.Percent < 0 || sample.Percent > 100 {
			e.unknown++
			if e.unknown >= batteryUnknownLimit {
				e.end("battery-unknown")
			}
			return
		}
		e.unknown = 0
		if !Plugged(sample) && sample.Percent <= e.journal.MinPercent {
			e.end("battery-low")
		}
	}
}
