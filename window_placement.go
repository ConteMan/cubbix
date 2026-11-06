package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
)

// placementWindow is the part of a window that placement needs. The settings
// window and every JSON tool window satisfy it, and a test replaces it with a
// fake frame so no case creates a native window.
type placementWindow interface {
	IsVisible() bool
	IsMinimized() bool
	Restore()
	Show()
	Hide()
	Focus()
	Bounds() mygo.Rectangle
	SetPosition(x, y int)
}

// windowPlaceState is one window's move observation. MyGo's OnMove carries no
// origin and no generation, so a baseline plus a pending seat is the only way
// to tell the user's drag from our own placement and from a system restore.
type windowPlaceState struct {
	placing     bool         // a controlled move is running
	observed    bool         // origin and display describe the window
	origin      screenPoint  // last seen top-left corner
	display     mygo.Display // the display it was on
	pending     *seat        // a seat captured but not written yet
	pendingID   int64
	seq         uint64 // order of the pending seat among this run's moves
	needsRepair bool   // hidden or minimized when its display disappeared
}

// nextMoveSeq numbers one observed move. Quitting keeps only the newest seat of
// each display, so a window that moved first and closed last cannot win.
func (a *application) nextMoveSeq() uint64 {
	a.moveSeq++
	return a.moveSeq
}

// settingsPlacementWindow is the settings frame placement works on: the real
// window, or the fake one a test installs.
func (a *application) settingsPlacementWindow() placementWindow {
	if a.settingsFrame != nil {
		return a.settingsFrame()
	}
	if a.window == nil {
		return nil
	}
	return a.window
}

// windowSize is the outer frame of a window, which is what both position and
// seat are measured in.
func windowSize(w placementWindow) screenSize {
	bounds := w.Bounds()
	return screenSize{Width: bounds.Width, Height: bounds.Height}
}

// resolveSettingsDisplay picks the display the settings window belongs on:
// the pointer's display when it follows the pointer, the chosen one when it is
// fixed. matched is false only when a fixed choice could not be identified.
func resolveSettingsDisplay(config Config, displays []mygo.Display, cursor mygo.Point) (mygo.Display, bool) {
	if len(displays) == 0 {
		return mygo.Display{}, false
	}
	if config.WindowPlacement != "display" {
		return displayNearest(displays, cursor), true
	}
	return fixedDisplay(displays, config)
}

// fixedDisplay finds the chosen display: by number, then by a unique name and
// logical size, then by a unique name. A size alone never matches, and a tie
// is never guessed. The primary display is the fallback, and matched is false
// so the settings page can say the choice was not found.
func fixedDisplay(displays []mygo.Display, config Config) (mygo.Display, bool) {
	if len(displays) == 0 {
		return mygo.Display{}, false
	}
	if config.WindowDisplayID != 0 {
		for _, d := range displays {
			if d.ID == config.WindowDisplayID {
				return d, true
			}
		}
	}
	name := strings.TrimSpace(config.WindowDisplay)
	if name != "" {
		width, height, sized := parseDisplaySize(config.WindowDisplaySize)
		var sameSize, sameName []mygo.Display
		for _, d := range displays {
			if strings.TrimSpace(d.Label) != name {
				continue
			}
			sameName = append(sameName, d)
			if sized && d.Bounds.Width == width && d.Bounds.Height == height {
				sameSize = append(sameSize, d)
			}
		}
		if sized && len(sameSize) == 1 {
			return sameSize[0], true
		}
		if len(sameName) == 1 {
			return sameName[0], true
		}
	}
	for _, d := range displays {
		if d.Primary {
			return d, false
		}
	}
	return displays[0], false
}

// parseDisplaySize reads the stored logical size. A malformed value only means
// this display cannot take part in the size match; it never blocks a launch.
func parseDisplaySize(value string) (int, int, bool) {
	width, height, ok := strings.Cut(strings.TrimSpace(value), "x")
	if !ok {
		return 0, 0, false
	}
	w, err := strconv.Atoi(width)
	if err != nil {
		return 0, 0, false
	}
	h, err := strconv.Atoi(height)
	if err != nil {
		return 0, 0, false
	}
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// displaySizeLabel is the stored form of a display's logical size.
func displaySizeLabel(display mygo.Display) string {
	return fmt.Sprintf("%dx%d", display.Bounds.Width, display.Bounds.Height)
}

// displayByID finds the display a window was on, if it is still connected.
func displayByID(displays []mygo.Display, id int64) (mygo.Display, bool) {
	for _, d := range displays {
		if d.ID == id {
			return d, true
		}
	}
	return mygo.Display{}, false
}

// refreshDisplaySnapshot takes a fresh snapshot and keeps it for the frames
// that follow: the settings rows and their height read this one.
func (a *application) refreshDisplaySnapshot() []mygo.Display {
	displays := a.displaysNow()
	if len(displays) > 0 {
		a.knownDisplays = displays
	}
	return a.knownDisplays
}

// displaySnapshot is the snapshot the current frames draw from. It is taken
// when the app is ready, when a window opens and when a display changes, never
// inside a frame.
func (a *application) displaySnapshot() []mygo.Display { return a.knownDisplays }

// sameDisplays reports whether two snapshots describe the same layout: same
// displays, numbers, frames, work areas and scale factors.
func sameDisplays(before, after []mygo.Display) bool {
	if len(before) != len(after) {
		return false
	}
	for index, d := range before {
		next := after[index]
		if d.ID != next.ID || d.Label != next.Label || d.Bounds != next.Bounds ||
			d.WorkArea != next.WorkArea || d.ScaleFactor != next.ScaleFactor {
			return false
		}
	}
	return true
}

// seatPointForWindow is where a window of the current frame size opens on a
// display: its remembered seat when the display has one, the default
// otherwise.
func seatPointForWindow(w placementWindow, display mygo.Display, table seatTable) screenPoint {
	size := windowSize(w)
	remembered, ok := table.lookup(display)
	if !ok {
		return screenPlacement(display, nil, size)
	}
	return screenPlacement(display, &remembered, size)
}

// moveWindowTo is the one controlled move: it drops any pending seat, marks the
// move as ours, moves, then re-reads the frame as the new baseline. An
// automatic placement is never written to the configuration.
func (a *application) moveWindowTo(state *windowPlaceState, w placementWindow, p screenPoint) {
	state.pending, state.pendingID = nil, 0
	state.placing = true
	w.SetPosition(p.X, p.Y)
	bounds := w.Bounds()
	state.placing = false
	state.observed = true
	state.needsRepair = false
	state.origin = screenPoint{X: bounds.X, Y: bounds.Y}
}

// observeBaseline records where a window is without treating it as a move.
func observeBaseline(state *windowPlaceState, w placementWindow, displays []mygo.Display) {
	bounds := w.Bounds()
	state.observed = true
	state.origin = screenPoint{X: bounds.X, Y: bounds.Y}
	if display, ok := windowDisplayMatching(displays, bounds); ok {
		state.display = display
	}
}

// placeSettingsWindow puts the hidden settings window where its display
// remembers it, before it is shown.
func (a *application) placeSettingsWindow() {
	w := a.settingsPlacementWindow()
	if w == nil {
		return
	}
	displays := a.refreshDisplaySnapshot()
	display, _ := resolveSettingsDisplay(a.config, displays, a.cursorNow())
	if display.ID == 0 {
		return
	}
	a.moveWindowTo(&a.settingsPlace, w, seatPointForWindow(w, display, seatTable{seats: &a.config.WindowPositions}))
	a.settingsPlace.display = display
}

// reconcileSettingsDisplayIfNeeded repairs the place of a settings window that
// comes back from the Dock, without following the pointer: a display change
// must not leave it unreachable, and an unchanged layout must not move it.
func (a *application) reconcileSettingsDisplayIfNeeded() {
	w := a.settingsPlacementWindow()
	if w == nil {
		return
	}
	displays := a.refreshDisplaySnapshot()
	if len(displays) == 0 {
		return
	}
	if !a.settingsPlace.observed {
		observeBaseline(&a.settingsPlace, w, displays)
		return
	}
	if !a.settingsPlace.needsRepair && !a.topologyChanged(&a.settingsPlace, displays) {
		return
	}
	a.repairPlacement(&a.settingsPlace, w, displays, a.settingsReselect, seatTable{seats: &a.config.WindowPositions})
}

// settingsReselect answers which display the settings window should reopen on.
func (a *application) settingsReselect(displays []mygo.Display) (mygo.Display, bool) {
	display, _ := resolveSettingsDisplay(a.config, displays, a.cursorNow())
	if display.ID == 0 {
		return mygo.Display{}, false
	}
	return display, true
}

// jsonReselect answers which display a tool window should reopen on: the one
// under the pointer, since JSON windows only ever follow the pointer.
func (a *application) jsonReselect(displays []mygo.Display) (mygo.Display, bool) {
	if len(displays) == 0 {
		return mygo.Display{}, false
	}
	return displayNearest(displays, a.cursorNow()), true
}

// topologyChanged reports whether the display a window was last seen on is
// gone or changed its frame, work area or scale factor. Comparing against the
// cached display is what lets a notification and OnMove arrive in either
// order.
func (a *application) topologyChanged(state *windowPlaceState, displays []mygo.Display) bool {
	if !state.observed || state.display.ID == 0 {
		return false
	}
	current, ok := displayByID(displays, state.display.ID)
	if !ok {
		return true
	}
	return current.Bounds != state.display.Bounds ||
		current.WorkArea != state.display.WorkArea ||
		current.ScaleFactor != state.display.ScaleFactor
}

// repairPlacement moves a window a display change left on a gone display or in
// an unreachable place: the same display keeps the seat it had, mapped from
// the old work area, and a display that disappeared re-selects.
func (a *application) repairPlacement(state *windowPlaceState, w placementWindow, displays []mygo.Display, reselect func([]mygo.Display) (mygo.Display, bool), table seatTable) {
	if w == nil || state.placing {
		return
	}
	display, found := displayByID(displays, state.display.ID)
	var target screenPoint
	if found {
		relative := seatOffset(state.origin, state.display.WorkArea)
		target = screenPlacement(display, &relative, windowSize(w))
	} else {
		next, ok := reselect(displays)
		if !ok {
			return
		}
		display = next
		target = seatPointForWindow(w, display, table)
	}
	a.moveWindowTo(state, w, target)
	state.display = display
	state.needsRepair = false
}

// observeSettingsMove is the settings window's OnMove handler.
func (a *application) observeSettingsMove() {
	w := a.settingsPlacementWindow()
	if w == nil {
		return
	}
	displays := a.displaysNow()
	if len(displays) == 0 {
		return
	}
	a.observeMove(&a.settingsPlace, w, displays, func(displays []mygo.Display) (mygo.Display, bool) {
		return a.settingsReselect(displays)
	}, seatTable{seats: &a.config.WindowPositions})
}

// observeJSONMove is one tool window's OnMove handler.
func (a *application) observeJSONMove(d *jsonDocument) {
	a.observeMove(&d.place, d.frame(), a.displaysNow(), a.jsonReselect, seatTable{seats: &a.config.JSONPositions})
}

// observeMove records a real move as the pending seat to save. A topology
// change first repairs the window's reachable position, so system restores are
// never stored as the user's choice.
func (a *application) observeMove(state *windowPlaceState, w placementWindow, displays []mygo.Display, reselect func([]mygo.Display) (mygo.Display, bool), table seatTable) {
	if w == nil || state.placing || len(displays) == 0 {
		return
	}
	if a.topologyChanged(state, displays) {
		a.repairPlacement(state, w, displays, reselect, table)
		return
	}
	bounds := w.Bounds()
	origin := screenPoint{X: bounds.X, Y: bounds.Y}
	if state.observed && origin == state.origin {
		// A late notification for a move we made ourselves: the baseline
		// already describes it.
		return
	}
	display, ok := windowDisplayMatching(displays, bounds)
	if !ok {
		// Off every display: nothing worth remembering, but the baseline has
		// to follow, or the next notification looks like a move.
		state.observed, state.origin = true, origin
		return
	}
	state.observed, state.origin, state.display = true, origin, display
	if !w.IsVisible() || w.IsMinimized() {
		// A hidden or minimized window is not the user dragging it.
		return
	}
	value := seatOffset(origin, display.WorkArea)
	state.pending, state.pendingID, state.seq = &value, display.ID, a.nextMoveSeq()
}

// captureSeat reads a live frame into the pending seat when the window has
// moved since the last observation. The save paths call it first, so a last
// move whose notification never arrived is still remembered. A changed
// topology drops the pending seat instead: a system restore is not the user's
// position.
func (a *application) captureSeat(state *windowPlaceState, w placementWindow, displays []mygo.Display) {
	if w == nil || state.placing || len(displays) == 0 {
		return
	}
	if a.topologyChanged(state, displays) {
		state.pending, state.pendingID = nil, 0
		return
	}
	bounds := w.Bounds()
	origin := screenPoint{X: bounds.X, Y: bounds.Y}
	if state.observed && origin == state.origin {
		return
	}
	display, ok := windowDisplayMatching(displays, bounds)
	if !ok {
		state.observed, state.origin = true, origin
		return
	}
	value := seatOffset(origin, display.WorkArea)
	state.pending, state.pendingID, state.seq = &value, display.ID, a.nextMoveSeq()
	state.observed, state.origin, state.display = true, origin, display
}

// takePending removes the cached seat, if any.
func takePending(state *windowPlaceState) (int64, seat, uint64, bool) {
	if state.pending == nil {
		return 0, seat{}, 0, false
	}
	id, value, seq := state.pendingID, *state.pending, state.seq
	state.pending, state.pendingID = nil, 0
	return id, value, seq, true
}

// cancelPending drops a seat that a topology change made untrustworthy.
func cancelPending(state *windowPlaceState) {
	state.pending, state.pendingID = nil, 0
}

// rememberSettingsPlace writes where the settings window is, at hide and at
// quit. It reports whether the configuration changed.
func (a *application) rememberSettingsPlace() bool {
	w := a.settingsPlacementWindow()
	if w == nil {
		return false
	}
	displays := a.displaysNow()
	a.captureSeat(&a.settingsPlace, w, displays)
	id, value, _, ok := takePending(&a.settingsPlace)
	if !ok {
		return false
	}
	return seatTable{seats: &a.config.WindowPositions}.storeSeat(id, value)
}

// rememberJSONPlace writes a tool window's place when it closes or is
// destroyed. A seat older than the one this display already committed is
// dropped, so a window that moved first and closed last cannot win.
func (a *application) rememberJSONPlace(d *jsonDocument) bool {
	if d == nil {
		return false
	}
	w := d.frame()
	if w == nil {
		return false
	}
	a.captureSeat(&d.place, w, a.displaysNow())
	id, value, seq, ok := takePending(&d.place)
	if !ok {
		return false
	}
	if committed, seen := a.jsonCommitted[id]; seen && seq <= committed {
		return false
	}
	changed := seatTable{seats: &a.config.JSONPositions}.storeSeat(id, value)
	if a.jsonCommitted == nil {
		a.jsonCommitted = map[int64]uint64{}
	}
	a.jsonCommitted[id] = seq
	return changed
}

// saveWindowPlacesOnQuit writes one save for every window that moved: the
// settings seat, plus the newest seat of each display among the JSON windows.
func (a *application) saveWindowPlacesOnQuit() {
	displays := a.displaysNow()
	changed := false
	if w := a.settingsPlacementWindow(); w != nil {
		a.captureSeat(&a.settingsPlace, w, displays)
		if id, value, _, ok := takePending(&a.settingsPlace); ok {
			table := seatTable{seats: &a.config.WindowPositions}
			if table.storeSeat(id, value) {
				changed = true
			}
		}
	}
	type latest struct {
		value seat
		seq   uint64
	}
	newest := map[int64]latest{}
	for _, d := range a.jsonDocuments {
		if w := d.frame(); w != nil {
			a.captureSeat(&d.place, w, displays)
		}
		id, value, seq, ok := takePending(&d.place)
		if !ok {
			continue
		}
		if committed, seen := a.jsonCommitted[id]; seen && seq <= committed {
			continue
		}
		if current, seen := newest[id]; !seen || seq > current.seq {
			newest[id] = latest{value: value, seq: seq}
		}
	}
	table := seatTable{seats: &a.config.JSONPositions}
	for id, item := range newest {
		if table.storeSeat(id, item.value) {
			changed = true
		}
		if a.jsonCommitted == nil {
			a.jsonCommitted = map[int64]uint64{}
		}
		a.jsonCommitted[id] = item.seq
	}
	if changed {
		a.save()
	}
}

// displaysChanged is the main-thread display notification: 005 keeps its own
// handling, then every settings and JSON window keeps or repairs its place.
func (a *application) displaysChanged() {
	a.paletteDisplaysChanged()
	displays := a.displaysNow()
	if len(displays) == 0 {
		// Keep the last valid state; the next valid snapshot does the work.
		a.invalidateWindow()
		return
	}
	previous := a.knownDisplays
	a.knownDisplays = displays
	if !sameDisplays(previous, displays) {
		a.repairWindowPlaces(displays)
	}
	a.invalidateWindow()
}

// repairWindowPlaces handles one display snapshot for every window. A visible
// window moves at once; a hidden or minimized one only records that it needs
// the repair, so nothing shows, focuses or writes in the background.
func (a *application) repairWindowPlaces(displays []mygo.Display) {
	if w := a.settingsPlacementWindow(); w != nil && a.topologyChanged(&a.settingsPlace, displays) {
		if w.IsVisible() && !w.IsMinimized() {
			a.repairPlacement(&a.settingsPlace, w, displays, a.settingsReselect, seatTable{seats: &a.config.WindowPositions})
		} else {
			cancelPending(&a.settingsPlace)
			a.settingsPlace.needsRepair = true
		}
	}
	for _, d := range a.jsonDocuments {
		w := d.frame()
		if w == nil || d.closed || !a.topologyChanged(&d.place, displays) {
			continue
		}
		// A JSON window has no restore hook, so a minimized one is repaired
		// in place: setting the frame neither shows nor focuses it.
		a.repairPlacement(&d.place, w, displays, a.jsonReselect, seatTable{seats: &a.config.JSONPositions})
	}
}

// How far each additional JSON window on one display moves away from the seat
// that display remembers, and how many slots it tries before overlapping.
const (
	jsonStaggerStep  = 24
	jsonStaggerSlots = 5
)

// placeJSONWindow opens a new tool window on the display under the pointer: at
// the seat that display remembers, moved to the first slot no live window of
// the same display already uses.
func (a *application) placeJSONWindow(d *jsonDocument) {
	w := d.frame()
	if w == nil {
		return
	}
	displays := a.displaysNow()
	if len(displays) == 0 {
		return
	}
	display := displayNearest(displays, a.cursorNow())
	if display.ID == 0 {
		return
	}
	table := seatTable{seats: &a.config.JSONPositions}
	base := seatPointForWindow(w, display, table)
	size := windowSize(w)
	target := jsonStaggerTarget(base, size, display.WorkArea, a.jsonOccupied(displays, display, d))
	a.moveWindowTo(&d.place, w, target)
	d.place.display = display
}

// jsonOccupied lists the frames of the live JSON windows on one display. A
// window already dragged to another display occupies nothing here, and the
// window being placed is not one of them.
func (a *application) jsonOccupied(displays []mygo.Display, target mygo.Display, self *jsonDocument) []mygo.Rectangle {
	var occupied []mygo.Rectangle
	for _, d := range a.jsonDocuments {
		if d == self || d.closed {
			continue
		}
		w := d.frame()
		if w == nil {
			continue
		}
		bounds := w.Bounds()
		if owner, ok := windowDisplayMatching(displays, bounds); !ok || owner.ID != target.ID {
			continue
		}
		occupied = append(occupied, bounds)
	}
	return occupied
}

// jsonStaggerTarget picks where the next window of a display goes: the first
// candidate at base plus k steps that no live window already uses, else the
// candidate covered least. Clamped candidates are de-duplicated, so a window
// against an edge does not pretend to offer five places.
func jsonStaggerTarget(base screenPoint, size screenSize, work mygo.Rectangle, occupied []mygo.Rectangle) screenPoint {
	var candidates []screenPoint
	for k := 0; k < jsonStaggerSlots; k++ {
		candidate := clampToWorkArea(screenPoint{X: base.X + k*jsonStaggerStep, Y: base.Y + k*jsonStaggerStep}, size, work)
		if !slices.Contains(candidates, candidate) {
			candidates = append(candidates, candidate)
		}
	}
	best, bestArea := candidates[0], -1
	for _, candidate := range candidates {
		rect := screenRect(candidate, size)
		area, taken := 0, false
		for _, other := range occupied {
			if other.X == candidate.X && other.Y == candidate.Y {
				taken = true
			}
			area += overlapArea(rect, other)
		}
		if !taken {
			return candidate
		}
		if bestArea < 0 || area < bestArea {
			best, bestArea = candidate, area
		}
	}
	return best
}

// overlapArea is the positive area two rectangles share.
func overlapArea(a, b mygo.Rectangle) int {
	width := min(a.X+a.Width, b.X+b.Width) - max(a.X, b.X)
	height := min(a.Y+a.Height, b.Y+b.Height) - max(a.Y, b.Y)
	return max(width, 0) * max(height, 0)
}

// displayChoice is one row of the display menu: the value the control stores
// is the display number as text; the label is what the menu shows.
type displayChoice struct {
	Value string
	Label string
	Title string // full name for the tooltip and the accessible name
}

// displayChoices lists the connected displays in system order, the primary one
// first. A display without a name shows its logical size, a name shared by two
// displays adds the size, and two identical displays add their position in
// this menu only: none of that is ever stored as an identity.
func displayChoices(displays []mygo.Display) []displayChoice {
	names := map[string]int{}
	for _, d := range displays {
		names[displayPrimaryName(d)]++
	}
	used := map[string]int{}
	choices := make([]displayChoice, 0, len(displays))
	for _, d := range displays {
		name := displayPrimaryName(d)
		label := name
		if strings.TrimSpace(d.Label) != "" && names[name] > 1 {
			label = name + " · " + displaySizeLabel(d)
		}
		used[label]++
		if used[label] > 1 {
			label = fmt.Sprintf("%s (%d)", label, used[label])
		}
		choices = append(choices, displayChoice{Value: seatKey(d.ID), Label: label, Title: label})
	}
	return choices
}

// displayPrimaryName is the display's own name, or its logical size when the
// system gave it none.
func displayPrimaryName(display mygo.Display) string {
	if name := strings.TrimSpace(display.Label); name != "" {
		return name
	}
	return displaySizeLabel(display)
}

// hasWindowDisplayChoice reports whether the user ever picked a display. A
// size alone is not a choice.
func (a *application) hasWindowDisplayChoice() bool {
	return a.config.WindowDisplayID != 0 || strings.TrimSpace(a.config.WindowDisplay) != ""
}

// setWindowDisplay stores a display as the fixed choice: its number, the name
// the system gave it and its logical size. Automatic matching never writes
// these; only an explicit choice does.
func (a *application) setWindowDisplay(display mygo.Display) bool {
	size := displaySizeLabel(display)
	changed := a.config.WindowDisplayID != display.ID ||
		a.config.WindowDisplay != display.Label ||
		a.config.WindowDisplaySize != size
	a.config.WindowDisplayID = display.ID
	a.config.WindowDisplay = display.Label
	a.config.WindowDisplaySize = size
	return changed
}

// changeWindowPlacement stores the mode. A first switch to the fixed mode,
// with no choice stored yet, adopts the display the window overlaps most, or
// the one under the pointer when it is off screen. The open window does not
// move: the choice applies the next time it opens.
func (a *application) changeWindowPlacement(mode string) {
	if mode != "cursor" && mode != "display" {
		return
	}
	changed := a.config.WindowPlacement != mode
	a.config.WindowPlacement = mode
	if mode == "display" && !a.hasWindowDisplayChoice() {
		if display, ok := a.settingsWindowDisplay(); ok {
			a.setWindowDisplay(display)
			changed = true
		}
	}
	if changed {
		a.save()
	}
}

// settingsWindowDisplay is the display the settings window belongs to right
// now: the one it overlaps most, or the pointer's when it is off every display.
func (a *application) settingsWindowDisplay() (mygo.Display, bool) {
	displays := a.displaysNow()
	if len(displays) == 0 {
		return mygo.Display{}, false
	}
	if w := a.settingsPlacementWindow(); w != nil {
		if w.IsVisible() {
			if display, ok := windowDisplayMatching(displays, w.Bounds()); ok {
				return display, true
			}
		}
	}
	return displayNearest(displays, a.cursorNow()), true
}

// commitWindowDisplayID stores one menu choice. The display list is read again
// before anything is written: a display that disappeared after the frame was
// drawn cannot become the choice.
func (a *application) commitWindowDisplayID(value string) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id == 0 {
		return
	}
	display, ok := displayByID(a.displaysNow(), id)
	if !ok {
		return
	}
	if a.setWindowDisplay(display) {
		a.save()
	}
}
