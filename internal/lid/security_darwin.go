package lid

import (
	"errors"
	"github.com/ebitengine/purego"
	"net"
	"sync"
	"syscall"
	"unsafe"
)

var secOnce sync.Once
var secErr error
var cfData func(uintptr, unsafe.Pointer, int64) uintptr
var cfString func(uintptr, string, uint32) uintptr
var cfDict func(uintptr, unsafe.Pointer, unsafe.Pointer, int64, uintptr, uintptr) uintptr
var cfRelease func(uintptr)
var secGuest func(uintptr, uintptr, uint32, *uintptr) int32
var secReq func(uintptr, uint32, *uintptr) int32
var secCheck func(uintptr, uint32, uintptr) int32
var auditKey, keyCallbacks, valueCallbacks uintptr

func initSecurity() {
	cf, e := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if e != nil {
		secErr = e
		return
	}
	sec, e := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if e != nil {
		secErr = e
		return
	}
	purego.RegisterLibFunc(&cfData, cf, "CFDataCreate")
	purego.RegisterLibFunc(&cfString, cf, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&cfDict, cf, "CFDictionaryCreate")
	purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
	purego.RegisterLibFunc(&secGuest, sec, "SecCodeCopyGuestWithAttributes")
	purego.RegisterLibFunc(&secReq, sec, "SecRequirementCreateWithString")
	purego.RegisterLibFunc(&secCheck, sec, "SecCodeCheckValidity")
	p, e := purego.Dlsym(sec, "kSecGuestAttributeAudit")
	if e != nil {
		secErr = e
		return
	}
	// The exported CFString is a pointer variable. Copy its value rather
	// than converting a stored uintptr to a Go pointer (which vet rejects).
	system, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		secErr = err
		return
	}
	var copyPointer func(*uintptr, uintptr, uintptr) uintptr
	purego.RegisterLibFunc(&copyPointer, system, "memcpy")
	copyPointer(&auditKey, p, unsafe.Sizeof(auditKey))
	keyCallbacks, e = purego.Dlsym(cf, "kCFTypeDictionaryKeyCallBacks")
	if e != nil {
		secErr = e
		return
	}
	valueCallbacks, secErr = purego.Dlsym(cf, "kCFTypeDictionaryValueCallBacks")
}
func VerifyPeer(c *net.UnixConn, requirement string) error {
	secOnce.Do(initSecurity)
	if secErr != nil {
		return secErr
	}
	raw, e := c.SyscallConn()
	if e != nil {
		return e
	}
	var token [32]byte
	size := uint32(len(token))
	var sockErr error
	e = raw.Control(func(fd uintptr) {
		_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, 0, 6, uintptr(unsafe.Pointer(&token[0])), uintptr(unsafe.Pointer(&size)), 0)
		if errno != 0 {
			sockErr = errno
		}
	})
	if e != nil {
		return e
	}
	if sockErr != nil {
		return sockErr
	}
	if size != 32 {
		return errors.New("invalid audit token")
	}
	data := cfData(0, unsafe.Pointer(&token[0]), 32)
	if data == 0 {
		return errors.New("data")
	}
	defer cfRelease(data)
	dict := cfDict(0, unsafe.Pointer(&auditKey), unsafe.Pointer(&data), 1, keyCallbacks, valueCallbacks)
	if dict == 0 {
		return errors.New("dict")
	}
	defer cfRelease(dict)
	var guest uintptr
	if secGuest(0, dict, 0, &guest) != 0 {
		return errors.New("guest")
	}
	defer cfRelease(guest)
	str := cfString(0, requirement, 0x08000100)
	if str == 0 {
		return errors.New("string")
	}
	defer cfRelease(str)
	var req uintptr
	if secReq(str, 0, &req) != 0 {
		return errors.New("requirement")
	}
	defer cfRelease(req)
	if secCheck(guest, 0, req) != 0 {
		return errors.New("peer rejected")
	}
	return nil
}
