package main

import (
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

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
func snapPalette(candidate screenPoint, size screenSize, guides paletteGuides, threshold int) (screenPoint, paletteSnap) {
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
func palettePointerScreen(box ui.Rect, window screenPoint, ev ui.InputEvent) screenPoint {
	return screenPoint{
		X: window.X + int(math.Round(float64(box.X+ev.X))),
		Y: window.Y + int(math.Round(float64(box.Y+ev.Y))),
	}
}
