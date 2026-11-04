package main

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// paletteDragApp is a test app with fake screens and a fake window position,
// showing the palette at its default place on the main display. Nothing here
// opens a window.
func paletteDragApp(t *testing.T) (*application, *fakeScreens) {
	t.Helper()
	a := paletteTestApp(t)
	a.createPaletteWindow = func(*paletteState) {}
	f := paletteTestScreens(a)
	a.placePalette()
	a.palette.visible = true
	return a, f
}

// paletteWindowSize is the panel's window: the panel plus the shadow margin
// on every side, which the drawn shadow needs.
func paletteWindowSize() (int, int) {
	t := tokens(false, "zh-CN")
	margin := int(t.SizePaletteShadowMargin)
	return int(t.SizePaletteWidth) + 2*margin, int(t.SizePaletteHeight) + 2*margin
}

// paletteWindowWidth and paletteWindowHeight are for the calls that take the
// size as plain arguments, where a two-value call cannot be spread.
func paletteWindowWidth() int {
	w, _ := paletteWindowSize()
	return w
}

func paletteWindowHeight() int {
	_, h := paletteWindowSize()
	return h
}

// paletteTester builds the panel view at the window size, twice: the drag
// areas decide from the previous frame's boxes, which the first frame has not
// laid out yet.
func paletteTester(a *application, locale string) *ui.Tester {
	tt := ui.NewTester(a.paletteView, paletteWindowWidth(), paletteWindowHeight())
	tt.SetLocale(locale)
	tt.Frame()
	tt.Frame()
	return tt
}

// palettePress, paletteMove and paletteRelease report the pointer at a screen
// position. The platform gives a pointer relative to the window, and a drag
// moves that window under the pointer, so the same screen position is a
// different window position every frame.
func palettePress(tt *ui.Tester, f *fakeScreens, at palettePoint) {
	tt.Press(float32(at.X-f.window.X), float32(at.Y-f.window.Y))
}

func paletteMove(tt *ui.Tester, f *fakeScreens, at palettePoint) {
	tt.Move(float32(at.X-f.window.X), float32(at.Y-f.window.Y))
}

func paletteRelease(tt *ui.Tester, f *fakeScreens, at palettePoint) {
	tt.Release(float32(at.X-f.window.X), float32(at.Y-f.window.Y))
}

// panelPointOf is where the panel's top-left corner is, from the fake window.
func panelPointOf(a *application, f *fakeScreens) palettePoint {
	margin := a.paletteMargin()
	return palettePoint{X: f.window.X + margin, Y: f.window.Y + margin}
}

// grabScreen is the centre of the search row's icon, the drag handle.
func grabScreen(a *application, f *fakeScreens) palettePoint {
	t := tokens(false, "zh-CN")
	panel := panelPointOf(a, f)
	return palettePoint{
		X: panel.X + int(2*t.Space8+t.SizePaletteSearchHeight/2),
		Y: panel.Y + int(t.Space8+t.SizePaletteSearchHeight/2),
	}
}

func TestPaletteDragMovesAndRemembersThePanel(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	grab := grabScreen(a, f)
	palettePress(tt, f, grab)
	paletteMove(tt, f, palettePoint{X: grab.X + 40, Y: grab.Y + 30})
	if !a.palette.drag.active {
		t.Fatal("a move past the dead zone did not start a drag")
	}
	paletteRelease(tt, f, palettePoint{X: grab.X + 40, Y: grab.Y + 30})
	if got := panelPointOf(a, f); got != (palettePoint{X: 440, Y: 151}) {
		t.Fatalf("panel = %+v; want the press position plus the offset", got)
	}
	if seat, ok := a.config.PalettePositions["1"]; !ok || seat != (paletteSeat{X: 440, Y: 126}) {
		t.Fatalf("stored seat = %+v, %v", seat, ok)
	}
	if a.guide != nil && a.guide.visible {
		t.Fatal("the guides stayed after the release")
	}
}

func TestPaletteDragSnapsToTheCentreLine(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	grab := grabScreen(a, f)
	palettePress(tt, f, grab)
	paletteMove(tt, f, palettePoint{X: grab.X + 6, Y: grab.Y})
	if !a.palette.drag.snap.X {
		t.Fatal("six DIPs from the centre line did not snap")
	}
	if got := panelPointOf(a, f); got.X != 400 {
		t.Fatalf("snapped panel x = %d; want the centre line at 400", got.X)
	}
	paletteRelease(tt, f, palettePoint{X: grab.X + 6, Y: grab.Y})
}

func TestPaletteDragLeavesThePanelFreeBeyondTheThreshold(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	grab := grabScreen(a, f)
	palettePress(tt, f, grab)
	paletteMove(tt, f, palettePoint{X: grab.X + 20, Y: grab.Y})
	if a.palette.drag.snap.X {
		t.Fatal("twenty DIPs from the centre line still snapped")
	}
	if got := panelPointOf(a, f); got.X != 420 {
		t.Fatalf("panel x = %d; want the pointer's 420", got.X)
	}
	paletteRelease(tt, f, palettePoint{X: grab.X + 20, Y: grab.Y})
}

// TestPaletteDragHoldingOptionDoesNotSnap drives the handler directly: the
// Tester carries no modifiers on a move, and ⌥ is read from the move's own
// event.
func TestPaletteDragHoldingOptionDoesNotSnap(t *testing.T) {
	a, f := paletteDragApp(t)
	base := ui.Rect{}
	handle := a.paletteDragHandler(&base, nil)
	// A pointer at a screen position, reported relative to the window that
	// moves under it.
	send := func(kind ui.InputKind, at palettePoint, mods ui.Modifiers) bool {
		return handle(ui.InputEvent{Kind: kind, Button: 0, Clicks: 1, Mods: mods,
			X: float32(at.X - f.window.X), Y: float32(at.Y - f.window.Y)})
	}
	press := palettePoint{X: f.window.X + 100, Y: f.window.Y + 100}
	if !send(ui.InputPointerDown, press, 0) {
		t.Fatal("the drag area refused the press")
	}
	send(ui.InputPointerMove, palettePoint{X: press.X + 6, Y: press.Y}, ui.Alt)
	if a.palette.drag.snap.X {
		t.Fatal("⌥ still snapped")
	}
	if got := panelPointOf(a, f); got.X != 406 {
		t.Fatalf("panel x = %d; want the pointer's 406", got.X)
	}
	// Letting ⌥ go snaps the same pointer position onto the centre line.
	send(ui.InputPointerMove, palettePoint{X: press.X + 6, Y: press.Y}, 0)
	if !a.palette.drag.snap.X || panelPointOf(a, f).X != 400 {
		t.Fatalf("letting ⌥ go did not snap: snap=%v x=%d", a.palette.drag.snap.X, panelPointOf(a, f).X)
	}
	send(ui.InputPointerUp, palettePoint{X: press.X + 6, Y: press.Y}, 0)
}

func TestPalettePressInsideTheDeadZoneIsAClick(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	before := panelPointOf(a, f)
	grab := grabScreen(a, f)
	palettePress(tt, f, grab)
	paletteMove(tt, f, palettePoint{X: grab.X + 2, Y: grab.Y + 1})
	paletteRelease(tt, f, palettePoint{X: grab.X + 2, Y: grab.Y + 1})
	if a.palette.drag.active {
		t.Fatal("two DIPs started a drag")
	}
	if got := panelPointOf(a, f); got != before {
		t.Fatalf("panel moved from %+v to %+v", before, got)
	}
	if len(a.config.PalettePositions) != 0 {
		t.Fatalf("a click stored a position: %+v", a.config.PalettePositions)
	}
	if a.guide != nil && a.guide.visible {
		t.Fatal("a click flashed the guides")
	}
	if !a.palette.visible {
		t.Fatal("a click inside the panel closed it")
	}
}

// TestPaletteTextSelectionKeepsThePanelAndTheFocus is why a drag area has to
// answer false: the text field has no handler of its own, so a handler that
// took every press would swallow the selection and drop the focus.
func TestPaletteTextSelectionKeepsThePanelAndTheFocus(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	tt.Type("select these words")
	field, ok := tt.Find(a.text("zh-CN", "PaletteSearch"))
	if !ok {
		t.Fatalf("no search field in %v", tt.Texts())
	}
	before := panelPointOf(a, f)
	start := palettePoint{X: f.window.X + int(field.X+2), Y: f.window.Y + int(field.Y+field.H/2)}
	end := palettePoint{X: start.X + 55, Y: start.Y}
	palettePress(tt, f, start)
	paletteMove(tt, f, end)
	paletteRelease(tt, f, end)
	if a.palette.drag.down {
		t.Fatal("a press in the text field started a drag")
	}
	if got := panelPointOf(a, f); got != before {
		t.Fatalf("panel moved from %+v to %+v", before, got)
	}
	if active, _ := tt.TextInputActive(); !active {
		t.Fatal("the text field lost the keyboard: KeepFocus is missing")
	}
	tt.Command("copy")
	selected := tt.Clipboard()
	if selected == "" || selected == a.palette.query {
		t.Fatalf("pointer drag did not select part of the query: %q", selected)
	}
	tt.Type("X")
	if a.palette.query == "select these wordsX" || len(a.palette.query) != len("select these words")-len(selected)+1 {
		t.Fatalf("typing did not replace the pointer selection: %q", a.palette.query)
	}
}

func TestPaletteDoubleClickMoveDoesNotStartDrag(t *testing.T) {
	a, f := paletteDragApp(t)
	a.config.putPaletteSeat("1", paletteSeat{X: 100, Y: 200})
	a.config.putPaletteSeat("2", paletteSeat{X: 80, Y: 90})
	f.window = palettePoint{X: 100, Y: 200}
	base := ui.Rect{X: 32, Y: 32}
	handle := a.paletteDragHandler(&base, nil)
	handle(ui.InputEvent{Kind: ui.InputPointerDown, Clicks: 2, X: 25, Y: 25})
	reset := f.window
	handle(ui.InputEvent{Kind: ui.InputPointerMove, X: -100, Y: 300})
	handle(ui.InputEvent{Kind: ui.InputPointerUp})
	if f.window != reset || !a.palette.visible || len(a.config.PalettePositions) != 1 || a.config.PalettePositions["2"] != (paletteSeat{X: 80, Y: 90}) {
		t.Fatalf("double-click move changed the reset: window=%+v seats=%v", f.window, a.config.PalettePositions)
	}
	if a.guide != nil || a.palette.drag.down {
		t.Fatal("double-click move started a drag or displayed guides")
	}
}

func TestPaletteDragDeadZoneBoundaryAndSecondaryRelease(t *testing.T) {
	a, f := paletteDragApp(t)
	base := ui.Rect{}
	handle := a.paletteDragHandler(&base, nil)
	handle(ui.InputEvent{Kind: ui.InputPointerDown, X: 100, Y: 100})
	before := f.window
	handle(ui.InputEvent{Kind: ui.InputPointerMove, X: 103, Y: 103})
	if f.window != before || a.palette.drag.active || a.guide != nil {
		t.Fatal("a movement of exactly three DIPs crossed the dead zone")
	}
	handle(ui.InputEvent{Kind: ui.InputPointerMove, X: 104, Y: 100, Mods: ui.Alt})
	if !a.palette.drag.active || f.window.X != before.X+4 {
		t.Fatal("a movement past three DIPs did not drag")
	}
	if handle(ui.InputEvent{Kind: ui.InputPointerUp, Button: 1}) || !a.palette.drag.down || len(a.config.PalettePositions) != 0 {
		t.Fatal("a secondary release committed the primary drag")
	}
	handle(ui.InputEvent{Kind: ui.InputPointerUp})
	if a.palette.drag.down || len(a.config.PalettePositions) != 1 {
		t.Fatal("the primary release did not commit the drag")
	}
}

func TestPaletteDisplayChangeCancelsDragWithoutMovingOrSaving(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	grab := grabScreen(a, f)
	palettePress(tt, f, grab)
	paletteMove(tt, f, palettePoint{X: grab.X + 60, Y: grab.Y + 60})
	before := f.window
	f.displays = f.displays[1:]
	a.paletteDisplaysChanged()
	if f.window != before || len(a.config.PalettePositions) != 0 || a.palette.drag.down || a.guide.visible {
		t.Fatalf("display change moved or committed a drag: window=%+v drag=%+v", f.window, a.palette.drag)
	}
	paletteRelease(tt, f, palettePoint{X: grab.X + 60, Y: grab.Y + 60})
	if len(a.config.PalettePositions) != 0 {
		t.Fatal("release committed the cancelled drag")
	}
	// Reopening uses the remaining display's normal placement and keeps the
	// old display's remembered seat intact.
	a.config.putPaletteSeat("1", paletteSeat{X: 100, Y: 200})
	a.hidePalette()
	a.showPalette()
	if got := panelPointOf(a, f); got != (palettePoint{X: -1280, Y: 121}) || len(a.config.PalettePositions) != 1 {
		t.Fatalf("reopen after disconnect: panel=%+v seats=%v", got, a.config.PalettePositions)
	}
}

func TestPaletteDragKeepsTextInputAndComposition(t *testing.T) {
	for _, area := range []string{"grab", "padding", "margin"} {
		t.Run(area, func(t *testing.T) {
			a, f := paletteDragApp(t)
			tt := paletteTester(a, "zh-CN")
			tt.Type("prefix ")
			tt.Compose("pinyin", 6)
			caret, composing := tt.TextCaret()
			start := grabScreen(a, f)
			if area == "padding" {
				panel := panelPointOf(a, f)
				start = palettePoint{X: panel.X + 4, Y: panel.Y + 4}
			} else if area == "margin" {
				start = palettePoint{X: f.window.X + 16, Y: f.window.Y + 16}
			}
			palettePress(tt, f, start)
			if active, noIME := tt.TextInputActive(); !active || noIME {
				t.Fatal("press stopped text input or disabled input methods")
			}
			paletteMove(tt, f, palettePoint{X: start.X + 60, Y: start.Y + 40})
			paletteRelease(tt, f, palettePoint{X: start.X + 60, Y: start.Y + 40})
			if after, active := tt.TextCaret(); !composing || !active || after != caret || a.palette.query != "prefix " {
				t.Fatal("drag changed the text input caret or committed composition")
			}
			tt.Type("拼音")
			if a.palette.query != "prefix 拼音" || !a.palette.visible {
				t.Fatalf("typing after drag failed: %q", a.palette.query)
			}
		})
	}
}

func TestPaletteRowStillClicks(t *testing.T) {
	a, f := paletteDragApp(t)
	// Somewhere that is not the default, so the row's command shows in the
	// position it leaves behind.
	a.config.putPaletteSeat("1", paletteSeat{X: 10, Y: 20})
	f.window = palettePoint{X: 700, Y: 400}
	tt := paletteTester(a, "zh-CN")
	// The command sits at the end of a long list: filter it to the top first.
	tt.Type(a.text("zh-CN", "PaletteResetPosition"))
	if err := tt.Click(a.text("zh-CN", "PaletteResetPosition")); err != nil {
		t.Fatal(err)
	}
	if a.palette.drag.down || a.palette.drag.active {
		t.Fatal("clicking a row started a drag")
	}
	if _, ok := a.config.PalettePositions["1"]; ok {
		t.Fatalf("the row's command did not run: %+v", a.config.PalettePositions)
	}
	if got := panelPointOf(a, f); got != (palettePoint{X: 400, Y: 121}) {
		t.Fatalf("the panel did not go back to its default: %+v", got)
	}
}

func TestPaletteClickOnTheMarginClosesIt(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	margin := a.paletteMargin() / 2
	palettePress(tt, f, palettePoint{X: f.window.X + margin, Y: f.window.Y + margin})
	paletteRelease(tt, f, palettePoint{X: f.window.X + margin, Y: f.window.Y + margin})
	if a.palette.visible {
		t.Fatal("a click on the shadow margin kept the panel open")
	}
}

func TestPaletteDragFromTheMarginMovesIt(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	margin := a.paletteMargin() / 2
	before := panelPointOf(a, f)
	start := palettePoint{X: f.window.X + margin, Y: f.window.Y + margin}
	palettePress(tt, f, start)
	paletteMove(tt, f, palettePoint{X: start.X + 30, Y: start.Y + 20})
	paletteRelease(tt, f, palettePoint{X: start.X + 30, Y: start.Y + 20})
	if !a.palette.visible {
		t.Fatal("a drag on the margin closed the panel")
	}
	if got := panelPointOf(a, f); got != (palettePoint{X: before.X + 30, Y: before.Y + 20}) {
		t.Fatalf("panel = %+v; want %+v", got, before)
	}
}

// TestPaletteBlankSpaceDrags covers the panel's own padding, which the panel
// answers for itself rather than the search row or the list.
func TestPaletteBlankSpaceDrags(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	before := panelPointOf(a, f)
	// The panel's padding ring, just inside its top-left corner.
	start := palettePoint{X: before.X + 4, Y: before.Y + 4}
	palettePress(tt, f, start)
	paletteMove(tt, f, palettePoint{X: start.X + 25, Y: start.Y + 35})
	paletteRelease(tt, f, palettePoint{X: start.X + 25, Y: start.Y + 35})
	if got := panelPointOf(a, f); got != (palettePoint{X: before.X + 25, Y: before.Y + 35}) {
		t.Fatalf("panel = %+v; want %+v", got, before)
	}
}

func TestPaletteDoubleClickRestoresTheDefaultPosition(t *testing.T) {
	a, f := paletteDragApp(t)
	a.config.putPaletteSeat("1", paletteSeat{X: 10, Y: 20})
	f.window = palettePoint{X: 500, Y: 400}
	tt := paletteTester(a, "zh-CN")
	grab := grabScreen(a, f)
	// Two presses within the double click interval: the engine counts the
	// second one as Clicks == 2.
	gx, gy := float32(grab.X-f.window.X), float32(grab.Y-f.window.Y)
	tt.ClickAt(gx, gy)
	tt.ClickAt(gx, gy)
	if _, ok := a.config.PalettePositions["1"]; ok {
		t.Fatalf("a double click kept the seat: %+v", a.config.PalettePositions)
	}
	if got := panelPointOf(a, f); got != (palettePoint{X: 400, Y: 121}) {
		t.Fatalf("panel = %+v; want the default position", got)
	}
	if !a.palette.visible {
		t.Fatal("the release of a double click closed the panel")
	}
}

func TestPaletteDragAcrossDisplays(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	grab := grabScreen(a, f)
	palettePress(tt, f, grab)
	// Drag far left: most of the panel ends up on the second display, whose
	// work area then clamps it, not the display the press started on.
	paletteMove(tt, f, palettePoint{X: grab.X - 1200, Y: grab.Y})
	if a.palette.drag.displayID != 2 {
		t.Fatalf("drag display = %d; want the left display", a.palette.drag.displayID)
	}
	if got := panelPointOf(a, f); got.X != -800 {
		t.Fatalf("panel x = %d; want the pointer's -800", got.X)
	}
	if a.guide == nil || a.guide.work.X != -1920 {
		t.Fatalf("guides cover %+v; want the left display", a.guide)
	}
	// Past the left edge of that display it is pulled back inside it, not
	// inside the display the press started on.
	paletteMove(tt, f, palettePoint{X: grab.X - 2500, Y: grab.Y})
	if got := panelPointOf(a, f); got.X != -1920 {
		t.Fatalf("panel x = %d; want the left edge of the left display", got.X)
	}
	paletteRelease(tt, f, palettePoint{X: grab.X - 2500, Y: grab.Y})
	if _, ok := a.config.PalettePositions["2"]; !ok {
		t.Fatalf("the drag stored no seat for the left display: %+v", a.config.PalettePositions)
	}
	if _, ok := a.config.PalettePositions["1"]; ok {
		t.Fatalf("the drag kept a seat for the first display: %+v", a.config.PalettePositions)
	}
}

func TestPaletteGuideLines(t *testing.T) {
	a := paletteTestApp(t)
	work := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	guides := paletteGuidesFor(work, 96)
	tk := tokens(false, "zh-CN")
	a.guide = &paletteGuideState{guides: guides, work: work, snap: paletteSnap{X: true, Y: paletteSnapTop}, visible: true}
	lines := a.paletteGuideLines(tk)
	if len(lines) != 3 {
		t.Fatalf("lines = %+v; want three", lines)
	}
	local := paletteGuideLocal(guides, work)
	want := []paletteGuideLine{
		{Vertical: true, X: local.CenterX, Stroke: int(tk.StrokeIcon), Snapped: true},
		{X: 0, Y: local.TopY, Stroke: int(tk.StrokeIcon), Snapped: true},
		{X: 0, Y: local.MiddleY, Stroke: int(tk.StrokeHairline)},
	}
	for index := range want {
		if lines[index] != want[index] {
			t.Errorf("line %d = %+v; want %+v", index, lines[index], want[index])
		}
	}
	// Only the line the drag snapped to is emphasized, whichever it is.
	a.guide.snap = paletteSnap{Y: paletteSnapMiddle}
	lines = a.paletteGuideLines(tk)
	emphasized := 0
	for _, line := range lines {
		if line.Snapped {
			emphasized++
		}
	}
	if emphasized != 1 || !lines[2].Snapped || lines[0].Stroke != int(tk.StrokeHairline) {
		t.Fatalf("middle snap emphasized %+v", lines)
	}
	// The overlay draws what the layout says: three boxes, and the image is
	// not empty.
	tt := ui.NewTester(a.paletteGuideView, work.Width, work.Height)
	tt.Frame()
	image := tt.Image()
	painted := 0
	for x := 0; x < work.Width; x++ {
		for y := 0; y < work.Height; y++ {
			if image.RGBAAt(x, y).A != 0 {
				painted++
			}
		}
	}
	if painted == 0 {
		t.Fatal("the overlay drew nothing")
	}
}

func TestPaletteGuidesHideWithThePanel(t *testing.T) {
	a, f := paletteDragApp(t)
	tt := paletteTester(a, "zh-CN")
	grab := grabScreen(a, f)
	palettePress(tt, f, grab)
	paletteMove(tt, f, palettePoint{X: grab.X + 40, Y: grab.Y + 30})
	if a.guide == nil || !a.guide.visible {
		t.Fatal("the guides did not show while dragging")
	}
	paletteRelease(tt, f, palettePoint{X: grab.X + 40, Y: grab.Y + 30})
	if a.guide.visible {
		t.Fatal("the guides stayed after the release")
	}
}

type recordingPaletteGuideWindow struct {
	bounds                      []mygo.Rectangle
	shows, hides, invalidations int
}

func (w *recordingPaletteGuideWindow) SetBounds(r mygo.Rectangle) { w.bounds = append(w.bounds, r) }
func (w *recordingPaletteGuideWindow) ShowInactive()              { w.shows++ }
func (w *recordingPaletteGuideWindow) Hide()                      { w.hides++ }
func (w *recordingPaletteGuideWindow) Invalidate()                { w.invalidations++ }

func TestPaletteGuideWindowLifecycleAndRedraw(t *testing.T) {
	a, f := paletteDragApp(t)
	window := &recordingPaletteGuideWindow{}
	created := 0
	a.createPaletteGuide = func(g *paletteGuideState) { created++; g.window = window }
	work := f.displays[0].WorkArea
	guides := paletteGuidesFor(work, a.paletteTopOffset())
	a.showPaletteGuides(guides, work, paletteSnap{})
	a.showPaletteGuides(guides, work, paletteSnap{})
	if created != 1 || window.shows != 1 || len(window.bounds) != 1 || window.invalidations != 1 {
		t.Fatalf("unchanged guides were recreated or redrawn: %+v", window)
	}
	a.showPaletteGuides(guides, work, paletteSnap{X: true})
	if window.invalidations != 2 || window.shows != 1 || len(window.bounds) != 1 {
		t.Fatal("snap change should only invalidate the guide window")
	}
	work = f.displays[1].WorkArea
	a.showPaletteGuides(paletteGuidesFor(work, a.paletteTopOffset()), work, paletteSnap{})
	if window.invalidations != 3 || window.shows != 1 || len(window.bounds) != 2 || window.bounds[1] != work {
		t.Fatal("cross-screen drag did not move/resize and redraw the guides")
	}
	a.hidePalette()
	a.hidePaletteGuides()
	if window.hides != 1 || a.guide.visible {
		t.Fatal("hiding the palette must hide its guides once")
	}
	a.showPaletteGuides(guides, f.displays[0].WorkArea, paletteSnap{})
	if created != 1 || window.shows != 2 || window.invalidations != 4 {
		t.Fatal("next drag should reuse and redraw the inactive guide window")
	}
}
