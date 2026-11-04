package main

import (
	"math"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// palettePoint is a position in screen DIPs: the display space whose origin
// is the main display's top-left corner. The panel's position and the
// pointer's position are both in it.
type palettePoint struct{ X, Y int }

// paletteSize is the panel's size, not the window's: the window adds
// SizePaletteShadowMargin on every side for the drawn shadow.
type paletteSize struct{ Width, Height int }

// paletteSnap says what the candidate hit: the vertical centre line and at
// most one horizontal line.
type paletteSnap struct {
	X bool
	Y int // paletteSnapNone, paletteSnapTop or paletteSnapMiddle
}

const (
	paletteSnapNone = iota
	paletteSnapTop
	paletteSnapMiddle
)

// paletteGuides are one display's three guide lines, in screen DIPs.
type paletteGuides struct {
	CenterX int
	TopY    int
	MiddleY int
}

// paletteGuidesFor returns the guides of a work area. The top line is where
// the default position puts the panel's upper edge.
func paletteGuidesFor(work mygo.Rectangle, topOffset int) paletteGuides {
	return paletteGuides{
		CenterX: work.X + work.Width/2,
		TopY:    work.Y + topOffset,
		MiddleY: work.Y + work.Height/2,
	}
}

// snapPalette pulls a candidate position onto the guides it is within
// threshold of. Horizontal snapping aligns the panel's centre with the
// vertical line; vertical snapping aligns its top edge with the default
// height line, or its centre with the work area's centre. Two horizontal
// lines at the same distance prefer the default height: it is the position
// the panel opens at.
func snapPalette(candidate palettePoint, size paletteSize, guides paletteGuides, threshold int) (palettePoint, paletteSnap) {
	snap := paletteSnap{}
	if absInt(candidate.X+size.Width/2-guides.CenterX) <= threshold {
		candidate.X = guides.CenterX - size.Width/2
		snap.X = true
	}
	top := absInt(candidate.Y - guides.TopY)
	middle := absInt(candidate.Y + size.Height/2 - guides.MiddleY)
	switch {
	case top <= threshold && (middle > threshold || top <= middle):
		candidate.Y = guides.TopY
		snap.Y = paletteSnapTop
	case middle <= threshold:
		candidate.Y = guides.MiddleY - size.Height/2
		snap.Y = paletteSnapMiddle
	}
	return candidate, snap
}

// paletteClamp keeps the panel inside the work area. A work area too small
// for the panel leaves that axis where the caller put it: opening falls back
// to palettePosition's own fit (centred across, against the top), and a drag
// keeps the pointer's choice.
func paletteClamp(p palettePoint, size paletteSize, work mygo.Rectangle) palettePoint {
	if work.Width >= size.Width {
		p.X = clampInt(p.X, work.X, work.X+work.Width-size.Width)
	}
	if work.Height >= size.Height {
		p.Y = clampInt(p.Y, work.Y, work.Y+work.Height-size.Height)
	}
	return p
}

// paletteGuideLocal moves the guides into the overlay window's own
// coordinates: the window sits at the work area's origin, Absolute places
// its lines relative to it, and the guides are in screen DIPs.
func paletteGuideLocal(guides paletteGuides, work mygo.Rectangle) paletteGuides {
	return paletteGuides{
		CenterX: guides.CenterX - work.X,
		TopY:    guides.TopY - work.Y,
		MiddleY: guides.MiddleY - work.Y,
	}
}

// palettePointerScreen rebuilds the pointer's screen position from one event.
// The event carries the pointer relative to the element's box, and the window
// moves under it while dragging, so a screen position has to be composed from
// the window's current placement, the element's box in the window, and the
// event, every frame: subtracting two element-local positions would collapse
// to the residual after the first move.
func palettePointerScreen(box ui.Rect, window palettePoint, ev ui.InputEvent) palettePoint {
	return palettePoint{
		X: window.X + int(math.Round(float64(box.X+ev.X))),
		Y: window.Y + int(math.Round(float64(box.Y+ev.Y))),
	}
}

// paletteRect is the panel's rectangle for a position.
func paletteRect(p palettePoint, size paletteSize) mygo.Rectangle {
	return mygo.Rectangle{X: p.X, Y: p.Y, Width: size.Width, Height: size.Height}
}

// paletteRectContains reports whether p is inside r.
func paletteRectContains(r mygo.Rectangle, p palettePoint) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X < r.X+r.Width && p.Y < r.Y+r.Height
}

// paletteSeatKey is the config key of a display: its runtime display number,
// never a serial number or a name.
func paletteSeatKey(id int64) string { return strconv.FormatInt(id, 10) }

// paletteSeatFor turns a panel position into the offset stored for a display.
func paletteSeatFor(panel palettePoint, work mygo.Rectangle) paletteSeat {
	return paletteSeat{X: panel.X - work.X, Y: panel.Y - work.Y}
}

// palettePanelFromSeat turns a stored offset back into a screen position.
func palettePanelFromSeat(seat paletteSeat, work mygo.Rectangle) palettePoint {
	return palettePoint{X: work.X + seat.X, Y: work.Y + seat.Y}
}

// paletteDisplayMatching returns the display overlapping r the most, the way
// Screen.DisplayMatching does, from a list the caller supplies so tests need
// no app and no real screen.
func paletteDisplayMatching(displays []mygo.Display, r mygo.Rectangle) mygo.Display {
	best, bestArea := mygo.Display{}, -1
	for _, d := range displays {
		w := min(d.Bounds.X+d.Bounds.Width, r.X+r.Width) - max(d.Bounds.X, r.X)
		h := min(d.Bounds.Y+d.Bounds.Height, r.Y+r.Height) - max(d.Bounds.Y, r.Y)
		if area := max(w, 0) * max(h, 0); area > bestArea {
			best, bestArea = d, area
		}
	}
	return best
}

// paletteDisplayNearest returns the display closest to p, as
// Screen.DisplayNearestPoint does.
func paletteDisplayNearest(displays []mygo.Display, p mygo.Point) mygo.Display {
	best, bestDistance := mygo.Display{}, math.Inf(1)
	for _, d := range displays {
		b := d.Bounds
		dx := math.Max(math.Max(float64(b.X-p.X), 0), float64(p.X-(b.X+b.Width)))
		dy := math.Max(math.Max(float64(b.Y-p.Y), 0), float64(p.Y-(b.Y+b.Height)))
		if distance := dx*dx + dy*dy; distance < bestDistance {
			best, bestDistance = d, distance
		}
	}
	return best
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func clampInt(v, low, high int) int {
	return min(max(v, low), high)
}
