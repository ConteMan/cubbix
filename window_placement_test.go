package main

import (
	"testing"

	"github.com/egoist/mygo"
)

// fakeFrame is a window a test moves without a native window: it records every
// controlled move and reports the frame back the way MyGo's setters do.
type fakeFrame struct {
	bounds                   mygo.Rectangle
	visible, minimized       bool
	moves                    []screenPoint
	shows, focuses, restores int
}

func (f *fakeFrame) IsVisible() bool        { return f.visible }
func (f *fakeFrame) IsMinimized() bool      { return f.minimized }
func (f *fakeFrame) Restore()               { f.restores++; f.minimized = false }
func (f *fakeFrame) Show()                  { f.visible = true; f.shows++ }
func (f *fakeFrame) Hide()                  { f.visible = false }
func (f *fakeFrame) Focus()                 { f.focuses++ }
func (f *fakeFrame) Bounds() mygo.Rectangle { return f.bounds }
func (f *fakeFrame) SetPosition(x, y int) {
	f.moves = append(f.moves, screenPoint{X: x, Y: y})
	f.bounds.X, f.bounds.Y = x, y
}

// drag moves the frame the way AppKit would, without a controlled move.
func (f *fakeFrame) drag(x, y int) {
	f.bounds.X, f.bounds.Y = x, y
}

// placementTestApp is an app with fake displays, a pointer and a settings
// frame: nothing in these tests reads a real screen or creates a window.
func placementTestApp(t *testing.T) (*application, *fakeFrame) {
	t.Helper()
	a := awakeTestApp(t)
	f := &fakeFrame{bounds: mygo.Rectangle{Width: 900, Height: 520}}
	a.settingsFrame = func() placementWindow { return f }
	a.knownDisplays = a.displaysNow()
	return a, f
}

// setTestCursor moves the fake pointer.
func setTestCursor(a *application, p mygo.Point) {
	a.cursorPoint = func() mygo.Point { return p }
}

func TestFixedDisplayResolution(t *testing.T) {
	main := mygo.Display{ID: 1, Label: "Main", Primary: true, Bounds: mygo.Rectangle{X: 0, Y: 0, Width: 1440, Height: 900}}
	main.WorkArea = mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	left := mygo.Display{ID: 2, Label: "Studio", Bounds: mygo.Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1080}}
	left.WorkArea = mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055}
	twin := mygo.Display{ID: 3, Label: "Studio", Bounds: left.Bounds}
	twin.WorkArea = left.WorkArea
	displays := []mygo.Display{main, left}
	cases := []struct {
		name     string
		config   Config
		displays []mygo.Display
		want     int64
		matched  bool
	}{
		{"the id wins", Config{WindowDisplayID: 2, WindowDisplay: "gone"}, displays, 2, true},
		{"unique name and size", Config{WindowDisplayID: 99, WindowDisplay: "Studio", WindowDisplaySize: "1920x1080"}, displays, 2, true},
		// A resolution change keeps the display when the name is unique.
		{"unique name after a size change", Config{WindowDisplay: "Studio", WindowDisplaySize: "1280x720"}, displays, 2, true},
		{"ambiguous name falls back", Config{WindowDisplay: "Studio"}, []mygo.Display{main, left, twin}, 1, false},
		{"name and size together still resolve", Config{WindowDisplay: "Studio", WindowDisplaySize: "1920x1080"}, []mygo.Display{main, left, twin}, 1, false},
		// A size alone never matches: two devices can be the same model.
		{"size alone never matches", Config{WindowDisplay: "", WindowDisplaySize: "1920x1080"}, displays, 1, false},
		{"blank name and no id", Config{WindowDisplay: "  "}, displays, 1, false},
		{"missing choice", Config{}, displays, 1, false},
		{"malformed size is ignored", Config{WindowDisplay: "Studio", WindowDisplaySize: "1920*1080"}, displays, 2, true},
		{"no primary takes the first", Config{WindowDisplay: "Gone"}, []mygo.Display{{ID: 8, Label: "a", Bounds: main.Bounds}, {ID: 9, Label: "b", Bounds: main.Bounds}}, 8, false},
		{"no displays at all", Config{WindowDisplay: "Studio"}, nil, 0, false},
	}
	for _, c := range cases {
		display, matched := fixedDisplay(c.displays, c.config)
		if display.ID != c.want || matched != c.matched {
			t.Errorf("%s: got %d, %v; want %d, %v", c.name, display.ID, matched, c.want, c.matched)
		}
	}
	// Following the pointer reports a match and reads the nearest display.
	if display, matched := resolveSettingsDisplay(Config{WindowPlacement: "cursor"}, displays, mygo.Point{X: -400, Y: 300}); !matched || display.ID != 2 {
		t.Fatalf("cursor mode = %d, %v", display.ID, matched)
	}
	if display, _ := resolveSettingsDisplay(Config{WindowPlacement: "display", WindowDisplayID: 2}, displays, mygo.Point{}); display.ID != 2 {
		t.Fatalf("display mode = %d", display.ID)
	}
	// The stored size label round trips through the matcher.
	label := displaySizeLabel(left)
	if label != "1920x1080" {
		t.Fatalf("size label = %q", label)
	}
	if width, height, ok := parseDisplaySize(label); !ok || width != 1920 || height != 1080 {
		t.Fatalf("parsed size = %d x %d, %v", width, height, ok)
	}
}

func TestShowPlacesOnlyWhenHidden(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	// Hidden: one controlled move before Show, centred on the pointer's
	// display (0, 25, 1440, 875) for a 900 × 520 frame.
	if len(f.moves) != 1 || f.moves[0] != (screenPoint{X: 270, Y: 202}) {
		t.Fatalf("first show moved to %+v", f.moves)
	}
	if !f.visible || f.shows != 1 || f.focuses != 1 {
		t.Fatalf("first show: visible=%v shows=%d focuses=%d", f.visible, f.shows, f.focuses)
	}
	// A visible window is only shown and focused again: no second move, even
	// after the pointer went to another display.
	setTestCursor(a, mygo.Point{X: -400, Y: 300})
	a.show()
	if len(f.moves) != 1 || f.shows != 2 || f.focuses != 2 {
		t.Fatalf("repeat show moved the window: %+v", f.moves)
	}
}

func TestShowAfterHideFollowsThePointer(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	a.hide()
	if f.visible {
		t.Fatal("hide left the window visible")
	}
	setTestCursor(a, mygo.Point{X: -400, Y: 300})
	a.show()
	want := screenPoint{X: -1920 + (1920-900)/2, Y: 25 + (1055-520)/2}
	if len(f.moves) != 2 || f.moves[1] != want {
		t.Fatalf("second show moved to %+v; want %+v", f.moves, want)
	}
}

func TestShowMinimizedRestoresWithoutFollowingThePointer(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	placed := f.moves[0]
	f.minimized = true
	setTestCursor(a, mygo.Point{X: -400, Y: 300})
	a.show()
	if f.restores != 1 || f.minimized {
		t.Fatalf("minimized window was not restored: %d", f.restores)
	}
	if len(f.moves) != 1 || f.moves[0] != placed {
		t.Fatalf("restore moved the window: %+v", f.moves)
	}
}

func TestSettingsHideSavesTheLastMove(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	// A real drag, whose notification arrives normally.
	f.drag(500, 300)
	a.observeSettingsMove()
	if len(a.config.WindowPositions) != 0 {
		t.Fatalf("a move was saved before the window hid: %+v", a.config.WindowPositions)
	}
	a.hide()
	if got := a.config.WindowPositions["1"]; got != (seat{X: 500, Y: 275}) {
		t.Fatalf("saved seat = %+v", got)
	}
	// Showing again does not write: the place came from the file.
	f.visible = false
	a.show()
	if got := a.config.WindowPositions["1"]; got != (seat{X: 500, Y: 275}) {
		t.Fatalf("reopening changed the seat: %+v", got)
	}
	if len(f.moves) != 2 || f.moves[1] != (screenPoint{X: 500, Y: 300}) {
		t.Fatalf("reopening moved to %+v", f.moves)
	}
}

func TestMoveNotificationsDoNotPolluteTheSeat(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	// A late notification for our own placement: the baseline already has it.
	a.observeSettingsMove()
	if a.settingsPlace.pending != nil {
		t.Fatal("a programmatic move became pending")
	}
	// The user drags; the pending seat is cached, never written.
	f.drag(300, 300)
	a.observeSettingsMove()
	if a.settingsPlace.pending == nil {
		t.Fatal("a user move was not cached")
	}
	if len(a.config.WindowPositions) != 0 {
		t.Fatal("a user move was written before the window hid")
	}
	// A second notification for the same origin changes nothing.
	first := *a.settingsPlace.pending
	a.observeSettingsMove()
	if *a.settingsPlace.pending != first {
		t.Fatal("an unchanged origin produced a new seat")
	}
	// Off every display, nothing new is remembered: the seat cached on the
	// last display stays, and the baseline follows so a later return is a real
	// move again.
	f.drag(9000, 9000)
	a.observeSettingsMove()
	if a.settingsPlace.pending == nil || *a.settingsPlace.pending != first {
		t.Fatalf("an off-screen move replaced the pending seat: %+v", a.settingsPlace.pending)
	}
}

func TestDisplaysChangedRepairsOnlyWhatNeedsIt(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	f.drag(300, 300)
	a.observeSettingsMove()
	// Only another display was inserted: the current work area is unchanged,
	// so neither the window nor its pending seat moves.
	original := a.displaysNow()
	inserted := append([]mygo.Display(nil), original...)
	inserted = append(inserted, fakeDisplay(3, "right", 1440, 0, 1280, 800))
	a.displays = func() []mygo.Display { return inserted }
	a.cursorPoint = func() mygo.Point { return mygo.Point{X: 2000, Y: 400} }
	a.displaysChanged()
	if len(f.moves) != 1 || a.settingsPlace.pending == nil {
		t.Fatalf("inserting a display moved the window or dropped the seat: %+v", f.moves)
	}
	// Repeating the same snapshot keeps the seat as well.
	a.displaysChanged()
	if a.settingsPlace.pending == nil {
		t.Fatal("a repeated notification dropped a normal pending seat")
	}
}

func TestDisplaysChangedKeepsTheSeatOnTheSameDisplay(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	f.drag(300, 300)
	a.observeSettingsMove()
	// The main display keeps its number and origin but loses height, as when
	// the Dock appears: the seat maps from the old work area, then clamps.
	resized := []mygo.Display{
		{ID: 1, Label: "main", Primary: true, Bounds: mygo.Rectangle{X: 0, Y: 0, Width: 1440, Height: 700}, WorkArea: mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 700}},
		fakeDisplay(2, "left", -1920, 0, 1920, 1080),
	}
	a.displays = func() []mygo.Display { return resized }
	a.cursorPoint = func() mygo.Point { return mygo.Point{X: -400, Y: 300} }
	a.displaysChanged()
	if len(f.moves) != 2 || f.moves[1] != (screenPoint{X: 300, Y: 205}) {
		t.Fatalf("moves = %+v; want the same seat, clamped into the new work area", f.moves)
	}
	if len(a.config.WindowPositions) != 0 {
		t.Fatal("an automatic repair wrote the configuration")
	}
	if a.settingsPlace.pending != nil {
		t.Fatal("an automatic repair kept a pending seat")
	}
}

func TestDisplaysChangedUsesTheCachedPlaceNotTheSystemRestore(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	f.drag(300, 300)
	a.observeSettingsMove()
	// AppKit moves the window to another display before the notification
	// arrives. The cached place decides, not the current frame.
	f.drag(-1920, 0)
	a.displays = func() []mygo.Display {
		return []mygo.Display{fakeDisplay(2, "left", -1920, 0, 1920, 1080)}
	}
	a.cursorPoint = func() mygo.Point { return mygo.Point{X: -400, Y: 300} }
	a.displaysChanged()
	// The old display is gone, so the window reopens on the pointer's display
	// at its default place: centred, both ways.
	want := screenPoint{X: -1920 + (1920-900)/2, Y: (1080 - 520) / 2}
	if len(f.moves) != 2 || f.moves[1] != want {
		t.Fatalf("moves = %+v; want %+v", f.moves, want)
	}
}

func TestDisplaysChangedDefersAMinimizedWindow(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	f.minimized = true
	f.drag(300, 300)
	a.observeSettingsMove()
	a.displays = func() []mygo.Display {
		return []mygo.Display{fakeDisplay(2, "left", -1920, 0, 1920, 1080)}
	}
	a.displaysChanged()
	if len(f.moves) != 1 {
		t.Fatalf("a minimized window was moved: %+v", f.moves)
	}
	if !a.settingsPlace.needsRepair || a.settingsPlace.pending != nil {
		t.Fatalf("minimized window: needsRepair=%v pending=%v", a.settingsPlace.needsRepair, a.settingsPlace.pending)
	}
	// Coming back from the Dock repairs the now unreachable place, still
	// without following the pointer.
	a.cursorPoint = func() mygo.Point { return mygo.Point{X: 200, Y: 400} }
	a.show()
	if len(f.moves) != 2 || f.restores != 1 {
		t.Fatalf("restore moves = %+v restores = %d", f.moves, f.restores)
	}
	if a.settingsPlace.needsRepair {
		t.Fatal("the repair flag survived the restore")
	}
}

func TestDisplaysChangedIgnoresAnEmptySnapshot(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	f.drag(300, 300)
	a.observeSettingsMove()
	a.displays = func() []mygo.Display { return nil }
	a.displaysChanged()
	if len(f.moves) != 1 || a.settingsPlace.pending == nil {
		t.Fatalf("an empty snapshot moved the window or dropped the seat: %+v", f.moves)
	}
}

// addJSONFrame adds a shown tool window with a fake frame, sized like the real
// one: a window is visible right after it opens.
func addJSONFrame(a *application, width, height int) (*jsonDocument, *fakeFrame) {
	d := a.addJSONDocument("", "", true)
	f := &fakeFrame{bounds: mygo.Rectangle{Width: width, Height: height}, visible: true}
	d.frameForTest = f
	return d, f
}

func TestJSONWindowsStaggerPerDisplay(t *testing.T) {
	a, _ := placementTestApp(t)
	a.config.JSONPositions = map[string]seat{"1": {X: 40, Y: 40}}
	var documents []*jsonDocument
	for index := 0; index < jsonStaggerSlots; index++ {
		d, _ := addJSONFrame(a, 700, 460)
		a.placeJSONWindow(d)
		documents = append(documents, d)
	}
	for index, want := range []screenPoint{{X: 40, Y: 65}, {X: 64, Y: 89}, {X: 88, Y: 113}, {X: 112, Y: 137}, {X: 136, Y: 161}} {
		if got := documents[index].frameForTest.(*fakeFrame).moves[0]; got != want {
			t.Fatalf("window %d opened at %+v; want %+v", index, got, want)
		}
	}
	// A sixth window has no free candidate: it takes the least covered one,
	// which is the first slot here because every slot is used once.
	sixth, sixthFrame := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(sixth)
	if got := sixthFrame.moves[0]; got != (screenPoint{X: 40, Y: 65}) {
		t.Fatalf("sixth window opened at %+v", got)
	}
	if len(a.config.JSONPositions) != 1 || a.config.JSONPositions["1"] != (seat{X: 40, Y: 40}) {
		t.Fatalf("staggering wrote a seat: %+v", a.config.JSONPositions)
	}
	// Closing a window releases its candidate.
	a.removeJSONDocument(documents[0])
	seventh, seventhFrame := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(seventh)
	if got := seventhFrame.moves[0]; got != (screenPoint{X: 40, Y: 65}) {
		t.Fatalf("released slot was not reused: %+v", got)
	}
	_ = sixth
}

func TestJSONStaggerFollowsTheWindowFrames(t *testing.T) {
	a, _ := placementTestApp(t)
	a.config.JSONPositions = map[string]seat{"1": {X: 40, Y: 40}}
	// A window dragged to the other display stops occupying this one.
	placed, placedFrame := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(placed)
	placedFrame.drag(-1800, 100)
	a.observeJSONMove(placed)
	d, f := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(d)
	if got := f.moves[0]; got != (screenPoint{X: 40, Y: 65}) {
		t.Fatalf("dragged-away window still occupied a slot: %+v", got)
	}
	// A minimized window keeps its slot: it is still open.
	f.minimized = true
	d2, f2 := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(d2)
	if got := f2.moves[0]; got != (screenPoint{X: 64, Y: 89}) {
		t.Fatalf("minimized window released its slot: %+v", got)
	}
}

func TestJSONStaggerClampsAndDeduplicates(t *testing.T) {
	a, _ := placementTestApp(t)
	// A work area barely bigger than the window: every candidate clamps to
	// the same place, so no window pretends to be offset.
	small := []mygo.Display{{ID: 1, Label: "main", Primary: true,
		Bounds: mygo.Rectangle{X: 0, Y: 0, Width: 720, Height: 500}, WorkArea: mygo.Rectangle{X: 0, Y: 0, Width: 720, Height: 500}}}
	a.displays = func() []mygo.Display { return small }
	a.config.JSONPositions = map[string]seat{"1": {X: 700, Y: 400}}
	var frames []*fakeFrame
	for index := 0; index < 3; index++ {
		d, f := addJSONFrame(a, 700, 460)
		a.placeJSONWindow(d)
		frames = append(frames, f)
	}
	for index, f := range frames {
		if got := f.moves[0]; got != (screenPoint{X: 20, Y: 40}) {
			t.Fatalf("window %d opened at %+v; want the single clamped place", index, got)
		}
	}
	// A window larger than the work area keeps the default axis: the top edge
	// stays reachable on the vertical axis, and X is centred by the seat rule.
	wide := []mygo.Display{{ID: 1, Label: "main", Primary: true,
		Bounds: mygo.Rectangle{X: 0, Y: 0, Width: 600, Height: 400}, WorkArea: mygo.Rectangle{X: 0, Y: 0, Width: 600, Height: 400}}}
	a.displays = func() []mygo.Display { return wide }
	d, f := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(d)
	if got := f.moves[0]; got != (screenPoint{X: -50, Y: 0}) {
		t.Fatalf("oversized window opened at %+v", got)
	}
}

func TestJSONPlaceIsSavedOnCloseAndDestroy(t *testing.T) {
	a, _ := placementTestApp(t)
	d, f := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(d)
	f.drag(100, 200)
	a.observeJSONMove(d)
	if len(a.config.JSONPositions) != 0 {
		t.Fatal("a move was written before the window closed")
	}
	if !a.rememberJSONPlace(d) {
		t.Fatal("closing the window did not report a change")
	}
	if got := a.config.JSONPositions["1"]; got != (seat{X: 100, Y: 175}) {
		t.Fatalf("saved seat = %+v", got)
	}
	// A window that moved later wins, and a window that moved earlier cannot
	// overwrite the newer seat when it closes last.
	other, otherFrame := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(other)
	otherFrame.drag(300, 400)
	a.observeJSONMove(other)
	if !a.rememberJSONPlace(other) {
		t.Fatal("the newer window did not report a change")
	}
	if got := a.config.JSONPositions["1"]; got != (seat{X: 300, Y: 375}) {
		t.Fatalf("newest seat = %+v", got)
	}
	if a.rememberJSONPlace(d) {
		t.Fatalf("an older pending seat overwrote a newer one: %+v", a.config.JSONPositions["1"])
	}
	if a.config.JSONPositions["1"] != (seat{X: 300, Y: 375}) {
		t.Fatalf("seat changed to %+v", a.config.JSONPositions["1"])
	}
	// Destroy saves before the window is gone: a fresh document that moved.
	d2, f2 := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(d2)
	f2.drag(700, 600)
	a.observeJSONMove(d2)
	a.destroyJSON(d2)
	if got := a.config.JSONPositions["1"]; got != (seat{X: 700, Y: 575}) {
		t.Fatalf("destroy saved %+v", got)
	}
	if len(a.jsonDocuments) != 2 {
		t.Fatalf("destroy left %d documents", len(a.jsonDocuments))
	}
	// A window that never moved writes nothing and leaves no state behind.
	d3, _ := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(d3)
	a.removeJSONDocument(d3)
	if d3.place != (windowPlaceState{}) {
		t.Fatalf("a removed document kept %+v", d3.place)
	}
}

func TestSameSeatAdvancesTheSequence(t *testing.T) {
	a, _ := placementTestApp(t)
	d, f := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(d)
	f.drag(100, 200)
	a.observeJSONMove(d)
	if !a.rememberJSONPlace(d) {
		t.Fatal("the first move did not report a change")
	}
	committed := a.jsonCommitted[1]
	if committed == 0 {
		t.Fatal("no sequence was committed")
	}
	// The same place again: the file does not change, but the sequence moves
	// on, so a later close of another window cannot win with an older one.
	f.drag(400, 400)
	a.observeJSONMove(d)
	f.drag(100, 200)
	a.observeJSONMove(d)
	if a.rememberJSONPlace(d) {
		t.Fatal("storing an unchanged seat reported a change")
	}
	if a.jsonCommitted[1] <= committed {
		t.Fatalf("the sequence stayed at %d", a.jsonCommitted[1])
	}
}

func TestQuitWritesOneSeatPerDisplay(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	f.drag(200, 200)
	a.observeSettingsMove()
	first, firstFrame := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(first)
	firstFrame.drag(10, 20)
	a.observeJSONMove(first)
	second, secondFrame := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(second)
	secondFrame.drag(60, 80)
	a.observeJSONMove(second)
	// A window on the other display keeps its own seat.
	other, otherFrame := addJSONFrame(a, 700, 460)
	otherFrame.bounds = mygo.Rectangle{X: -1920, Y: 0, Width: 700, Height: 460}
	other.place = windowPlaceState{}
	a.placeJSONWindow(other)
	otherFrame.drag(-1800, 100)
	a.observeJSONMove(other)
	a.saveWindowPlacesOnQuit()
	if got := a.config.WindowPositions["1"]; got != (seat{X: 200, Y: 175}) {
		t.Fatalf("settings seat = %+v", got)
	}
	// The last moved window of the display wins, regardless of the order the
	// documents were created in.
	if got := a.config.JSONPositions["1"]; got != (seat{X: 60, Y: 55}) {
		t.Fatalf("json seat = %+v", got)
	}
	if got := a.config.JSONPositions["2"]; got != (seat{X: 120, Y: 75}) {
		t.Fatalf("other display seat = %+v", got)
	}
	// Nothing is left pending after the batch.
	if a.settingsPlace.pending != nil || first.place.pending != nil || second.place.pending != nil {
		t.Fatal("the batch left a pending seat")
	}
}
