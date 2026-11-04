//go:build darwin

package main

import (
	"fmt"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// Called only by the existing hidden AppKit helper, on the main thread.
// These are production windows; neither is ever ordered on screen.
func nativePalettePositionChecks(a *application, nsapp objc.ID) error {
	p := &paletteState{}
	a.palette = p
	a.buildPaletteWindow(p)
	defer func() { p.window.Destroy(); a.palette = nil }()
	work := mygo.Rectangle{X: -1200, Y: 25, Width: 1200, Height: 800}
	g := &paletteGuideState{work: work, guides: paletteGuidesFor(work, a.paletteTopOffset())}
	a.buildPaletteGuideWindow(g)
	guide := g.window.(*mygo.Window)
	defer guide.Destroy()
	const required = uint(1<<0 | 1<<3 | 1<<6 | 1<<8)
	for _, w := range []*mygo.Window{p.window, guide} {
		native := objc.ID(w.NativeHandle())
		style := objc.Send[uint](native, objc.RegisterName("styleMask"))
		behavior := objc.Send[uint](native, objc.RegisterName("collectionBehavior"))
		if !w.IsPanel() || style&(1<<7) == 0 || behavior&required != required || w.HasShadow() || w.IsVisible() || w.IsFocused() {
			return fmt.Errorf("hidden palette/guide flags: style=%x behavior=%x", style, behavior)
		}
		if objc.Send[bool](native, objc.RegisterName("isOpaque")) || objc.Send[bool](native, objc.RegisterName("isMovable")) {
			return fmt.Errorf("hidden palette/guide must be transparent and manually positioned")
		}
	}
	if !objc.Send[bool](objc.ID(guide.NativeHandle()), objc.RegisterName("ignoresMouseEvents")) || objc.Send[bool](objc.ID(p.window.NativeHandle()), objc.RegisterName("ignoresMouseEvents")) {
		return fmt.Errorf("only the guide should ignore mouse events")
	}
	if width, height := p.window.Size(); width != 704 || height != 464 {
		return fmt.Errorf("hidden palette shadow window size = %dx%d", width, height)
	}
	guide.SetBounds(work)
	for _, point := range []palettePoint{{X: 400, Y: 121}, {X: -1000, Y: 160}} {
		a.movePalettePanelTo(point)
		if a.palettePanelPoint() != point || p.window.IsVisible() || guide.IsVisible() || p.window.IsFocused() || guide.IsFocused() || objc.Send[bool](nsapp, objc.RegisterName("isActive")) {
			return fmt.Errorf("hidden positioning changed focus, visibility or geometry")
		}
	}
	width, height := settingsMinimumSize()
	if width != 720 || height != 368 {
		return fmt.Errorf("settings minimum = %dx%d; want 720x368", width, height)
	}
	fmt.Println("PASS: hidden production command/guide panels: nonactivating, transparent, all-Spaces/fullscreen auxiliary, no system shadow; guide ignores mouse; 704x464 shadow margin and screen-DIP moves; settings minimum 720x368; never shown or focused")
	return nil
}
