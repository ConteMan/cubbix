package main

import (
	"errors"
	"github.com/egoist/mygo"
	"time"
)

func (a *application) savedLidPreference() bool {
	return a.lid != nil && a.lid.Available() && a.config.AwakeLid
}
func (a *application) lidPreference() bool {
	if a.lid == nil || !a.lid.Available() {
		return false
	}
	if a.lidPending != nil {
		return *a.lidPending
	}
	return a.config.AwakeLid
}
func (a *application) lidCapable() bool { return a.lid != nil && a.lid.capable }
func (a *application) lidReason() string {
	if a.lid == nil {
		return ""
	}
	if a.lid.capable && !a.lid.supported {
		if a.lid.unavailable != "" {
			return a.lid.unavailable
		}
		return "LidUnsupported"
	}
	if key := a.lid.snapshot().reason; key != "" {
		return key
	}
	return a.lidMessage
}
func (a *application) setSessionLid(value bool) {
	a.awake.mu.Lock()
	a.awake.state.Session.Lid = value
	a.awake.mu.Unlock()
}
func (a *application) completeLid(err error) {
	if err == nil && a.awake.snapshot().Active && a.lid.snapshot().state == lidOn {
		a.config.AwakeLid = true
		a.lidPending = nil
		a.setSessionLid(true)
		a.lidNotice = ""
	} else {
		a.setSessionLid(false)
	}
	if err != nil {
		a.notifyLid(a.lidReason())
	}
	a.awakeChanged()
}
func (a *application) enableLid(end time.Time, authorize bool) error {
	if a.lid == nil || !a.lid.Available() {
		return errors.New("LidUnsupported")
	}
	a.lidMessage = ""
	a.lidNotice = ""
	fromURL := a.lidURL
	min := a.config.AwakeLidBatteryMin
	if min == 0 {
		min = 20
	}
	if !a.lidAsync {
		err := a.lid.enable(end, min, authorize)
		a.completeLid(err)
		if fromURL && err == nil {
			a.lidURLFeedback()
		}
		return err
	}
	if a.lidBusy {
		return errors.New("LidEnableFailed")
	}
	a.lidBusy = true
	a.refreshAwake()
	go func() {
		err := a.lid.enable(end, min, authorize)
		mygo.RunOnMain(func() {
			a.lidBusy = false
			if a.quitting {
				return
			}
			a.completeLid(err)
			if fromURL {
				if err == nil {
					a.lidURLFeedback()
				} else {
					a.announceAwake(err)
				}
			}
		})
	}()
	return nil
}
func (a *application) toggleLid() {
	if a.lid == nil || a.lidBusy {
		return
	}
	if a.lid.snapshot().state == lidOn {
		if err := a.lid.stop(); err != nil {
			a.notifyLid("LidRestoreFailed")
		} else {
			a.config.AwakeLid = false
			a.setSessionLid(false)
		}
		a.awakeChanged()
		return
	}
	if !a.lid.Available() {
		a.panelResult(errors.New(a.lidReason()))
		return
	}
	s := a.awake.snapshot()
	if !s.Active {
		ordinary := a.selectedSession()
		ordinary.Lid = false
		if err := a.awake.start(ordinary); err != nil {
			a.panelResult(err)
			return
		}
		s = a.awake.snapshot()
	}
	a.panelResult(a.enableLid(s.Session.End, true))
}

// syncLid is the common convergence point, including child exits and polling.
func (a *application) syncLid() {
	if a.lid == nil {
		return
	}
	state := a.awake.snapshot()
	ls := a.lid.snapshot()
	if !state.Active && (ls.state == lidOn || ls.state == lidRestoreFailed) {
		if err := a.lid.stop(); err != nil {
			a.notifyLid("LidRestoreFailed")
		}
		a.setSessionLid(false)
	}
}
func (a *application) stopAwake() error {
	if a.lid != nil {
		if err := a.lid.stop(); err != nil {
			a.notifyLid("LidRestoreFailed")
			a.refreshAwake()
			return err
		}
	}
	a.setSessionLid(false)
	a.awake.stop()
	return nil
}
func (a *application) pollLid() {
	if a.lid == nil || !a.lid.Available() || a.lidBusy || a.lidPolling {
		return
	}
	if a.lidAsync {
		a.lidPolling = true
		go func() {
			a.lid.Inspect()
			mygo.RunOnMain(func() {
				a.lidPolling = false
				if !a.quitting {
					a.finishLidPoll()
				}
			})
		}()
		return
	}
	a.lid.Inspect()
	a.finishLidPoll()
}
func (a *application) finishLidPoll() {
	if a.lidBusy {
		return
	}
	s := a.lid.snapshot()
	awake := a.awake.snapshot()
	if awake.Active && awake.Session.Lid && s.state != lidOn {
		a.setSessionLid(false)
		a.awake.stop()
	}
	a.notifyLid(s.reason)
	a.refreshAwake()
}
func (a *application) notifyLid(key string) {
	switch key {
	case "LidRestoreFailed", "LidHelperGone", "LidDenied", "LidForceRestoreFailed", "LidStaleSetting", "LidMaxDuration", "LidBatteryLow", "LidBatteryUnknown":
	default:
		return
	}
	if a.lidNotice == key {
		return
	}
	a.lidNotice = key
	if a.notifyBattery != nil {
		a.notifyBattery(a.text(uiLocale(), "Awake"), a.text(uiLocale(), key))
	}
}
func (a *application) forceLid() {
	if a.lid == nil || a.lidBusy {
		return
	}
	err := a.lid.force()
	if err != nil && err.Error() == "LidForceRestoreFailed" {
		a.notifyLid(err.Error())
	}
	if err == nil || err.Error() == "LidForceRestoreFailed" {
		a.panelResult(err)
	}
	a.refreshAwake()
}

func (a *application) lidURLFeedback() {
	state := a.awake.snapshot()
	if !state.Active || !state.Session.Lid {
		return
	}
	if a.notifyBattery != nil {
		m := a.awakeFeedbackMessage(uiLocale(), a.awake.snapshot().Session)
		a.notifyBattery(m.Title, m.Text)
	}
}
