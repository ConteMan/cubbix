//go:build darwin

package main

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// AX attribute names are header macros (#define kAXSelectedTextAttribute
// CFSTR("AXSelectedText")), not exported symbols. Looking them up with
// dlsym yields a null CFStringRef and AXUIElementCopyAttributeValue answers
// kAXErrorIllegalArgument (-25201), so every name is built here instead.
const (
	hiservicesLibrary = "/System/Library/Frameworks/ApplicationServices.framework/Frameworks/HIServices.framework/HIServices"
	appKitLibrary     = "/System/Library/Frameworks/AppKit.framework/AppKit"

	axAttributeFocusedUIElement = "AXFocusedUIElement"
	axAttributeSelectedText     = "AXSelectedText"

	axErrorSuccess         int32 = 0
	axErrorIllegalArgument int32 = -25201
	axErrorCannotComplete  int32 = -25204
	axErrorAPIDisabled     int32 = -25211
	axErrorNoValue         int32 = -25212

	// A bounded copy keeps a pathological selection from allocating an
	// unbounded Go buffer. Final clipping to 4000 runes happens when the
	// text is submitted, where the panel can show its truncation hint.
	maxSelectionSystemBytes = 256 << 10
)

// axSymbols is the whole C surface used by the reader. Only the HIServices
// and CoreFoundation entry points are dlsym'd; the attribute strings and the
// frontmost application come from CFStringCreateWithCString and NSWorkspace.
type axSymbols struct {
	isTrusted      func() uint8
	createApp      func(int32) uintptr
	copyAttribute  func(uintptr, uintptr, unsafe.Pointer) int32
	setTimeout     func(uintptr, float32) int32
	axTypeID       func() uint64
	createString   func(uintptr, string, uint32) uintptr
	release        func(uintptr)
	stringLength   func(uintptr) int64
	stringMaxSize  func(int64, uint32) int64
	stringBytes    func(uintptr, unsafe.Pointer, int64, uint32) uint8
	getTypeID      func(uintptr) uint64
	cfStringTypeID func() uint64
}

var (
	axOnce    sync.Once
	axHandles axSymbols
	axLoadErr error
)

func loadAXSymbols() {
	cf, err := purego.Dlopen(coreFoundationLibrary, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		axLoadErr = err
		return
	}
	his, err := purego.Dlopen(hiservicesLibrary, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		axLoadErr = err
		return
	}
	// AppKit is already loaded in the app; dlopen makes the standalone
	// read-only experiment work as well. It is reference counted by dyld.
	if _, err := purego.Dlopen(appKitLibrary, purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		axLoadErr = err
		return
	}
	for _, symbol := range []struct {
		handle uintptr
		name   string
	}{
		{cf, "CFStringCreateWithCString"}, {cf, "CFStringGetLength"},
		{cf, "CFStringGetMaximumSizeForEncoding"}, {cf, "CFStringGetCString"},
		{cf, "CFRelease"}, {cf, "CFGetTypeID"}, {cf, "CFStringGetTypeID"},
		{his, "AXIsProcessTrusted"}, {his, "AXUIElementCreateApplication"},
		{his, "AXUIElementCopyAttributeValue"},
		{his, "AXUIElementSetMessagingTimeout"}, {his, "AXUIElementGetTypeID"},
	} {
		if _, err := purego.Dlsym(symbol.handle, symbol.name); err != nil {
			axLoadErr = err
			return
		}
	}
	purego.RegisterLibFunc(&axHandles.isTrusted, his, "AXIsProcessTrusted")
	purego.RegisterLibFunc(&axHandles.createApp, his, "AXUIElementCreateApplication")
	purego.RegisterLibFunc(&axHandles.copyAttribute, his, "AXUIElementCopyAttributeValue")
	purego.RegisterLibFunc(&axHandles.setTimeout, his, "AXUIElementSetMessagingTimeout")
	purego.RegisterLibFunc(&axHandles.axTypeID, his, "AXUIElementGetTypeID")
	purego.RegisterLibFunc(&axHandles.createString, cf, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&axHandles.release, cf, "CFRelease")
	purego.RegisterLibFunc(&axHandles.stringLength, cf, "CFStringGetLength")
	purego.RegisterLibFunc(&axHandles.stringMaxSize, cf, "CFStringGetMaximumSizeForEncoding")
	purego.RegisterLibFunc(&axHandles.stringBytes, cf, "CFStringGetCString")
	purego.RegisterLibFunc(&axHandles.getTypeID, cf, "CFGetTypeID")
	purego.RegisterLibFunc(&axHandles.cfStringTypeID, cf, "CFStringGetTypeID")
}

func axAvailable() bool {
	axOnce.Do(loadAXSymbols)
	return axLoadErr == nil
}

// systemSelectionReader uses only AX. It never synthesizes Cmd+C, never
// reads or writes the pasteboard and never calls the prompting variant of
// AXIsProcessTrusted.
type systemSelectionReader struct{}

// accessibilityTrusted is the spec's darwin entry point. The return value is
// the process's current TCC state; this call cannot show an authorization
// dialog. A rebuilt development binary may need to be re-approved by the
// maintainer (TCC records the signature and build identity).
func accessibilityTrusted() bool {
	if !axAvailable() {
		return false
	}
	return axHandles.isTrusted() != 0
}

func (systemSelectionReader) Trusted() bool { return accessibilityTrusted() }

// FrontmostPID uses NSWorkspace.frontmostApplication, not the system-wide AX
// element: the latter answers kAXErrorCannotComplete (-25204) outside a GUI
// session. The autorelease pool is worker-local, like the process picker.
func (systemSelectionReader) FrontmostPID() int32 {
	pool := nativeProcessPool()
	defer pool()
	if !axAvailable() {
		return 0
	}
	workspace := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
	if workspace == 0 {
		return 0
	}
	app := workspace.Send(objc.RegisterName("frontmostApplication"))
	if app == 0 {
		return 0
	}
	return objc.Send[int32](app, objc.RegisterName("processIdentifier"))
}

// copySelectedText is the spec's read-only entry point. It goes through the
// same decision tree as the shortcut: no permission, Cubbix in front or a
// failed AX read all mean "no selection".
func copySelectedText() (string, bool) {
	a := application{selection: systemSelectionReader{}}
	answer := a.captureSelection()
	return answer.text, answer.status == selectionFound
}

func (systemSelectionReader) SelectedText(ctx context.Context, pid int32) (string, error) {
	if pid <= 0 || !axAvailable() {
		return "", errSelectionUnavailable
	}
	return axHandles.selectedText(ctx, pid)
}

func (symbols axSymbols) selectedText(ctx context.Context, pid int32) (string, error) {
	app := symbols.createApp(pid)
	if app == 0 {
		return "", errSelectionUnavailable
	}
	defer symbols.release(app)

	focusedName := symbols.createString(0, axAttributeFocusedUIElement, cfStringEncodingUTF8)
	if focusedName == 0 {
		return "", errSelectionIllegalArgument
	}
	defer symbols.release(focusedName)
	if err := symbols.messagingTimeout(ctx, app); err != nil {
		return "", err
	}
	var focused uintptr
	code := symbols.copyAttribute(app, focusedName, unsafe.Pointer(&focused))
	if focused != 0 {
		defer symbols.release(focused)
	}
	if err := axSelectionError(code); err != nil {
		return "", err
	}
	if focused == 0 {
		return "", errSelectionNoValue
	}
	if symbols.getTypeID(focused) != symbols.axTypeID() {
		return "", errSelectionUnavailable
	}

	selectedName := symbols.createString(0, axAttributeSelectedText, cfStringEncodingUTF8)
	if selectedName == 0 {
		return "", errSelectionIllegalArgument
	}
	defer symbols.release(selectedName)
	if err := symbols.messagingTimeout(ctx, focused); err != nil {
		return "", err
	}
	var value uintptr
	code = symbols.copyAttribute(focused, selectedName, unsafe.Pointer(&value))
	if value != 0 {
		defer symbols.release(value)
	}
	if err := axSelectionError(code); err != nil {
		return "", err
	}
	if value == 0 {
		return "", errSelectionNoValue
	}
	if symbols.getTypeID(value) != symbols.cfStringTypeID() {
		return "", errSelectionUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return symbols.goString(value), nil
}

func (symbols axSymbols) messagingTimeout(ctx context.Context, element uintptr) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	remaining := selectionTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining = min(remaining, time.Until(deadline))
	}
	if remaining <= 0 {
		return context.DeadlineExceeded
	}
	return axSelectionError(symbols.setTimeout(element, float32(remaining.Seconds())))
}

// axSelectionError folds AXError into the status the platform-independent
// layer understands. Every failure is a "no selection" fallback except a
// disabled API, which also offers the accessibility hint once per launch.
func axSelectionError(code int32) error {
	switch code {
	case axErrorSuccess:
		return nil
	case axErrorNoValue:
		return errSelectionNoValue
	case axErrorAPIDisabled:
		return errSelectionDenied
	case axErrorIllegalArgument:
		return errSelectionIllegalArgument
	case axErrorCannotComplete:
		return errSelectionUnavailable
	default:
		return errors.New("selection: unexpected AX error")
	}
}

// goString copies a CFStringRef into Go memory as UTF-8. The buffer comes
// from CoreFoundation and is bounded before allocation.
func (symbols axSymbols) goString(ref uintptr) string {
	if ref == 0 {
		return ""
	}
	size := symbols.stringMaxSize(symbols.stringLength(ref), cfStringEncodingUTF8) + 1
	if size <= 0 || size > maxSelectionSystemBytes {
		return ""
	}
	buffer := make([]byte, size)
	if symbols.stringBytes(ref, unsafe.Pointer(&buffer[0]), size, cfStringEncodingUTF8) == 0 {
		return ""
	}
	end := bytes.IndexByte(buffer, 0)
	if end < 0 {
		end = len(buffer)
	}
	return string(buffer[:end])
}
