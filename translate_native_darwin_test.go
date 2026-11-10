//go:build darwin

package main

import (
	"context"
	"fmt"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

func nativeTranslateWindowChecks(a *application, nsapp objc.ID) error {
	state := a.ensureTranslateState()
	a.buildTranslateWindow(state)
	window := state.window
	defer window.Destroy()
	reference := mygo.NewWindow(a.jsonWindowOptions(&jsonDocument{}, tokens(false, "zh-CN"), "zh-CN"))
	defer reference.Destroy()
	native := objc.ID(window.NativeHandle())
	jsonNative := objc.ID(reference.NativeHandle())
	mask := objc.Send[uint](native, objc.RegisterName("styleMask"))
	jsonMask := objc.Send[uint](jsonNative, objc.RegisterName("styleMask"))
	if window.IsPanel() || mask != jsonMask || mask&8 == 0 || !window.IsClosable() || !window.IsMovable() || !window.IsMinimizable() || !window.IsMaximizable() {
		return fmt.Errorf("translation tool flags differ from JSON: style=%x, JSON=%x", mask, jsonMask)
	}
	if objc.Send[uint](native, objc.RegisterName("collectionBehavior"))&(1<<7) != 0 {
		return fmt.Errorf("translation tool allows full screen")
	}
	for button := 0; button < 3; button++ {
		control := native.Send(objc.RegisterName("standardWindowButton:"), button)
		jsonControl := jsonNative.Send(objc.RegisterName("standardWindowButton:"), button)
		if control == 0 || objc.Send[bool](control, objc.RegisterName("isHidden")) {
			return fmt.Errorf("translation system button %d missing", button)
		}
		position := objc.Send[struct{ X, Y float64 }](control, objc.RegisterName("frameOrigin"))
		jsonPosition := objc.Send[struct{ X, Y float64 }](jsonControl, objc.RegisterName("frameOrigin"))
		if position != jsonPosition {
			return fmt.Errorf("translation system button %d position differs: %+v, JSON=%+v", button, position, jsonPosition)
		}
	}
	minimum := objc.Send[struct{ Width, Height float64 }](native, objc.RegisterName("minSize"))
	if minimum.Width != 480 || minimum.Height != 260 {
		return fmt.Errorf("translation minimum=%+v", minimum)
	}
	a.setTranslatePinned(true)
	if !window.IsAlwaysOnTop() || !state.pinned {
		return fmt.Errorf("translation pin did not set floating level")
	}
	a.setTranslatePinned(false)
	if window.IsAlwaysOnTop() || state.pinned {
		return fmt.Errorf("translation unpin did not restore normal level")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state.cancel = cancel
	window.Close()
	if ctx.Err() == nil || a.translate != nil || !window.IsDestroyed() {
		return fmt.Errorf("native close retained translation work or window")
	}
	if window.IsVisible() || window.IsFocused() || reference.IsVisible() || reference.IsFocused() || objc.Send[bool](nsapp, objc.RegisterName("isActive")) {
		return fmt.Errorf("translation native checks activated a window")
	}
	fmt.Println("PASS: hidden translation tool matches JSON native chrome, minimum, pin and close cancellation; no activation")
	return nil
}
