package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
)

// Feedback timing. One hint is the first frame without the capsule, the
// entrance, the dwell, then the exit; the window hides only after the exit
// has played, so the next hint starts from a hidden window again.
const (
	feedbackFirstFrame    = 16 * time.Millisecond
	feedbackEnterDuration = 150 * time.Millisecond
	feedbackDwell         = 1200 * time.Millisecond
	feedbackExitDuration  = 150 * time.Millisecond
	feedbackExitDelay     = feedbackExitDuration + feedbackFirstFrame
	feedbackMargin        = 24 // Space24
)

const (
	feedbackNotificationID    = "cubbix.feedback.awake"
	feedbackNotificationGroup = "cubbix.feedback"
)

// feedbackModes and feedbackCorners map the stored strings to the indices the
// segmented controls use. The order is the order of the segments.
var (
	feedbackModes   = []string{"hint", "notification", "off"}
	feedbackCorners = []string{"top-left", "top-right", "bottom-left", "bottom-right", "center"}
)

// feedbackMessage is one hint: a Lucide icon name, an already localized
// sentence, and the notification title the same sentence belongs to. It is
// the only thing a tile has to build; the presenter owns the window,
// placement, timing and the feedback mode.
type feedbackMessage struct {
	Icon   string
	Title  string
	Text   string
	Danger bool // failure; the icon uses ColorDanger
	Muted  bool // a quieter icon, as for "off"
}

// feedbackTimer is what the presenter needs of a scheduled callback. The
// tests replace the scheduler, so no test waits for a real hint to finish.
type feedbackTimer interface {
	stop()
}

type realFeedbackTimer struct{ timer *time.Timer }

func (t realFeedbackTimer) stop() { t.timer.Stop() }

// feedbackWindow exposes only operations that cannot request keyboard focus.
type feedbackWindow interface {
	ShowInactive()
	Hide()
	Close()
	SetPosition(int, int)
	Invalidate()
}

// feedbackPresenter owns the single hint window. Everything runs on the main
// thread; the window is created on the first hint and kept for the next one.
type feedbackPresenter struct {
	window      feedbackWindow
	message     feedbackMessage
	showing     bool // the capsule is built; false during the first and last frame
	visible     bool // the window is on screen
	priming     bool // the empty root must survive until the entrance frame
	displayID   int64
	generation  uint64
	denied      bool // notifications were refused; hints take over for this run
	timers      []feedbackTimer
	offDisplays func()
	position    mygo.Point // the slot's last placement, kept for display changes and tests

	// Seams. Production points them at MyGo and the application; tests
	// replace them so nothing is shown, notified or waited for.
	mode              func() string
	corner            func() string
	create            func(*feedbackPresenter)
	send              func(mygo.NotificationOptions) error
	cursorDisplay     func() mygo.Display
	displays          func() []mygo.Display
	onDisplaysChanged func(func()) func()
	schedule          func(time.Duration, func()) feedbackTimer
	changed           func() // the settings page needs a redraw, as after a denial
}

func newFeedbackPresenter() *feedbackPresenter {
	return &feedbackPresenter{
		mode:   func() string { return "hint" },
		corner: func() string { return "top-right" },
		send: func(options mygo.NotificationOptions) error {
			return mygo.NewNotification(options).Show()
		},
		cursorDisplay: func() mygo.Display {
			return mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
		},
		displays: func() []mygo.Display { return mygo.Screen.Displays() },
		onDisplaysChanged: func(fn func()) func() {
			// The callback comes from the main thread; keep that promise here
			// so the presenter never touches a window from anywhere else.
			return mygo.Screen.OnDisplaysChanged(func() { mygo.RunOnMain(fn) })
		},
		schedule: func(delay time.Duration, run func()) feedbackTimer {
			return realFeedbackTimer{time.AfterFunc(delay, func() { mygo.RunOnMain(run) })}
		},
		changed: func() {},
	}
}

// present dispatches one hint by the effective mode. A notification that the
// system refuses falls back to a hint once, and the refusal is remembered for
// the rest of this run so the user is not asked again.
func (p *feedbackPresenter) present(message feedbackMessage) {
	mode := p.mode()
	if mode == "off" {
		return
	}
	if mode == "notification" {
		if p.denied {
			// Known refusal: do not ask the system again this run.
			p.hint(message)
			return
		}
		if message.Title == "" {
			message.Title = message.Text
		}
		err := p.send(mygo.NotificationOptions{ID: feedbackNotificationID, Title: message.Title, Body: message.Text, Group: feedbackNotificationGroup, Silent: true})
		if err == nil {
			return
		}
		log.Printf("feedback notification: %v", err)
		if errors.Is(err, mygo.ErrNotificationsDenied) {
			p.denied = true
			p.changed()
		}
	}
	p.hint(message)
}

// hint shows or updates the on-screen capsule. One window serves every
// trigger: an arriving message replaces the content and restarts the dwell
// without replaying the entrance.
func (p *feedbackPresenter) hint(message feedbackMessage) {
	p.message = message
	if p.visible && p.priming {
		// Keep the pending empty-root frame even during a burst of shortcuts.
		return
	}
	p.generation++
	generation := p.generation
	p.stopTimers()
	if p.visible {
		// The window is already up, including while the exit plays: build
		// the capsule again and restart the dwell.
		p.showing = true
		p.invalidate()
		p.scheduleDwell(generation, feedbackDwell)
		return
	}
	display := p.cursorDisplay()
	if display.ID == 0 {
		return // no display to put the hint on
	}
	p.displayID = display.ID
	p.visible = true
	p.priming = true
	p.showing = false
	p.ensureWindow()
	p.place(display)
	if p.window != nil {
		p.window.ShowInactive()
		p.invalidate()
	}
	// The root view must be drawn once before the capsule is born, or MyGo
	// has no earlier parent and skips the entrance.
	p.timers = append(p.timers, p.schedule(feedbackFirstFrame, func() {
		if p.generation != generation {
			return
		}
		p.priming = false
		p.showing = true
		p.place(p.currentDisplay())
		p.invalidate()
		p.scheduleDwell(generation, feedbackEnterDuration+feedbackDwell)
	}))
}

func (p *feedbackPresenter) scheduleDwell(generation uint64, delay time.Duration) {
	p.timers = append(p.timers, p.schedule(delay, func() {
		if p.generation != generation {
			return
		}
		// Drop the capsule and let the exit ghost play under the root view.
		p.showing = false
		p.invalidate()
		p.timers = append(p.timers, p.schedule(feedbackExitDelay, func() {
			if p.generation != generation {
				return
			}
			p.visible = false
			if p.window != nil {
				p.window.Hide()
			}
		}))
	}))
}

func (p *feedbackPresenter) stopTimers() {
	for _, timer := range p.timers {
		timer.stop()
	}
	p.timers = p.timers[:0]
}

func (p *feedbackPresenter) invalidate() {
	if p.window != nil {
		p.window.Invalidate()
	}
}

func (p *feedbackPresenter) ensureWindow() {
	if p.window == nil && p.create != nil {
		p.create(p)
	}
}

func (p *feedbackPresenter) currentDisplay() mygo.Display {
	for _, display := range p.displays() {
		if display.ID == p.displayID {
			return display
		}
	}
	if p.displayID != 0 {
		return mygo.Display{}
	}
	return p.cursorDisplay()
}

func (p *feedbackPresenter) place(display mygo.Display) {
	if display.ID == 0 {
		return
	}
	t := tokens(false, "zh-CN")
	x, y := feedbackPosition(display.WorkArea, p.corner(), int(t.SizeToastSlotWidth), int(t.SizeToastHeight), feedbackMargin)
	p.position = mygo.Point{X: x, Y: y}
	if p.window != nil {
		p.window.SetPosition(x, y)
	}
}

// displaysChanged keeps the hint on its own screen. A display that is still
// there moves the window to its new work area; one that is gone hides the
// hint at once, since there is nowhere left for the exit to land.
func (p *feedbackPresenter) displaysChanged() {
	if !p.visible {
		return
	}
	for _, display := range p.displays() {
		if display.ID == p.displayID {
			p.displayID = display.ID
			p.place(display)
			return
		}
	}
	p.cancel()
}

func (p *feedbackPresenter) cancel() {
	p.stopTimers()
	p.generation++
	p.showing = false
	p.priming = false
	p.visible = false
	if p.window != nil {
		p.window.Hide()
	}
}

// watch subscribes to display changes once when application services start.
func (p *feedbackPresenter) watch() {
	if p.offDisplays != nil || p.onDisplaysChanged == nil {
		return
	}
	p.offDisplays = p.onDisplaysChanged(p.displaysChanged)
}

func (p *feedbackPresenter) close() {
	if p.offDisplays != nil {
		p.offDisplays()
		p.offDisplays = nil
	}
	p.cancel()
	if p.window != nil {
		p.window.Close()
		p.window = nil
	}
}

// feedbackPosition puts the fixed-size slot in one corner of a work area, or
// in its center. WorkArea already excludes the menu bar and the Dock.
func feedbackPosition(area mygo.Rectangle, corner string, width, height, margin int) (int, int) {
	left := area.X + margin
	right := area.X + area.Width - width - margin
	top := area.Y + margin
	bottom := area.Y + area.Height - height - margin
	switch corner {
	case "top-left":
		return left, top
	case "bottom-left":
		return left, bottom
	case "bottom-right":
		return right, bottom
	case "center":
		return area.X + (area.Width-width)/2, area.Y + (area.Height-height)/2
	default:
		return right, top
	}
}

// announce is the feedback entry for every tile. The caller supplies an
// already localized icon and sentence; the presenter decides whether the
// user sees a hint, a notification or nothing.
func (a *application) announce(message feedbackMessage) {
	if a.feedback == nil {
		return
	}
	a.feedback.present(message)
}

// announceAwake reports a keep awake action taken outside the menu, the card
// panel and the command palette. err is the return value of this call, not
// the controller state: a rejected mode or an expired deadline leaves the
// snapshot inactive with no error, which would read as "turned off".
func (a *application) announceAwake(err error) {
	locale := uiLocale()
	text := func(key string) string { return a.text(locale, key) }
	if err != nil {
		a.announce(feedbackMessage{Icon: "triangle-alert", Title: text("Awake"), Text: text("ToastAwakeError"), Danger: true})
		return
	}
	state := a.awake.snapshot()
	if !state.Active {
		a.announce(feedbackMessage{Icon: "power", Title: text("Awake"), Text: text("ToastAwakeOff"), Muted: true})
		return
	}
	a.announce(a.awakeFeedbackMessage(locale, state.Session))
}

// awakeFeedbackMessage turns the running session into one sentence. The
// session carries what this action requested (duration, deadline, followed
// process), so a deep link reports its own parameters rather than the
// remembered setting.
func (a *application) awakeFeedbackMessage(locale string, session awakeSession) feedbackMessage {
	text := func(key string) string { return a.text(locale, key) }
	message := feedbackMessage{Icon: "sun", Title: text("Awake")}
	switch {
	case session.PID > 0:
		message.Text = fmt.Sprintf(text("ToastAwakeOnFollow"), session.Name)
	case session.Until != "":
		message.Text = fmt.Sprintf(text("ToastAwakeOnUntil"), session.Until)
	case session.Minutes > 0:
		message.Text = fmt.Sprintf(text("ToastAwakeOn"), awakeDurationText(a, locale, session.Minutes))
	case !session.End.IsZero():
		// An older session keeps only its deadline; show that.
		message.Text = fmt.Sprintf(text("ToastAwakeOnUntil"), session.End.Format("15:04"))
	default:
		message.Text = text("ToastAwakeOnUnlimited")
	}
	return message
}

// uiLocale is the language the app shows, with the source language before
// the app has one (tests, and the first frames of a launch). i18n.Locale
// reads the same value as App.Locale without waiting for the main thread,
// which tests never run.
func uiLocale() string {
	if locale := i18n.Locale(); locale != "" {
		return locale
	}
	return "zh-CN"
}

// --- Settings ---

func feedbackIndex(values []string, value string) int {
	for index, candidate := range values {
		if candidate == value {
			return index
		}
	}
	return 0
}

func (a *application) feedbackModeName() string {
	if a.feedbackMode < 0 || a.feedbackMode >= len(feedbackModes) {
		return feedbackModes[0]
	}
	return feedbackModes[a.feedbackMode]
}

func (a *application) feedbackCornerName() string {
	if a.feedbackCorner < 0 || a.feedbackCorner >= len(feedbackCorners) {
		return "top-right"
	}
	return feedbackCorners[a.feedbackCorner]
}

// effectiveFeedbackMode is what this run actually shows. A refused
// notification degrades to a hint without rewriting the stored choice, so the
// user keeps seeing the mode they picked in Settings.
func (a *application) effectiveFeedbackMode() string {
	mode := a.feedbackModeName()
	if mode == "notification" && a.feedback != nil && a.feedback.denied {
		return "hint"
	}
	return mode
}

// feedbackPositionEditable keeps the position row usable whenever a hint can
// still appear: the hint mode, and the degraded notification mode.
func (a *application) feedbackPositionEditable() bool {
	return a.effectiveFeedbackMode() == "hint"
}

func (a *application) changeFeedbackMode() {
	a.config.FeedbackMode = a.feedbackModeName()
	if a.feedback != nil && a.effectiveFeedbackMode() != "hint" {
		a.feedback.cancel()
	}
	a.save()
}

func (a *application) changeFeedbackCorner() {
	a.config.FeedbackCorner = a.feedbackCornerName()
	if a.feedback != nil && a.feedback.visible {
		a.feedback.place(a.feedback.currentDisplay())
		a.feedback.invalidate()
	}
	a.save()
}
