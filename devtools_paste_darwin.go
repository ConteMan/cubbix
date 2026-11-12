//go:build darwin

package main

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
)

const (
	coreGraphicsLibrary = "/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics"

	// keyV is the ANSI V virtual key code. The virtual code is what the event
	// carries; a non-ANSI layout prints another legend but still answers the
	// same code, and the maintainer verifies that by hand.
	keyV uint16 = 0x09
	// eventFlagCommand is kCGEventFlagMaskCommand, set on the event itself
	// instead of pressing a modifier and having to release it.
	eventFlagCommand uint64 = 1 << 20
	// eventTapHID posts where a real keyboard posts.
	eventTapHID uint32 = 0
)

// cgSymbols is the whole C surface used by the paste: CoreGraphics only. The
// permission landing is a preflight, never a request.
type cgSymbols struct {
	preflight func() bool
	createKey func(uintptr, uint16, bool) uintptr
	setFlags  func(uintptr, uint64)
	post      func(uint32, uintptr)
	release   func(uintptr)
}

var (
	cgOnce    sync.Once
	cgHandles cgSymbols
	cgLoadErr error
)

func loadCGSymbols() {
	cg, err := purego.Dlopen(coreGraphicsLibrary, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		cgLoadErr = err
		return
	}
	cf, err := purego.Dlopen(coreFoundationLibrary, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		cgLoadErr = err
		return
	}
	for _, symbol := range []struct {
		handle uintptr
		name   string
	}{
		{cg, "CGPreflightPostEventAccess"}, {cg, "CGEventCreateKeyboardEvent"},
		{cg, "CGEventSetFlags"}, {cg, "CGEventPost"}, {cf, "CFRelease"},
	} {
		if _, err := purego.Dlsym(symbol.handle, symbol.name); err != nil {
			cgLoadErr = err
			return
		}
	}
	purego.RegisterLibFunc(&cgHandles.preflight, cg, "CGPreflightPostEventAccess")
	purego.RegisterLibFunc(&cgHandles.createKey, cg, "CGEventCreateKeyboardEvent")
	purego.RegisterLibFunc(&cgHandles.setFlags, cg, "CGEventSetFlags")
	purego.RegisterLibFunc(&cgHandles.post, cg, "CGEventPost")
	purego.RegisterLibFunc(&cgHandles.release, cf, "CFRelease")
}

func cgAvailable() bool {
	cgOnce.Do(loadCGSymbols)
	return cgLoadErr == nil
}

// devtoolsPostAccess answers whether synthesized key events are already
// allowed. CGPreflightPostEventAccess only reads the TCC answer for the
// PostEvent service; it cannot show an authorization dialog.
func devtoolsPostAccess() bool {
	if !cgAvailable() {
		return false
	}
	return cgHandles.preflight()
}

// devtoolsFrontmostPID records the application in front before the panel
// closes, so the paste can be checked against it afterwards.
func devtoolsFrontmostPID() int32 {
	return systemSelectionReader{}.FrontmostPID()
}

// postDevtoolsCommandV synthesizes one Cmd+V. It refuses when the symbols are
// missing, the permission is not granted, or another application came to the
// front while the panel closed; it never asks for permission and never touches
// the clipboard.
func postDevtoolsCommandV(target int32) error {
	if !cgAvailable() {
		return errors.New("devtools: key events unavailable")
	}
	if !cgHandles.preflight() {
		return errors.New("devtools: key events not allowed")
	}
	if target > 0 && devtoolsFrontmostPID() != target {
		return errors.New("devtools: another application is in front")
	}
	down := cgHandles.createKey(0, keyV, true)
	if down == 0 {
		return errors.New("devtools: no key event")
	}
	defer cgHandles.release(down)
	cgHandles.setFlags(down, eventFlagCommand)
	up := cgHandles.createKey(0, keyV, false)
	if up == 0 {
		return errors.New("devtools: no key event")
	}
	defer cgHandles.release(up)
	cgHandles.setFlags(up, eventFlagCommand)
	cgHandles.post(eventTapHID, down)
	cgHandles.post(eventTapHID, up)
	return nil
}
