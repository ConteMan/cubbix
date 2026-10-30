//go:build darwin

package main

import (
	"bytes"
	"image"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/tiff"
)

// Native objects and their autorelease pool remain on the worker's OS thread.
// This path only reads metadata; it never launches or activates applications.
func nativeProcessPool() func() {
	runtime.LockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	return func() { pool.Send(objc.RegisterName("drain")); runtime.UnlockOSThread() }
}

func nativeProcessString(value objc.ID) string {
	if value == 0 {
		return ""
	}
	return objc.Send[string](value, objc.RegisterName("UTF8String"))
}

func runningGUIApplications() []graphicalProcess {
	defer nativeProcessPool()()
	workspace := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
	apps := workspace.Send(objc.RegisterName("runningApplications"))
	count := objc.Send[uint64](apps, objc.RegisterName("count"))
	out := make([]graphicalProcess, 0, count)
	for i := uint64(0); i < count; i++ {
		app := apps.Send(objc.RegisterName("objectAtIndex:"), i)
		if objc.Send[int64](app, objc.RegisterName("activationPolicy")) != 0 {
			continue
		}
		out = append(out, graphicalProcess{
			PID:        int(objc.Send[int32](app, objc.RegisterName("processIdentifier"))),
			Name:       nativeProcessString(app.Send(objc.RegisterName("localizedName"))),
			BundlePath: nativeProcessString(app.Send(objc.RegisterName("bundleURL")).Send(objc.RegisterName("path"))),
		})
	}
	return out
}

// Called exclusively by the icon cache workers, once per bundle path.
func graphicalAppIcon(bundlePath string) *ui.Bitmap {
	defer nativeProcessPool()()
	workspace := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
	path := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), bundlePath+"\x00")
	icon := workspace.Send(objc.RegisterName("iconForFile:"), path)
	if icon == 0 {
		return nil
	}
	// Draw just one 48px, 8-bit representation. Serializing an NSImage's
	// entire representation set can exceed 70 MB, and HDR TIFF samples are
	// not supported by the Go decoder. The graphics context is worker-local.
	colorSpace := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), "NSDeviceRGBColorSpace\x00")
	rep := objc.ID(objc.GetClass("NSBitmapImageRep")).Send(objc.RegisterName("alloc")).Send(
		objc.RegisterName("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:"),
		uintptr(0), 48, 48, 8, 4, true, false, colorSpace, 48*4, 32)
	if rep == 0 {
		return nil
	}
	defer rep.Send(objc.RegisterName("release"))
	graphicsClass := objc.ID(objc.GetClass("NSGraphicsContext"))
	graphics := graphicsClass.Send(objc.RegisterName("graphicsContextWithBitmapImageRep:"), rep)
	if graphics == 0 {
		return nil
	}
	graphicsClass.Send(objc.RegisterName("saveGraphicsState"))
	graphicsClass.Send(objc.RegisterName("setCurrentContext:"), graphics)
	rect := struct{ X, Y, W, H float64 }{W: 48, H: 48}
	icon.Send(objc.RegisterName("drawInRect:fromRect:operation:fraction:"), rect, struct{ X, Y, W, H float64 }{}, uintptr(2), float64(1))
	graphicsClass.Send(objc.RegisterName("restoreGraphicsState"))
	data := rep.Send(objc.RegisterName("TIFFRepresentation"))
	if data == 0 {
		return nil
	}
	size := objc.Send[uint64](data, objc.RegisterName("length"))
	ptr := objc.Send[unsafe.Pointer](data, objc.RegisterName("bytes"))
	if size == 0 || size > 16<<20 || ptr == nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(unsafe.Slice((*byte)(ptr), int(size))))
	if err != nil {
		return nil
	}
	return ui.NewBitmap(img)
}
