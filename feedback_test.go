package main

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

// --- Fakes: no window, no notification, no waiting ---

type fakeFeedbackTimer struct {
	at      time.Duration
	run     func()
	stopped bool
	fired   bool
}

func (t *fakeFeedbackTimer) stop() { t.stopped = true }

type fakeFeedbackClock struct {
	timers []*fakeFeedbackTimer
	now    time.Duration
}

func (c *fakeFeedbackClock) after(delay time.Duration, run func()) feedbackTimer {
	timer := &fakeFeedbackTimer{at: c.now + delay, run: run}
	c.timers = append(c.timers, timer)
	return timer
}

// fire runs the earliest timer that is still scheduled.
func (c *fakeFeedbackClock) fire() bool {
	for _, timer := range c.timers {
		if timer.fired || timer.stopped {
			continue
		}
		timer.fired = true
		c.now = timer.at
		timer.run()
		return true
	}
	return false
}

func (c *fakeFeedbackClock) pending() int {
	count := 0
	for _, timer := range c.timers {
		if !timer.fired && !timer.stopped {
			count++
		}
	}
	return count
}

func testFeedbackDisplay(id int64, x int) mygo.Display {
	return mygo.Display{ID: id, WorkArea: mygo.Rectangle{X: x, Y: 24, Width: 1440, Height: 876}}
}

type feedbackHarness struct {
	presenter *feedbackPresenter
	clock     *fakeFeedbackClock
	sent      []mygo.NotificationOptions
	changed   int
	displays  []mygo.Display
	mode      string
}

func newFeedbackHarness() *feedbackHarness {
	h := &feedbackHarness{mode: "hint", displays: []mygo.Display{testFeedbackDisplay(1, 0)}, clock: &fakeFeedbackClock{}}
	p := newFeedbackPresenter()
	p.schedule = h.clock.after
	p.create = nil
	p.changed = func() { h.changed++ }
	p.send = func(options mygo.NotificationOptions) error {
		h.sent = append(h.sent, options)
		return nil
	}
	p.cursorDisplay = func() mygo.Display { return h.displays[0] }
	p.displays = func() []mygo.Display { return h.displays }
	p.mode = func() string { return h.mode }
	h.presenter = p
	return h
}

// testFeedback installs a harness presenter on the test application.
func testFeedback(a *application) *feedbackHarness {
	h := newFeedbackHarness()
	h.presenter.corner = a.feedbackCornerName
	a.feedback = h.presenter
	return h
}

// The shortcuts page grew to five rows; both tiles pages must still fit the
// minimum window without changing its height when switched.
func TestFeedbackShortcutsPageHeight(t *testing.T) {
	tk := tokens(false, "zh-CN")
	if content := 5*tk.SizeRowHeight + 4*tk.StrokeHairline; content != 244 {
		t.Fatalf("five rows take %v; want 244", content)
	}
	shortcuts := settingsHeight(tk, 3, false)
	if shortcuts != 292 {
		t.Fatalf("shortcuts page = %d; want 292", shortcuts)
	}
	// The window keeps its own minimum size, so switching to a shorter page
	// never resizes it, and five rows fit inside that minimum.
	width, minimum := settingsMinimumSize()
	if width != 720 || minimum < shortcuts {
		t.Fatalf("minimum window = %dx%d; five rows need %d", width, minimum, shortcuts)
	}
}

// --- Position ---

func TestFeedbackPosition(t *testing.T) {
	area := mygo.Rectangle{X: 100, Y: 24, Width: 1440, Height: 876}
	width, height, margin := 320, 36, 24
	cases := map[string]mygo.Point{
		"top-left":      {X: 124, Y: 48},
		"top-right":     {X: 1196, Y: 48},
		"bottom-left":   {X: 124, Y: 840},
		"bottom-right":  {X: 1196, Y: 840},
		"center":        {X: 660, Y: 444},
		"unknown-value": {X: 1196, Y: 48},
	}
	for corner, want := range cases {
		x, y := feedbackPosition(area, corner, width, height, margin)
		if got := (mygo.Point{X: x, Y: y}); got != want {
			t.Fatalf("%s = %+v; want %+v", corner, got, want)
		}
	}
	// A second display at another origin keeps the same 24-point margins.
	second := mygo.Rectangle{X: 1600, Y: 0, Width: 1920, Height: 1080}
	if x, y := feedbackPosition(second, "top-right", width, height, margin); x != 3176 || y != 24 {
		t.Fatalf("second display top-right = %d,%d", x, y)
	}
	if x, y := feedbackPosition(second, "bottom-left", width, height, margin); x != 1624 || y != 1020 {
		t.Fatalf("second display bottom-left = %d,%d", x, y)
	}
}

// --- Copy ---

func TestFeedbackAwakeCopy(t *testing.T) {
	a := awakeTestApp(t)
	until := time.Date(2026, 10, 31, 2, 30, 0, 0, time.Local)
	cases := []struct {
		locale  string
		session awakeSession
		want    string
	}{
		{"zh-CN", awakeSession{Minutes: 60, End: until}, "常亮已开启 · 1 小时"},
		{"zh-CN", awakeSession{Minutes: 45, End: until}, "常亮已开启 · 45 分钟"},
		{"zh-CN", awakeSession{}, "常亮已开启 · 不限时"},
		{"zh-CN", awakeSession{Until: "02:30", End: until}, "常亮已开启 · 直到 02:30"},
		{"zh-CN", awakeSession{PID: 42, Name: "Google Chrome"}, "常亮已开启 · 跟随 Google Chrome"},
		{"en", awakeSession{Minutes: 60, End: until}, "Keep awake on · 1 hour"},
		{"en", awakeSession{Minutes: 45, End: until}, "Keep awake on · 45 min"},
		{"en", awakeSession{}, "Keep awake on · No limit"},
		{"en", awakeSession{Until: "02:30", End: until}, "Keep awake on · Until 02:30"},
		{"en", awakeSession{PID: 42, Name: "Google Chrome"}, "Keep awake on · Following Google Chrome"},
	}
	for _, test := range cases {
		message := a.awakeFeedbackMessage(test.locale, test.session)
		if message.Text != test.want || message.Icon != "sun" || message.Danger {
			t.Fatalf("%s %+v = %+v; want %q", test.locale, test.session, message, test.want)
		}
		if message.Title != a.text(test.locale, "Awake") {
			t.Fatalf("%s title = %q", test.locale, message.Title)
		}
	}
	// An older session keeps only its deadline; show the time instead of a
	// misleading duration.
	a.config.AwakeMinutes = 30
	if got := a.awakeFeedbackMessage("zh-CN", awakeSession{End: until}); got.Text != "常亮已开启 · 直到 02:30" {
		t.Fatalf("deadline-only copy = %q", got.Text)
	}
}

func TestFeedbackAnnounceFailureAndOff(t *testing.T) {
	a := awakeTestApp(t)
	h := testFeedback(a)
	a.announceAwake(errors.New("expired mode"))
	locale := uiLocale()
	if h.presenter.message.Text != a.text(locale, "ToastAwakeError") || !h.presenter.message.Danger || h.presenter.message.Icon != "triangle-alert" {
		t.Fatalf("failure copy = %+v", h.presenter.message)
	}
	h.presenter.message = feedbackMessage{}
	a.announceAwake(nil) // inactive snapshot: turned off
	if h.presenter.message.Text != a.text(locale, "ToastAwakeOff") || !h.presenter.message.Muted || h.presenter.message.Icon != "power" {
		t.Fatalf("off copy = %+v", h.presenter.message)
	}
}

// --- Trigger scope ---

func TestFeedbackOnlyShortcutsAndURLsAnnounce(t *testing.T) {
	a := awakeTestApp(t)
	h := testFeedback(a)
	// The global shortcut reports both directions.
	a.shortcutToggleAwake()
	if !a.awake.snapshot().Active || h.presenter.message.Text == "" {
		t.Fatalf("shortcut start did not announce: %+v", h.presenter.message)
	}
	h.presenter.message = feedbackMessage{}
	a.shortcutToggleAwake()
	locale := uiLocale()
	if a.awake.snapshot().Active || h.presenter.message.Text != a.text(locale, "ToastAwakeOff") {
		t.Fatalf("shortcut stop did not announce: %+v", h.presenter.message)
	}

	// The menu, the card panel and the command palette all show their own
	// state, so none of them announces.
	for name, action := range map[string]func(){
		"menu toggle": a.toggleAwake,
		"card switch": a.panelToggleAwake,
		"palette start": func() {
			_ = a.runAwakeStart(request{Command: &command{ID: "awake.start"}, Awake: awakeArg{Kind: awakeArgDefault}})
		},
	} {
		h.presenter.message = feedbackMessage{}
		action()
		if h.presenter.message.Text != "" {
			t.Fatalf("%s announced: %+v", name, h.presenter.message)
		}
	}
	// A URL has no other visible result and still reports.
	h.presenter.message = feedbackMessage{}
	if err := a.runURL(paletteURLRequest{Kind: "awake", Minutes: 60, HasMinutes: true}); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(a.text(locale, "ToastAwakeOn"), a.text(locale, "R05Hour"))
	if h.presenter.message.Text != want {
		t.Fatalf("URL did not announce its own request: %+v; want %q", h.presenter.message, want)
	}
	// A silent path still returns the failure; only the shortcut announces it.
	a.awake.stop()
	a.config.AwakeMinutes = -1
	h.presenter.message = feedbackMessage{}
	a.toggleAwake()
	if h.presenter.message.Text != "" {
		t.Fatalf("silent toggle announced: %+v", h.presenter.message)
	}
	if err := a.toggleAwakeResult(); err == nil {
		t.Fatal("failed start returned no error")
	}
	a.config.AwakeMinutes = 60
}

// --- Merging and timing ---

func TestFeedbackMergesOneCapsule(t *testing.T) {
	h := newFeedbackHarness()
	p := h.presenter
	first := feedbackMessage{Icon: "sun", Text: "常亮已开启 · 1 小时"}
	p.present(first)
	if !p.visible || p.showing || h.clock.pending() != 1 {
		t.Fatalf("first frame should show the window without the capsule: %+v", p)
	}
	if !h.clock.fire() || !p.showing || h.clock.pending() != 1 {
		t.Fatal("entrance frame did not build the capsule and the dwell")
	}
	// The same message only restarts the dwell: no new entrance.
	p.present(first)
	if !p.showing || !p.visible || h.clock.pending() != 1 {
		t.Fatalf("repeat did not merge: showing=%v visible=%v pending=%d", p.showing, p.visible, h.clock.pending())
	}
	// A different message replaces the content in place.
	p.present(feedbackMessage{Icon: "power", Text: "常亮已关闭"})
	if p.message.Text != "常亮已关闭" || !p.showing || h.clock.pending() != 1 {
		t.Fatalf("different message did not replace: %+v", p.message)
	}
	dwell := h.clock.timers[len(h.clock.timers)-1].run
	// The dwell drops the capsule; the exit keeps the window until it ends.
	if !h.clock.fire() || p.showing || !p.visible || h.clock.pending() != 1 {
		t.Fatal("dwell did not leave the window up for the exit")
	}
	exit := h.clock.timers[len(h.clock.timers)-1].run
	// A message arriving during the exit cancels the pending hide and
	// rebuilds the capsule.
	p.present(feedbackMessage{Icon: "sun", Text: "常亮已开启 · 不限时"})
	if !p.showing || !p.visible || h.clock.pending() != 1 {
		t.Fatalf("exit-phase message did not revive the capsule: %+v", p)
	}
	// Stale callbacks from the previous generation must not hide it.
	dwell()
	exit()
	if !p.visible || !p.showing {
		t.Fatal("a stale timer hid the new hint")
	}
	// The new dwell and exit finish the hint.
	if !h.clock.fire() || p.showing || !p.visible {
		t.Fatal("new dwell did not start the exit")
	}
	if !h.clock.fire() || p.visible {
		t.Fatal("exit did not hide the window")
	}
}

// Record the native-facing calls too: state flags alone cannot prove that a
// window was shown without focus, or hidden only after its exit finished.
type fakeFeedbackWindow struct {
	shows, hides, invalidations int
	closed                      bool
}

func (w *fakeFeedbackWindow) ShowInactive()        { w.shows++ }
func (w *fakeFeedbackWindow) Hide()                { w.hides++ }
func (w *fakeFeedbackWindow) Close()               { w.closed = true }
func (w *fakeFeedbackWindow) SetPosition(int, int) {}
func (w *fakeFeedbackWindow) Invalidate()          { w.invalidations++ }

func TestFeedbackTimingMatchesSpec(t *testing.T) {
	h := newFeedbackHarness()
	w := &fakeFeedbackWindow{}
	creates := 0
	h.presenter.create = func(p *feedbackPresenter) { creates++; p.window = w }
	h.presenter.present(feedbackMessage{Text: "first"})
	// Five triggers within the first frame must leave the empty root alive.
	for i := range 5 {
		h.presenter.present(feedbackMessage{Text: fmt.Sprint(i)})
	}
	if h.presenter.showing || creates != 1 || w.shows != 1 || h.clock.pending() != 1 {
		t.Fatal("burst skipped the empty root frame or created another window")
	}
	h.clock.fire()
	if h.clock.now != 16*time.Millisecond || !h.presenter.showing || h.presenter.message.Text != "4" {
		t.Fatal("entrance did not start at the first frame with the latest message")
	}
	h.clock.fire()
	if h.clock.now != 1366*time.Millisecond || h.presenter.showing || w.hides != 0 {
		t.Fatalf("exit must follow 150 ms entrance + 1200 ms dwell: now=%v hides=%d", h.clock.now, w.hides)
	}
	h.clock.fire()
	if h.clock.now != 1532*time.Millisecond || w.hides != 1 || h.presenter.visible {
		t.Fatalf("window must hide after the exit and one frame: now=%v hides=%d", h.clock.now, w.hides)
	}
	h.presenter.present(feedbackMessage{Text: "next"})
	if creates != 1 || w.shows != 2 || h.presenter.showing {
		t.Fatal("a later hint must reuse its window and prime a new root frame")
	}
	h.presenter.close()
	if !w.closed || h.clock.pending() != 0 {
		t.Fatal("close leaked a window or timer")
	}
}

// --- Modes and denial ---

func TestFeedbackModesAndDenial(t *testing.T) {
	// Hint: no notification call.
	h := newFeedbackHarness()
	h.presenter.present(feedbackMessage{Icon: "sun", Text: "on"})
	if len(h.sent) != 0 || !h.presenter.visible {
		t.Fatalf("hint mode notified or stayed hidden: sent=%d visible=%v", len(h.sent), h.presenter.visible)
	}

	// Notification: one silent notification with a fixed ID, no hint.
	h = newFeedbackHarness()
	h.mode = "notification"
	h.presenter.present(feedbackMessage{Icon: "sun", Title: "Awake", Text: "on"})
	if len(h.sent) != 1 || h.presenter.visible {
		t.Fatalf("notification mode = sent %d, hint %v", len(h.sent), h.presenter.visible)
	}
	options := h.sent[0]
	if options.ID != feedbackNotificationID || options.Group != feedbackNotificationGroup || !options.Silent || options.Title != "Awake" || options.Body != "on" {
		t.Fatalf("notification options = %+v", options)
	}

	// Denied: fall back to a hint once and never ask again this run.
	h = newFeedbackHarness()
	h.mode = "notification"
	h.presenter.send = func(mygo.NotificationOptions) error {
		h.sent = append(h.sent, mygo.NotificationOptions{})
		return mygo.ErrNotificationsDenied
	}
	h.presenter.present(feedbackMessage{Icon: "sun", Title: "Awake", Text: "on"})
	if !h.presenter.denied || h.changed != 1 || !h.presenter.visible || h.presenter.message.Text != "on" {
		t.Fatalf("denial did not degrade: denied=%v changed=%d visible=%v", h.presenter.denied, h.changed, h.presenter.visible)
	}
	h.presenter.present(feedbackMessage{Icon: "power", Text: "off"})
	if len(h.sent) != 1 {
		t.Fatalf("denied presenter notified again: %d", len(h.sent))
	}
	if !h.presenter.visible || h.presenter.message.Text != "off" {
		t.Fatal("denied presenter did not keep using hints")
	}

	// Off: nothing at all, even when notifications would be refused.
	h = newFeedbackHarness()
	h.mode = "off"
	h.presenter.present(feedbackMessage{Icon: "sun", Text: "on"})
	if len(h.sent) != 0 || h.presenter.visible || h.presenter.denied {
		t.Fatalf("off mode = sent %d visible %v denied %v", len(h.sent), h.presenter.visible, h.presenter.denied)
	}
}

func TestFeedbackPositionEditable(t *testing.T) {
	a := awakeTestApp(t)
	h := testFeedback(a)
	set := func(mode string, denied bool) {
		a.feedbackMode = feedbackIndex(feedbackModes, mode)
		h.presenter.denied = denied
	}
	set("hint", false)
	if !a.feedbackPositionEditable() {
		t.Fatal("hint mode must allow a position")
	}
	set("notification", false)
	if a.effectiveFeedbackMode() != "notification" || a.feedbackPositionEditable() {
		t.Fatal("a working notification mode must lock the position")
	}
	set("notification", true)
	if a.effectiveFeedbackMode() != "hint" || !a.feedbackPositionEditable() {
		t.Fatal("a degraded notification must allow the position")
	}
	set("off", false)
	if a.feedbackPositionEditable() {
		t.Fatal("off mode must lock the position")
	}
}

// --- Battery stays independent ---

func TestFeedbackSettingsDoNotSilenceBattery(t *testing.T) {
	a := awakeTestApp(t)
	sent := 0
	battery := newBatteryNotifier()
	battery.show = func(mygo.NotificationOptions) error { sent++; return nil }
	battery.record = func(err error) { t.Fatalf("unexpected battery error: %v", err) }
	a.notifyBattery = battery.send
	a.feedbackMode = feedbackIndex(feedbackModes, "off")
	a.notifyBattery("title", "body")
	if sent != 1 || battery.denied {
		t.Fatalf("feedback off silenced battery: sent=%d denied=%v", sent, battery.denied)
	}
	// A refused hint notification and a refused battery notification keep
	// separate memories.
	h := testFeedback(a)
	h.mode = "notification"
	h.presenter.send = func(mygo.NotificationOptions) error { return mygo.ErrNotificationsDenied }
	h.presenter.present(feedbackMessage{Icon: "sun", Text: "on"})
	if !h.presenter.denied || battery.denied {
		t.Fatalf("denial leaked between hint and battery: hint=%v battery=%v", h.presenter.denied, battery.denied)
	}
}

// --- Displays ---

func TestFeedbackDisplayChanges(t *testing.T) {
	h := newFeedbackHarness()
	p := h.presenter
	p.corner = func() string { return "top-right" }
	p.present(feedbackMessage{Icon: "sun", Text: "on"})
	h.clock.fire()
	if p.position != (mygo.Point{X: 1096, Y: 48}) {
		t.Fatalf("initial position = %+v", p.position)
	}
	// The same display, a new work area: move without losing the hint.
	h.displays = []mygo.Display{testFeedbackDisplay(1, 200)}
	p.displaysChanged()
	if p.position != (mygo.Point{X: 1296, Y: 48}) || !p.visible || !p.showing {
		t.Fatalf("reflow = %+v visible=%v showing=%v", p.position, p.visible, p.showing)
	}
	// The display is gone: hide at once and stop the timers.
	h.displays = []mygo.Display{testFeedbackDisplay(2, 0)}
	p.displaysChanged()
	if p.visible || p.showing || h.clock.pending() != 0 {
		t.Fatalf("unplugged display kept the hint: visible=%v showing=%v pending=%d", p.visible, p.showing, h.clock.pending())
	}
	// Nothing visible and nothing to do.
	p.displaysChanged()

	// No display at all: no window, no timer.
	h = newFeedbackHarness()
	h.displays = nil
	h.presenter.cursorDisplay = func() mygo.Display { return mygo.Display{} }
	h.presenter.present(feedbackMessage{Icon: "sun", Text: "on"})
	if h.presenter.visible || h.clock.pending() != 0 {
		t.Fatal("hint started without a display")
	}
}

func TestFeedbackSettingsApplyToVisibleHint(t *testing.T) {
	a := awakeTestApp(t)
	h := testFeedback(a)
	w := &fakeFeedbackWindow{}
	h.presenter.window = w
	a.feedbackMode = feedbackIndex(feedbackModes, "hint")
	h.presenter.present(feedbackMessage{Text: "on"})
	h.clock.fire()
	pendingDwell := h.clock.timers[len(h.clock.timers)-1].run
	a.feedbackCorner = feedbackIndex(feedbackCorners, "bottom-left")
	a.changeFeedbackCorner()
	if h.presenter.position != (mygo.Point{X: 24, Y: 840}) || h.clock.pending() != 1 {
		t.Fatal("position change did not move the current hint without resetting its dwell")
	}
	a.feedbackMode = feedbackIndex(feedbackModes, "off")
	a.changeFeedbackMode()
	pendingDwell()
	if h.presenter.visible || h.presenter.showing || w.hides != 1 || h.clock.pending() != 0 {
		t.Fatal("off failed to cancel an existing hint or left a live timer")
	}
}

func TestFeedbackDisplaySubscriptionIsReleased(t *testing.T) {
	h := newFeedbackHarness()
	subscribed, released := 0, 0
	var changed func()
	h.presenter.onDisplaysChanged = func(fn func()) func() {
		subscribed++
		changed = fn
		return func() { released++ }
	}
	h.presenter.watch()
	h.presenter.watch()
	h.presenter.present(feedbackMessage{Text: "on"})
	initial := h.clock.timers[0].run
	h.displays = nil
	changed()
	initial()
	if subscribed != 1 || h.presenter.visible || h.presenter.showing || h.clock.pending() != 0 {
		t.Fatal("unplug during priming failed to cancel the pending entrance")
	}
	h.presenter.close()
	h.presenter.close()
	if released != 1 {
		t.Fatal("display subscription was not released exactly once")
	}
}
