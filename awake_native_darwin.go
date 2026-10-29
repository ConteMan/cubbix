//go:build darwin

package main

import (
	"bytes"
	"image"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/tiff"
)

func nsString(s string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), s)
}

// Submit directly to UNUserNotificationCenter. Deliberately never request
// authorization: denied or undetermined permission silently drops the reminder.
// Tests replace this sender and never enter the notification framework.
func sendBatteryNotification(title, body string) {
	if !mygo.NotificationsSupported() {
		return
	}
	_, err := purego.Dlopen("/System/Library/Frameworks/UserNotifications.framework/UserNotifications", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	center := objc.ID(objc.GetClass("UNUserNotificationCenter")).Send(objc.RegisterName("currentNotificationCenter"))
	content := objc.ID(objc.GetClass("UNMutableNotificationContent")).Send(objc.RegisterName("new"))
	defer content.Send(objc.RegisterName("release"))
	content.Send(objc.RegisterName("setTitle:"), nsString(title))
	content.Send(objc.RegisterName("setBody:"), nsString(body))
	request := objc.ID(objc.GetClass("UNNotificationRequest")).Send(objc.RegisterName("requestWithIdentifier:content:trigger:"), nsString("cubbix.awake.battery"), content, objc.ID(0))
	center.Send(objc.RegisterName("addNotificationRequest:withCompletionHandler:"), request, uintptr(0))
}

// Read the icon of a running graphical application; never launch or activate it.
func runningAppIcon(pid int) *ui.Bitmap {
	app := objc.ID(objc.GetClass("NSRunningApplication")).Send(objc.RegisterName("runningApplicationWithProcessIdentifier:"), pid)
	if app == 0 {
		return nil
	}
	icon := app.Send(objc.RegisterName("icon"))
	if icon == 0 {
		return nil
	}
	data := icon.Send(objc.RegisterName("TIFFRepresentation"))
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
