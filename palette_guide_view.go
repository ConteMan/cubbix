package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// paletteGuideState is the overlay that draws the three snap lines while the
// panel is dragged. It covers the work area of the display the panel is over,
// takes no pointer and never becomes the key window.
type paletteGuideState struct {
	window  *mygo.Window
	guides  paletteGuides  // screen DIPs
	work    mygo.Rectangle // the work area the window covers
	snap    paletteSnap
	visible bool
}

// buildPaletteGuideWindow creates the guide overlay on the first drag. It is
// built like the hint window in feedback_view.go: a nonactivating panel that
// floats over every Space and lets the pointer through.
func (a *application) buildPaletteGuideWindow(g *paletteGuideState) {
	width, height := g.work.Width, g.work.Height
	if width <= 0 || height <= 0 {
		width, height = int(tokens(false, "zh-CN").SizePaletteWidth), int(tokens(false, "zh-CN").SizePaletteHeight)
	}
	window := mygo.NewWindow(mygo.WindowOptions{
		Title:             "Cubbix",
		Width:             width,
		Height:            height,
		Hidden:            true,
		Frameless:         true,
		Transparent:       true,
		DisableShadow:     true,
		DisableResize:     true,
		DisableMove:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableClose:      true,
		DisableFullScreen: true,
		Panel:             true,
		Content:           ui.View(a.paletteGuideView),
	})
	window.SetIgnoreMouseEvents(true)
	window.SetHasShadow(false)
	window.SetVisibleOnAllWorkspaces(true)
	g.window = window
}

// showPaletteGuides draws the guides of the display under the panel. A drag
// across displays moves and resizes the overlay and redraws the lines; a
// change of the snapped line redraws them in place.
func (a *application) showPaletteGuides(guides paletteGuides, work mygo.Rectangle, snap paletteSnap) {
	if a.guide == nil {
		a.guide = &paletteGuideState{}
	}
	g := a.guide
	first, moved := !g.visible, g.work != work
	changed := first || moved || g.guides != guides || g.snap != snap
	g.guides, g.work, g.snap, g.visible = guides, work, snap, true
	if g.window == nil && a.createPaletteGuide != nil {
		a.createPaletteGuide(g)
	}
	if g.window == nil {
		return
	}
	if first || moved {
		// The overlay covers the work area it draws the guides on: a drag
		// across displays moves and resizes it.
		g.window.SetBounds(mygo.Rectangle{X: work.X, Y: work.Y, Width: work.Width, Height: work.Height})
	}
	if first {
		// The overlay and the panel are both floating windows. Show the overlay
		// first, then bring the panel forward again so the lines stay under it.
		g.window.ShowInactive()
		if a.palette != nil && a.palette.visible && a.palette.window != nil {
			a.palette.window.Show()
		}
	}
	if changed {
		g.window.Invalidate()
	}
}

// hidePaletteGuides hides the overlay. It is hidden with the panel, on the
// release, and when the displays change mid-drag.
func (a *application) hidePaletteGuides() {
	g := a.guide
	if g == nil || !g.visible {
		return
	}
	g.visible = false
	if g.window != nil {
		g.window.Hide()
	}
}

// paletteGuideLine is one line of the overlay, in the overlay window's
// coordinates. The window sits at the work area's origin, so the screen
// positions have it subtracted.
type paletteGuideLine struct {
	Vertical bool
	X, Y     int
	Stroke   int
	Snapped  bool
}

// paletteGuideLines lays out the three lines of the guides in g: the work
// area's vertical centre, the default height, and its vertical middle. The
// line the drag would snap to is drawn thicker and solid.
func (a *application) paletteGuideLines(t tokenValues) []paletteGuideLine {
	g := a.guide
	if g == nil {
		return nil
	}
	local := paletteGuideLocal(g.guides, g.work)
	stroke := func(snapped bool) int {
		if snapped {
			return int(t.StrokeIcon)
		}
		return int(t.StrokeHairline)
	}
	return []paletteGuideLine{
		{Vertical: true, X: local.CenterX, Stroke: stroke(g.snap.X), Snapped: g.snap.X},
		{X: 0, Y: local.TopY, Stroke: stroke(g.snap.Y == paletteSnapTop), Snapped: g.snap.Y == paletteSnapTop},
		{X: 0, Y: local.MiddleY, Stroke: stroke(g.snap.Y == paletteSnapMiddle), Snapped: g.snap.Y == paletteSnapMiddle},
	}
}

// paletteGuideView draws the guide lines the drag is showing.
func (a *application) paletteGuideView(c *ui.Context) {
	_, t := applyTheme(c)
	c.Root().Background(ui.Transparent)
	for _, line := range a.paletteGuideLines(t) {
		color := t.ColorGuideLine
		if line.Snapped {
			color = t.ColorGuideLineSnap
		}
		stroke := float32(line.Stroke)
		box := ui.Box(c).Absolute()
		if line.Vertical {
			box = box.Left(float32(line.X)).Top(0).Width(stroke).FillHeight().BorderWidth(0, 0, 0, stroke)
		} else {
			box = box.Left(0).Top(float32(line.Y)).Height(stroke).FillWidth().BorderWidth(stroke, 0, 0, 0)
		}
		box.BorderColor(ui.Hex(color))
		if !line.Snapped {
			box.BorderStyle(ui.BorderDashed)
		}
	}
}
