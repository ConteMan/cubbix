package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// paletteDragDeadZone is how far the pointer moves before a press becomes a
// drag, in DIPs. Without it a click would flash the guides.
const paletteDragDeadZone = 3

// paletteDrag is one manual drag of the palette. Pressing records the
// pointer's screen position and the panel's, and every move recomposes the
// pointer's screen position to measure how far it went. All positions are
// screen DIPs, in the space whose origin is the main display's top-left.
type paletteDrag struct {
	down         bool // a drag handler took the press
	active       bool // the pointer went past the dead zone
	pressScreen  palettePoint
	panel        palettePoint // the panel's position at the press
	displayID    int64        // the display the panel is on now, recomputed per move
	snap         paletteSnap  // what this frame snapped to
	guides       bool         // the guide overlay is showing
	outsideClick bool         // a double click's press: its release closes nothing
}

// paletteCursorPoint is where the pointer is, as the palette sees it: the
// display under it is the one the panel opens on.
func (a *application) paletteCursorPoint() mygo.Point {
	if a.paletteCursor == nil {
		return mygo.Point{}
	}
	return a.paletteCursor()
}

// paletteSize is the panel's size in DIPs, from the tokens: the window is
// bigger by the shadow margin.
func (a *application) paletteSize() paletteSize {
	t := tokens(false, "zh-CN")
	return paletteSize{Width: int(t.SizePaletteWidth), Height: int(t.SizePaletteHeight)}
}

// paletteMargin is the transparent room the window keeps around the panel for
// its drawn shadow.
func (a *application) paletteMargin() int {
	return int(tokens(false, "zh-CN").SizePaletteShadowMargin)
}

// paletteTopOffset is where the default position puts the panel's top edge.
func (a *application) paletteTopOffset() int {
	return int(tokens(false, "zh-CN").SizePaletteTopOffset)
}

// palettePanelPoint is where the panel's top-left corner sits on screen.
func (a *application) palettePanelPoint() palettePoint {
	window := a.paletteWindowPosition()
	margin := a.paletteMargin()
	return palettePoint{X: window.X + margin, Y: window.Y + margin}
}

// movePalettePanelTo puts the panel's top-left corner at p.
func (a *application) movePalettePanelTo(p palettePoint) {
	margin := a.paletteMargin()
	a.setPaletteWindowPosition(palettePoint{X: p.X - margin, Y: p.Y - margin})
}

// paletteDisplays returns the screens the palette places itself on.
func (a *application) paletteDisplaysNow() []mygo.Display {
	if a.paletteDisplays == nil {
		return nil
	}
	return a.paletteDisplays()
}

// paletteDefaultPoint is where the panel opens on a display when that display
// remembers no position: centred across the work area, near its top, and
// pulled up when the work area is too short.
func (a *application) paletteDefaultPoint(display mygo.Display) palettePoint {
	size := a.paletteSize()
	x, y := palettePosition(display, size.Width, size.Height, float32(a.paletteTopOffset()))
	return palettePoint{X: x, Y: y}
}

// paletteSeatKeyFor is the config key of a display.
func paletteSeatKeyFor(display mygo.Display) string { return paletteSeatKey(display.ID) }

// storePaletteSeat remembers the panel's place on a display.
func (a *application) storePaletteSeat(display mygo.Display, panel palettePoint) {
	a.config.putPaletteSeat(paletteSeatKeyFor(display), paletteSeatFor(panel, display.WorkArea))
	a.save()
}

// resetPalettePosition forgets the panel's place on the display it is on and
// puts it back where it opens by default.
func (a *application) resetPalettePosition() {
	displays := a.paletteDisplaysNow()
	display := paletteDisplayMatching(displays, paletteRect(a.palettePanelPoint(), a.paletteSize()))
	delete(a.config.PalettePositions, paletteSeatKeyFor(display))
	a.save()
	a.movePalettePanelTo(paletteClamp(a.paletteDefaultPoint(display), a.paletteSize(), display.WorkArea))
}

// paletteContentHit reports whether a press landed on the search row or the
// list. Those keep their presses: selecting text and picking a row. base is
// the box of the element the event came to, in window DIPs.
func (a *application) paletteContentHit(ev ui.InputEvent, base ui.Rect) bool {
	p := a.palette
	if p == nil {
		return false
	}
	x, y := base.X+ev.X, base.Y+ev.Y
	return p.searchBounds.Contains(x, y) || p.listBounds.Contains(x, y)
}

// paletteDragHandler is the input handler of a drag area. Once it takes a
// press, MyGo sends every move and the release to it wherever the pointer
// goes (ui/handler.go). base is the area's box in the window, from the last
// build: an Element handle cannot be kept across frames. inside decides
// whether a press belongs to the area; nil means every press does.
func (a *application) paletteDragHandler(base *ui.Rect, inside func(ui.InputEvent) bool) func(ui.InputEvent) bool {
	return func(ev ui.InputEvent) bool {
		p := a.palette
		if p == nil {
			return false
		}
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 || inside != nil && !inside(ev) {
				return false
			}
			a.beginPaletteDrag(*base, ev)
			return true
		case ui.InputPointerMove:
			if !p.drag.down {
				return false
			}
			a.movePaletteDrag(*base, ev)
			return true
		case ui.InputPointerUp:
			if !p.drag.down {
				return false
			}
			a.endPaletteDrag()
			return true
		}
		return false
	}
}

// beginPaletteDrag records the press, or restores the default position on a
// double click.
func (a *application) beginPaletteDrag(base ui.Rect, ev ui.InputEvent) {
	p := a.palette
	panel := a.palettePanelPoint()
	if ev.Clicks >= 2 {
		// The release that follows closes nothing: the panel moved, not closed.
		p.drag = paletteDrag{down: true, outsideClick: true, panel: panel}
		a.resetPalettePosition()
		return
	}
	p.drag = paletteDrag{
		down:        true,
		pressScreen: palettePointerScreen(base, a.paletteWindowPosition(), ev),
		panel:       panel,
		displayID:   paletteDisplayMatching(a.paletteDisplaysNow(), paletteRect(panel, a.paletteSize())).ID,
	}
}

// movePaletteDrag moves the panel with the pointer, snapping to the guides of
// the display it is over this frame.
func (a *application) movePaletteDrag(base ui.Rect, ev ui.InputEvent) {
	p := a.palette
	drag := &p.drag
	size := a.paletteSize()
	screen := palettePointerScreen(base, a.paletteWindowPosition(), ev)
	dx, dy := screen.X-drag.pressScreen.X, screen.Y-drag.pressScreen.Y
	if !drag.active {
		if absInt(dx) < paletteDragDeadZone && absInt(dy) < paletteDragDeadZone {
			return
		}
		drag.active = true
		drag.guides = true
	}
	// Always the press's position plus the whole offset: snapping moves the
	// panel, and a per-frame delta would then be measured from the snapped
	// place and drift away from the pointer.
	candidate := palettePoint{X: drag.panel.X + dx, Y: drag.panel.Y + dy}
	displays := a.paletteDisplaysNow()
	display := paletteDisplayMatching(displays, paletteRect(candidate, size))
	drag.displayID = display.ID
	guides := paletteGuidesFor(display.WorkArea, a.paletteTopOffset())
	if ev.Mods&ui.Alt == 0 {
		candidate, drag.snap = snapPalette(candidate, size, guides, int(tokens(false, "zh-CN").SizePaletteSnap))
	} else {
		// Holding ⌥ places the panel exactly where the pointer is; the guides
		// stay so the user can still line it up.
		drag.snap = paletteSnap{}
	}
	candidate = paletteClamp(candidate, size, display.WorkArea)
	a.movePalettePanelTo(candidate)
	a.showPaletteGuides(guides, display.WorkArea, drag.snap)
}

// endPaletteDrag stores the panel's display and position, or closes the panel
// when the press landed on the shadow margin and never moved.
func (a *application) endPaletteDrag() {
	p := a.palette
	drag := p.drag
	p.drag = paletteDrag{}
	if drag.guides {
		a.hidePaletteGuides()
	}
	if drag.active {
		panel := a.palettePanelPoint()
		display := paletteDisplayMatching(a.paletteDisplaysNow(), paletteRect(panel, a.paletteSize()))
		a.storePaletteSeat(display, panel)
		return
	}
	if drag.outsideClick {
		return
	}
	// The margin belongs to the window and takes the click; a press there that
	// never moved reads as a click outside the panel.
	if !paletteRectContains(paletteRect(drag.panel, a.paletteSize()), drag.pressScreen) {
		a.hidePalette()
	}
}
