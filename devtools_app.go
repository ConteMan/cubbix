package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/ConteMan/cubbix/internal/devtools"
)

// devtoolsTab is the window's two segments: time conversion, or UUIDs.
const (
	devtoolsTimeTab = iota
	devtoolsUUIDTab
)

// devtoolsMaxClipboardBytes bounds what the shortcut may prefill. It matches
// the panel's clipboard suggestion: a whole hit inside 128 bytes.
const devtoolsMaxClipboardBytes = 128

// devtoolsPasteDelayDefault is how long a paste waits after the panel closes,
// so the modifiers the user still holds have been released. It is a starting
// point, not a setting.
const devtoolsPasteDelayDefault = 80 * time.Millisecond

// devtoolsRequest is one complete request to open the window with. Kind picks
// the segment; a UUID request carries its version, count and presentation, so
// "uuid7 21" opened in a window still makes 21 v7 values.
type devtoolsRequest struct {
	Kind    devtools.Kind
	Text    string
	Version int
	Count   int
	Upper   bool
	Plain   bool
}

// devtoolsState is the tool window's own state. It lives on the main thread,
// like qrState, and keeps nothing between launches but the two presentation
// switches, which live in the config.
type devtoolsState struct {
	window    *mygo.Window
	testFrame devtoolsWindow
	place     windowPlaceState
	// initialSize is the frame size the window opened with, so a resize is
	// told apart from an automatic placement.
	initialSize screenSize
	pinned      bool
	visible     bool
	closing     bool
	focus       bool
	tab         int
	// input is the time field. output is what it produced, computed only when
	// the text changes so a UUID or a clock face never moves between frames.
	input    string
	output   devtools.Output
	snapshot time.Time
	// uuidVersion is 4, 7, or 0 for the bare pair. values are the canonical
	// lowercase hyphenated texts; the switches only change how they are
	// written, never the values. uuidLiteral is a parsed UUID opened here.
	uuidVersion   int
	uuidCount     int
	uuidCountText string
	uuidCountErr  bool
	uuidLiteral   string
	values        []string
	upper         bool
	plain         bool
	// selected is the row Enter and ⌘C act on. copiedKey and copyFailedKey
	// mark one row's short success or its failure.
	selected      int
	copiedKey     string
	copiedUntil   time.Time
	copyFailedKey string
	// denied is the per-launch accessibility hint under the toolbar. It is an
	// overlay, not a state of the field: typing anything dismisses it.
	denied bool
}

// devtoolsRow is one line the window can copy. A batch is a single row whose
// payload joins every value; lines carries them for the multi-line drawing.
type devtoolsRow struct {
	Key     string
	Label   string
	Title   string
	Hint    string
	Payload string
	Lines   []string
	// ErrorKey marks a row that only explains a failure: it copies nothing.
	ErrorKey string
}

func (a *application) ensureDevtoolsState() *devtoolsState {
	if a.devtools == nil {
		a.devtools = &devtoolsState{tab: devtoolsTimeTab, uuidVersion: 0, uuidCount: 1}
	}
	return a.devtools
}

// devtoolsOnMain applies worker results to the UI. Tests replace the seam so
// no case needs a running application loop.
func (a *application) devtoolsOnMain(fn func()) {
	if a.devtoolsOnMainThread != nil {
		a.devtoolsOnMainThread(fn)
		return
	}
	fn()
}

func (a *application) runDevtoolsWorker(fn func()) {
	if a.devtoolsAsync {
		go fn()
		return
	}
	fn()
}

func (a *application) registerDevtoolsShortcut() {
	a.registerSlot("devtools", a.config.DevtoolsShortcut, a.config.DevtoolsEnabled, a.toggleDevtools)
}

// changeDevtoolsEnabled follows changeQREnabled step for step: the flag is
// written first, an opening registers its slot, and only a close releases the
// combination, clears its diagnostic, cancels a pending read and destroys the
// window.
func (a *application) changeDevtoolsEnabled(target bool) {
	a.cancelShortcutRecording()
	if target == a.config.DevtoolsEnabled {
		a.invalidateWindow()
		return
	}
	a.config.DevtoolsEnabled = target
	if target {
		a.registerDevtoolsShortcut()
	} else {
		a.releaseOwnedShortcut("devtools")
		a.clearSlotError("devtools")
		a.closeDevtools()
	}
	a.save()
	a.invalidateWindow()
	a.refreshMenubarPanel()
	a.invalidatePaletteAfterModuleChange()
}

func (a *application) devtoolsVisible() bool {
	return a.devtools != nil && a.devtools.visible
}

// toggleDevtools is the global shortcut. The AX read runs on a worker, so a
// stuck application can never hold the shortcut callback; the window opens
// only after that read has settled.
func (a *application) toggleDevtools() {
	if a.shortcutRecording || !a.config.DevtoolsEnabled {
		return
	}
	a.beginDevtoolsSelection()
}

// devtoolsPrefill is the clipboard rule of the shortcut path: only a whole hit
// inside devtoolsMaxClipboardBytes prefills the field. A sentence, an over-long
// text or a prefixed input leaves the window empty.
func (a *application) devtoolsPrefill() string {
	if a.devtoolsReadClipboard == nil {
		return ""
	}
	return devtoolsClipboardHit(a.devtoolsReadClipboard())
}

// devtoolsClipboardHit is devtoolsPrefill without the seam, so the rule is
// testable on its own.
func devtoolsClipboardHit(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" || len(text) > devtoolsMaxClipboardBytes {
		return ""
	}
	if _, ok := devtools.ConvertBareTimestamp(text, nil); ok {
		return text
	}
	if output, ok := devtools.ConvertDate(text, nil); ok && output.ErrorKey == "" {
		return text
	}
	if _, ok := devtools.ParseUUID(text); ok {
		return text
	}
	return ""
}

// beginDevtoolsSelection is the shortcut path: the AX read runs on a worker
// with its own generation, so a second press cancels a pending read instead of
// queueing another window.
func (a *application) beginDevtoolsSelection() {
	a.hideMenubarPanel()
	a.hidePalette()
	if a.devtoolsSelectionPending {
		a.devtoolsSelectionGeneration++
		a.devtoolsSelectionPending = false
		a.openDevtools(devtoolsRequest{})
		return
	}
	a.devtoolsSelectionGeneration++
	generation := a.devtoolsSelectionGeneration
	a.devtoolsSelectionPending = true
	a.runDevtoolsWorker(func() {
		answer := a.captureSelection()
		a.devtoolsOnMain(func() {
			if generation != a.devtoolsSelectionGeneration || !a.config.DevtoolsEnabled || a.quitting {
				return
			}
			a.devtoolsSelectionPending = false
			switch {
			case answer.status == selectionFound:
				// A selection that is not a whole devtools hit keeps the window
				// empty: the intent was the selection, not the clipboard.
				text := strings.TrimSpace(answer.text)
				if hit := devtoolsClipboardHit(text); hit != "" {
					a.openDevtools(devtoolsRequestForQuery(hit))
					return
				}
				a.openDevtools(devtoolsRequest{})
			case answer.status == selectionDenied:
				a.openDevtools(devtoolsRequest{})
				a.showDevtoolsSelectionHintOnce()
			case answer.busy:
				// Another read owns the single flight. That is not "nothing is
				// selected", so the clipboard must not stand in for it.
				a.openDevtools(devtoolsRequest{})
			default:
				a.openDevtools(devtoolsRequestForQuery(a.devtoolsPrefill()))
			}
		})
	})
}

func (a *application) cancelDevtoolsSelection() {
	a.devtoolsSelectionGeneration++
	a.devtoolsSelectionPending = false
}

// showDevtoolsSelectionHintOnce is this module's own per-launch marker: it
// never reuses the translate or QR hint, and it never opens a system window.
func (a *application) showDevtoolsSelectionHintOnce() {
	if a.devtoolsSelectionHintShown {
		return
	}
	a.devtoolsSelectionHintShown = true
	s := a.ensureDevtoolsState()
	s.denied = true
	a.refreshDevtools()
}

// openDevtools is the one UI entry: the shortcut, the card panel, the command
// palette and the palette's action all end here. It opens no other tool window
// and closes none.
func (a *application) openDevtools(req devtoolsRequest) {
	if !a.config.DevtoolsEnabled {
		return
	}
	a.cancelDevtoolsSelection()
	a.cancelDevtoolsPaste()
	a.hideMenubarPanel()
	a.hidePalette()
	s := a.ensureDevtoolsState()
	s.copiedKey, s.copiedUntil, s.copyFailedKey = "", time.Time{}, ""
	s.denied = false
	s.focus = true
	// An empty entry keeps the segment chosen in this window. A complete
	// request still selects its own segment below.
	if req == (devtoolsRequest{}) {
		s.input = ""
	} else {
		s.tab = devtoolsTimeTab
		s.input = ""
	}
	s.uuidLiteral = ""
	s.uuidVersion, s.uuidCount = 0, 1
	s.values = nil
	s.upper, s.plain = a.config.DevtoolsUUIDUpper, a.config.DevtoolsUUIDPlain
	if req.Upper {
		s.upper = true
	}
	if req.Plain {
		s.plain = true
	}
	if req.Kind == devtools.KindUUID {
		s.tab = devtoolsUUIDTab
		switch {
		case req.Text != "":
			s.uuidLiteral = req.Text
			s.values = []string{req.Text}
		case req.Version == 0:
			s.uuidVersion, s.uuidCount = 0, 1
			a.regenerateDevtoolsUUID()
		default:
			s.uuidVersion, s.uuidCount = req.Version, max(1, req.Count)
			a.regenerateDevtoolsUUID()
		}
	} else if req == (devtoolsRequest{}) && s.tab == devtoolsUUIDTab {
		a.regenerateDevtoolsUUID()
	} else {
		s.input = req.Text
		a.refreshDevtoolsTime()
	}
	s.uuidCountText = strconv.Itoa(s.uuidCount)
	s.uuidCountErr = false
	s.selected = 0
	if s.frame() == nil && a.createDevtoolsWindow != nil {
		a.createDevtoolsWindow(s)
		a.placeDevtools()
	}
	a.showDevtools()
}

func (a *application) showDevtools() {
	s := a.devtools
	if s == nil {
		return
	}
	if window := s.frame(); window != nil {
		if window.IsMinimized() {
			window.Restore()
		}
		a.reconcileDevtoolsDisplay()
		a.visibility(true)
		s.visible = true
		window.Show()
		window.Focus()
	}
	s.visible = true
	a.refreshDevtools()
}

func (a *application) closeDevtools() {
	a.cancelDevtoolsSelection()
	a.cancelDevtoolsPaste()
	s := a.devtools
	if s == nil || s.closing {
		return
	}
	s.closing = true
	if a.rememberDevtoolsPlace() {
		a.save()
	}
	s.visible = false
	if window := s.frame(); window != nil {
		window.Destroy()
	}
	if a.devtools == s {
		a.devtools = nil
	}
	a.visibility(false)
}

func (a *application) refreshDevtools() {
	if s := a.devtools; s != nil && s.window != nil {
		s.window.Invalidate()
	}
}

// syncDevtoolsInput reads the time field and recomputes at once: this window
// has no debounce, unlike the QR symbol.
func (a *application) syncDevtoolsInput() {
	s := a.devtools
	if s == nil {
		return
	}
	s.denied = false
	s.copyFailedKey = ""
	s.selected = 0
	a.refreshDevtoolsTime()
}

// refreshDevtoolsTime computes the time rows for the current field. An empty
// field is "now", exactly as the command palette's bare ts is.
func (a *application) refreshDevtoolsTime() {
	a.refreshDevtoolsTimeAt(time.Now())
	a.refreshDevtools()
}

// refreshDevtoolsTimeAt computes the rows for one instant. The instant is the
// frame clock while the window is open, so the live tick stays stable.
func (a *application) refreshDevtoolsTimeAt(now time.Time) {
	s := a.devtools
	if s == nil {
		return
	}
	s.snapshot = now
	s.output = devtools.ConvertTime(s.input, now, time.Local)
}

// devtoolsTimeLive reports whether the time tab shows the current instant, so
// the view can tick it once per second while it stays open.
func (s *devtoolsState) timeLive() bool {
	text := strings.TrimSpace(s.input)
	return text == "" || strings.EqualFold(text, "now")
}

// selectDevtoolsTab switches the segment. Each tab's own text is kept, so
// switching back shows what was there.
func (a *application) selectDevtoolsTab(tab int) {
	s := a.devtools
	if s == nil || tab == s.tab {
		return
	}
	s.tab = tab
	s.selected = 0
	s.copiedKey = ""
	s.copyFailedKey = ""
	if tab == devtoolsUUIDTab && len(s.values) == 0 {
		a.regenerateDevtoolsUUID()
	}
	a.refreshDevtools()
}

// regenerateDevtoolsUUID makes a fresh batch with the remembered version and
// count. The switches do not regenerate: they only rewrite the same values.
// selectDevtoolsUUIDVersion switches between the bare pair and a single
// version. Choosing a version is an explicit generation request, so it also
// replaces a literal opened from the palette.
func (a *application) selectDevtoolsUUIDVersion(version int) {
	s := a.devtools
	if s == nil || version != 0 && version != 4 && version != 7 {
		return
	}
	s.uuidVersion = version
	a.regenerateDevtoolsUUID()
}

// syncDevtoolsUUIDCount applies a whole decimal count in the 1-100 range. An
// unfinished or invalid edit shows the existing count error instead of making
// a partial batch. Editing the pair's count chooses v4, making the transition
// from the default pair to a batch explicit in the adjacent segmented control.
func (a *application) syncDevtoolsUUIDCount() {
	s := a.devtools
	if s == nil {
		return
	}
	text := s.uuidCountText
	valid := text != ""
	for _, r := range text {
		valid = valid && r >= '0' && r <= '9'
	}
	count, err := strconv.Atoi(text)
	if !valid || err != nil || count < 1 || count > 100 {
		s.uuidCountErr = true
		s.copyFailedKey = ""
		a.refreshDevtools()
		return
	}
	s.uuidCountErr = false
	if count == s.uuidCount && s.uuidVersion != 0 && s.uuidLiteral == "" {
		a.refreshDevtools()
		return
	}
	s.uuidCount = count
	if s.uuidVersion == 0 {
		s.uuidVersion = 4
	}
	a.regenerateDevtoolsUUID()
}

func (a *application) regenerateDevtoolsUUID() {
	s := a.devtools
	if s == nil {
		return
	}
	s.uuidLiteral = ""
	switch {
	case s.uuidVersion == 7:
		s.uuidCount = max(1, s.uuidCount)
		s.values = devtoolsValues(7, s.uuidCount)
	case s.uuidVersion == 4:
		s.uuidCount = max(1, s.uuidCount)
		s.values = devtoolsValues(4, s.uuidCount)
	default:
		// The bare pair, as the panel's "uuid" shows it.
		s.uuidVersion, s.uuidCount = 0, 1
		s.values = append(devtoolsValues(4, 1), devtoolsValues(7, 1)...)
	}
	s.uuidCountText = strconv.Itoa(s.uuidCount)
	s.uuidCountErr = false
	s.selected = 0
	s.copiedKey = ""
	s.copyFailedKey = ""
	a.refreshDevtools()
}

// devtoolsValues generates canonical values: lowercase and hyphenated. The
// presentation switches are applied when a row is drawn, never here.
func devtoolsValues(version, count int) []string {
	output := devtools.GenerateUUID(version, count, false, false)
	if output.ErrorKey != "" || len(output.Rows) == 0 {
		return nil
	}
	return strings.Split(output.Rows[0].Payload, "\n")
}

// writeDevtoolsSwitches stores the two presentation switches and redraws the
// same values in their new form. No value is regenerated.
func (a *application) writeDevtoolsSwitches(upper, plain bool) {
	s := a.devtools
	if s == nil {
		return
	}
	s.upper, s.plain = upper, plain
	a.config.DevtoolsUUIDUpper, a.config.DevtoolsUUIDPlain = upper, plain
	a.save()
	a.refreshDevtools()
}

// rows is what the current tab lists. It is rebuilt per frame from the cached
// values, so a redraw never generates anything.
func (a *application) devtoolsWindowRows(locale string) []devtoolsRow {
	s := a.devtools
	if s == nil {
		return nil
	}
	if s.tab == devtoolsUUIDTab {
		return a.devtoolsUUIDRows(locale, s)
	}
	text := func(key string) string { return a.text(locale, key) }
	if s.output.ErrorKey != "" {
		label := text(s.output.ErrorKey)
		return []devtoolsRow{{Key: "devtools.error", Title: label, ErrorKey: s.output.ErrorKey}}
	}
	rows := make([]devtoolsRow, 0, len(s.output.Rows))
	for index, row := range s.output.Rows {
		rows = append(rows, devtoolsRow{
			Key:     "devtools.time." + strconv.Itoa(index),
			Title:   row.Title,
			Hint:    a.devtoolsHint(locale, row),
			Payload: row.Payload,
		})
	}
	return rows
}

func (a *application) devtoolsUUIDRows(locale string, s *devtoolsState) []devtoolsRow {
	text := func(key string) string { return a.text(locale, key) }
	format := func(value string) string { return devtools.FormatBatch(value, s.upper, s.plain) }
	if s.uuidCountErr {
		return []devtoolsRow{{Key: "devtools.uuid.count.error", Title: text("DevtoolsCountInvalid"), ErrorKey: "DevtoolsCountInvalid"}}
	}
	if len(s.values) == 0 {
		return nil
	}
	if s.uuidLiteral != "" {
		value := format(s.values[0])
		label := ""
		if output, ok := devtools.ParseUUID(s.values[0]); ok && len(output.Rows) > 0 {
			label = a.devtoolsHint(locale, output.Rows[0])
		}
		return []devtoolsRow{{Key: "devtools.uuid.0", Label: label, Title: value, Payload: value}}
	}
	if s.uuidVersion == 0 && len(s.values) == 2 {
		rows := make([]devtoolsRow, 0, 2)
		for index, version := range []int{4, 7} {
			value := format(s.values[index])
			rows = append(rows, devtoolsRow{Key: "devtools.uuid." + strconv.Itoa(index), Label: "v" + strconv.Itoa(version), Title: value, Payload: value})
		}
		return rows
	}
	values := make([]string, 0, len(s.values))
	for _, value := range s.values {
		values = append(values, format(value))
	}
	payload := strings.Join(values, "\n")
	if len(values) > 1 {
		return []devtoolsRow{{Key: "devtools.uuid.0", Title: values[0], Hint: fmt.Sprintf(text("DevtoolsCount"), len(values)), Lines: values, Payload: payload}}
	}
	return []devtoolsRow{{Key: "devtools.uuid.0", Label: "v" + strconv.Itoa(s.uuidVersion), Title: values[0], Lines: values, Payload: payload}}
}

// copyDevtoolsRow writes one row's payload. A failed write keeps the window
// open and marks that row; a success shows a short check on it.
func (a *application) copyDevtoolsRow(row devtoolsRow) bool {
	s := a.devtools
	if s == nil || row.ErrorKey != "" || row.Payload == "" {
		return false
	}
	if err := a.copyDevtoolsPayload(row.Payload); err != nil {
		s.copyFailedKey = row.Key
		a.refreshDevtools()
		return false
	}
	s.copyFailedKey = ""
	s.copiedKey = row.Key
	s.copiedUntil = time.Now().Add(1500 * time.Millisecond)
	a.refreshDevtools()
	return true
}

func (a *application) copyDevtoolsSelected() bool {
	rows := a.devtoolsWindowRows(uiLocale())
	if a.devtools == nil || a.devtools.selected < 0 || a.devtools.selected >= len(rows) {
		return false
	}
	return a.copyDevtoolsRow(rows[a.devtools.selected])
}

// moveDevtoolsSelection walks the rows the current tab lists.
func (a *application) moveDevtoolsSelection(delta int) {
	s := a.devtools
	if s == nil {
		return
	}
	count := len(a.devtoolsWindowRows(uiLocale()))
	if count == 0 {
		return
	}
	next := s.selected + delta
	if next < 0 || next >= count {
		return
	}
	s.selected = next
	a.refreshDevtools()
}

// pasteDevtoolsSelected is the window's ⌘回车: copy the selected row, and paste
// it only when posting keys is already allowed.
func (a *application) pasteDevtoolsSelected() bool {
	rows := a.devtoolsWindowRows(uiLocale())
	s := a.devtools
	if s == nil || s.selected < 0 || s.selected >= len(rows) {
		return false
	}
	row := rows[s.selected]
	if row.ErrorKey != "" || row.Payload == "" {
		return false
	}
	if err := a.copyDevtoolsPayload(row.Payload); err != nil {
		s.copyFailedKey = row.Key
		a.refreshDevtools()
		return false
	}
	// Record the target while the previous application is still in front. The
	// window then goes away before preflight and the one synthesized Cmd+V.
	target := a.devtoolsTargetPID()
	a.closeDevtools()
	if a.devtoolsPostAllowed() {
		a.scheduleDevtoolsPaste(target)
	}
	return true
}

// devtoolsCopyExpired drops a stale copy marker without a timer: the view asks
// for a frame while the marker shows and clears it later.
func (a *application) devtoolsCopyExpired(now time.Time) bool {
	s := a.devtools
	if s == nil || s.copiedUntil.IsZero() {
		return false
	}
	return !now.Before(s.copiedUntil)
}

func (a *application) setDevtoolsPinned(pinned bool) {
	if s := a.devtools; s != nil {
		s.pinned = pinned
		if window := s.frame(); window != nil {
			window.SetAlwaysOnTop(pinned)
		}
		a.refreshDevtools()
	}
}

func (a *application) devtoolsWindowOptions(t tokenValues, locale string) mygo.WindowOptions {
	return mygo.WindowOptions{
		Title:                a.text(locale, "Devtools"),
		Width:                int(t.SizeDevtoolsWidth),
		Height:               int(t.SizeDevtoolsWindowHeight),
		MinWidth:             int(t.SizeDevtoolsWidth),
		MinHeight:            int(t.SizeDevtoolsMinHeight),
		Hidden:               true,
		TitleBarStyle:        mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: int(t.Space12), Y: int(t.Space16)},
		DisableFullScreen:    true,
		BackgroundColor:      t.ColorCard,
		Page:                 mygo.PageOptions{DevTools: mygo.DevToolsDisabled},
		Content:              ui.View(a.devtoolsView),
	}
}

func (a *application) buildDevtoolsWindow(s *devtoolsState) {
	t := tokens(mygo.Theme.IsDark(), mygo.App.Locale())
	window := mygo.NewWindow(a.devtoolsWindowOptions(t, mygo.App.Locale()))
	s.window = window
	window.OnClose(func(*mygo.CloseEvent) {
		if a.rememberDevtoolsPlace() {
			a.save()
		}
		a.cancelDevtoolsSelection()
		a.cancelDevtoolsPaste()
		s.visible = false
	})
	window.OnClosed(func() {
		if a.devtools == s {
			a.devtools = nil
		}
		a.visibility(false)
	})
	window.OnHide(func() { s.visible = false; a.visibility(false) })
	window.OnMove(a.observeDevtoolsMove)
	window.OnRestore(a.reconcileDevtoolsDisplay)
}

// --- Paste to the frontmost application ---

// pasteDevtoolsPayload writes the clipboard and, only when posting key events
// is already allowed, pastes it into the application that was in front. It
// never asks for permission and never reports a paste that did not happen.
func (a *application) pasteDevtoolsPayload(payload string) error {
	if err := a.copyDevtoolsPayload(payload); err != nil {
		return err
	}
	// Capture the target before closing the overlays. Unauthorized paste still
	// closes after the successful copy; permission never decides visibility.
	target := a.devtoolsTargetPID()
	a.hidePalette()
	a.hideMenubarPanel()
	if !a.devtoolsPostAllowed() {
		return nil
	}
	a.scheduleDevtoolsPaste(target)
	return nil
}

// devtoolsPostAllowed is a preflight only: it reads the current TCC answer and
// cannot show an authorization dialog.
func (a *application) devtoolsPostAllowed() bool {
	return a.devtoolsPasteAccess != nil && a.devtoolsPasteAccess()
}

// devtoolsTargetPID records the application in front before the panel closes.
// A zero answer fails closed: without a target, the scheduler only copies.
func (a *application) devtoolsTargetPID() int32 {
	if a.devtoolsFrontmost == nil {
		return 0
	}
	return a.devtoolsFrontmost()
}

// scheduleDevtoolsPaste closes the overlays and posts one ⌘V after the delay,
// which is a starting point for the user's modifiers to be released. A newer
// request cancels an older one, so one action never posts twice.
func (a *application) scheduleDevtoolsPaste(target int32) {
	a.devtoolsPasteGeneration++
	generation := a.devtoolsPasteGeneration
	post := a.devtoolsPostKey
	a.hidePalette()
	a.hideMenubarPanel()
	if post == nil || target <= 0 {
		return
	}
	run := func() {
		a.devtoolsOnMain(func() {
			if generation != a.devtoolsPasteGeneration || a.quitting || !a.config.DevtoolsEnabled {
				return
			}
			post(target)
		})
	}
	if !a.devtoolsAsync {
		run()
		return
	}
	delay := a.devtoolsPasteDelay
	if delay <= 0 {
		delay = devtoolsPasteDelayDefault
	}
	go func() {
		time.Sleep(delay)
		run()
	}()
}

// cancelDevtoolsPaste drops a pending post: a window closed or reopened before
// it fired must not receive a paste meant for another session.
func (a *application) cancelDevtoolsPaste() {
	a.devtoolsPasteGeneration++
}

// defaultDevtoolsCopy is the clipboard write of a result. It returns the
// failure so the panel can keep itself open and say the copy did not happen.
func defaultDevtoolsCopy(text string) error {
	return mygo.Clipboard.Write(transfer.TextData(text))
}

// devtoolsSuggestion is the empty-query clipboard hint: only a whole hit
// becomes a group. A long text, an ordinary sentence or a prefixed input is
// never read again for this launch of the panel.
func devtoolsSuggestion(text string) []devtools.Row {
	hit := devtoolsClipboardHit(text)
	if hit == "" {
		return nil
	}
	if output, ok := devtools.ConvertBareTimestamp(hit, nil); ok {
		return output.Rows
	}
	if output, ok := devtools.ConvertDate(hit, nil); ok && output.ErrorKey == "" {
		return output.Rows
	}
	if output, ok := devtools.ParseUUID(hit); ok {
		return output.Rows
	}
	return nil
}
