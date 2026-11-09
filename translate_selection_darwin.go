//go:build darwin

package main

import (
	"bytes"
	"errors"
	"os"
	"sync"
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
	_, _ = purego.Dlopen(appKitLibrary, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	for _, symbol := range []struct {
		handle uintptr
		name   string
	}{
		{cf, "CFStringCreateWithCString"}, {cf, "CFStringGetLength"},
		{cf, "CFStringGetMaximumSizeForEncoding"}, {cf, "CFStringGetCString"},
		{cf, "CFRelease"}, {cf, "CFGetTypeID"}, {cf, "CFStringGetTypeID"},
		{his, "AXIsProcessTrusted"}, {his, "AXUIElementCreateApplication"},
		{his, "AXUIElementCopyAttributeValue"},
	} {
		if _, err := purego.Dlsym(symbol.handle, symbol.name); err != nil {
			axLoadErr = err
			return
		}
	}
	purego.RegisterLibFunc(&axHandles.isTrusted, his, "AXIsProcessTrusted")
	purego.RegisterLibFunc(&axHandles.createApp, his, "AXUIElementCreateApplication")
	purego.RegisterLibFunc(&axHandles.copyAttribute, his, "AXUIElementCopyAttributeValue")
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
	answer := readSelection(systemSelectionReader{}, int32(os.Getpid()))
	return answer.text, answer.status == selectionFound
}

func (systemSelectionReader) SelectedText(pid int32) (string, error) {
	if pid <= 0 || !axAvailable() {
		return "", errSelectionUnavailable
	}
	app := axHandles.createApp(pid)
	if app == 0 {
		return "", errSelectionUnavailable
	}
	defer axHandles.release(app)

	focusedName := axHandles.createString(0, axAttributeFocusedUIElement, cfStringEncodingUTF8)
	if focusedName == 0 {
		return "", errSelectionIllegalArgument
	}
	defer axHandles.release(focusedName)
	var focused uintptr
	code := axHandles.copyAttribute(app, focusedName, unsafe.Pointer(&focused))
	if err := axSelectionError(code); err != nil {
		return "", err
	}
	if focused == 0 {
		return "", errSelectionNoValue
	}
	defer axHandles.release(focused)

	selectedName := axHandles.createString(0, axAttributeSelectedText, cfStringEncodingUTF8)
	if selectedName == 0 {
		return "", errSelectionIllegalArgument
	}
	defer axHandles.release(selectedName)
	var value uintptr
	code = axHandles.copyAttribute(focused, selectedName, unsafe.Pointer(&value))
	if err := axSelectionError(code); err != nil {
		return "", err
	}
	if value == 0 {
		return "", errSelectionNoValue
	}
	defer axHandles.release(value)
	if axHandles.getTypeID(value) != axHandles.cfStringTypeID() {
		return "", errSelectionUnavailable
	}
	return axGoString(value), nil
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

// axGoString copies a CFStringRef into Go memory as UTF-8. The buffer comes
// from CoreFoundation and is bounded before allocation.
func axGoString(ref uintptr) string {
	if ref == 0 {
		return ""
	}
	size := axHandles.stringMaxSize(axHandles.stringLength(ref), cfStringEncodingUTF8) + 1
	if size <= 0 || size > maxSelectionSystemBytes {
		return ""
	}
	buffer := make([]byte, size)
	if axHandles.stringBytes(ref, unsafe.Pointer(&buffer[0]), size, cfStringEncodingUTF8) == 0 {
		return ""
	}
	end := bytes.IndexByte(buffer, 0)
	if end < 0 {
		end = len(buffer)
	}
	return string(buffer[:end])
}
