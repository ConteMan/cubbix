package main

import (
	"errors"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"os"
	"path/filepath"
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
	return legacyLidStatus(objc.Send[int64](cls, objc.RegisterName("statusForLegacyURL:"), url))
}

// The caller has already confirmed both installed files exist.
func legacyLidStatus(status int64) lidServiceStatus {
	switch status {
	case 0, 3: // notRegistered / notFound: files exist, but no running service.
		return svcNotRunning
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
