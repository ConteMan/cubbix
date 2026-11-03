package main

import (
	"errors"
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

const lidInstalledPath = "/Library/PrivilegedHelperTools/com.conteman.cubbix.helper"
const lidPlistPath = "/Library/LaunchDaemons/com.conteman.cubbix.helper.plist"

type legacyLidService struct {
	admin    func(string) error
	allowed  func() bool
	restored func() bool
}

func (s *legacyLidService) Status() lidServiceStatus {
	if !s.allowed() {
		return svcUnavailable
	}
	_, helper := s.HelperPath()
	_, plistErr := os.Stat(lidPlistPath)
	if !helper && os.IsNotExist(plistErr) {
		return svcNotRegistered
	}
	if !helper || plistErr != nil {
		return svcBroken
	}
	defer nativeProcessPool()()
	cls := objc.ID(objc.GetClass("SMAppService"))
	if cls == 0 {
		return svcUnavailable
	}
	path := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), lidPlistPath+"\x00")
	url := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("fileURLWithPath:"), path)
	switch objc.Send[int64](cls, objc.RegisterName("statusForLegacyURL:"), url) {
	case 0:
		return svcNotRegistered
	case 1:
		return svcEnabled
	case 2:
		return svcRequiresApproval
	default:
		return svcBroken
	}
}
func (s *legacyLidService) Register() error {
	if !s.allowed() {
		return errors.New("LidUnsupported")
	}
	return s.admin(lidInstallScriptApple)
}
func (s *legacyLidService) Unregister() error {
	if !s.allowed() {
		return errors.New("LidUnsupported")
	}
	if !s.restored() {
		return errors.New("LidRestoreFailed")
	}
	return s.admin(lidUninstallScriptApple)
}
func (s *legacyLidService) OpenSettings() {
	if !s.allowed() {
		return
	}
	defer nativeProcessPool()()
	objc.ID(objc.GetClass("SMAppService")).Send(objc.RegisterName("openSystemSettingsLoginItems"))
}
func (s *legacyLidService) HelperPath() (string, bool) {
	info, err := os.Stat(lidInstalledPath)
	return lidInstalledPath, err == nil && info.Mode().IsRegular()
}

var displayOnce sync.Once
var displayBuiltin func(uint32) uint32
var displayList func(uint32, unsafe.Pointer, *uint32) int32

func lidBuiltinDisplay() bool {
	displayOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err == nil {
			purego.RegisterLibFunc(&displayBuiltin, lib, "CGDisplayIsBuiltin")
			purego.RegisterLibFunc(&displayList, lib, "CGGetOnlineDisplayList")
		}
	})
	if displayList == nil {
		return false
	}
	var displays [32]uint32
	var count uint32
	if displayList(32, unsafe.Pointer(&displays[0]), &count) != 0 {
		return false
	}
	for _, id := range displays[:min(count, 32)] {
		if displayBuiltin(id) != 0 {
			return true
		}
	}
	return false
}
func lidOSSupported() bool {
	defer nativeProcessPool()()
	info := objc.ID(objc.GetClass("NSProcessInfo")).Send(objc.RegisterName("processInfo"))
	version := objc.Send[struct{ Major, Minor, Patch int64 }](info, objc.RegisterName("operatingSystemVersion"))
	return version.Major >= 13
}
func lidBundlePath() string {
	defer nativeProcessPool()()
	b := objc.ID(objc.GetClass("NSBundle")).Send(objc.RegisterName("mainBundle"))
	return nativeProcessString(b.Send(objc.RegisterName("bundlePath")))
}
func lidPackagedHelper() bool {
	dir, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "bin", "cubbix-helper"))
	return err == nil && info.Mode().IsRegular()
}
