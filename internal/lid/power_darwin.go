package lid

import (
	"errors"
	"github.com/ebitengine/purego"
	"math"
	"sync"
	"unsafe"
)

// CapacityPercent deliberately rejects absent, zero and inconsistent capacity.
func CapacityPercent(current, maxCapacity int64) (int, error) {
	if maxCapacity <= 0 || current < 0 || current > maxCapacity {
		return -1, errors.New("invalid capacity")
	}
	return int(math.Floor(float64(current) / float64(maxCapacity) * 100)), nil
}

var powerOnce sync.Once
var powerErr error
var iopsInfo func() uintptr
var iopsList func(uintptr) uintptr
var iopsDescription func(uintptr, uintptr) uintptr
var arrayCount func(uintptr) int64
var arrayValue func(uintptr, int64) uintptr
var dictValue func(uintptr, uintptr) uintptr
var numberValue func(uintptr, int32, unsafe.Pointer) bool
var cfEqual func(uintptr, uintptr) bool
var typeID func(uintptr) uintptr
var numberType func() uintptr

func initPower() {
	secOnce.Do(initSecurity)
	if secErr != nil {
		powerErr = secErr
		return
	}
	io, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		powerErr = err
		return
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		powerErr = err
		return
	}
	purego.RegisterLibFunc(&iopsInfo, io, "IOPSCopyPowerSourcesInfo")
	purego.RegisterLibFunc(&iopsList, io, "IOPSCopyPowerSourcesList")
	purego.RegisterLibFunc(&iopsDescription, io, "IOPSGetPowerSourceDescription")
	purego.RegisterLibFunc(&arrayCount, cf, "CFArrayGetCount")
	purego.RegisterLibFunc(&arrayValue, cf, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&dictValue, cf, "CFDictionaryGetValue")
	purego.RegisterLibFunc(&numberValue, cf, "CFNumberGetValue")
	purego.RegisterLibFunc(&cfEqual, cf, "CFEqual")
	purego.RegisterLibFunc(&typeID, cf, "CFGetTypeID")
	purego.RegisterLibFunc(&numberType, cf, "CFNumberGetTypeID")
}

type SystemPower struct{}

func (SystemPower) Source() (string, int, error) {
	powerOnce.Do(initPower)
	if powerErr != nil {
		return "unknown", -1, powerErr
	}
	info := iopsInfo()
	if info == 0 {
		return "unknown", -1, errors.New("power info")
	}
	defer cfRelease(info)
	list := iopsList(info)
	if list == 0 {
		return "unknown", -1, errors.New("power list")
	}
	defer cfRelease(list)
	str := func(s string) uintptr { return cfString(0, s, 0x08000100) }
	for i := int64(0); i < arrayCount(list); i++ {
		d := iopsDescription(info, arrayValue(list, i))
		if d == 0 {
			continue
		}
		get := func(key string) uintptr { k := str(key); defer cfRelease(k); return dictValue(d, k) }
		internal := str("InternalBattery")
		typ := get("Type")
		matches := typ != 0 && cfEqual(typ, internal)
		cfRelease(internal)
		if !matches {
			continue
		}
		current, maxCap := int64(0), int64(0)
		c, m := get("Current Capacity"), get("Max Capacity")
		if c == 0 || m == 0 || typeID(c) != numberType() || typeID(m) != numberType() || !numberValue(c, 4, unsafe.Pointer(&current)) || !numberValue(m, 4, unsafe.Pointer(&maxCap)) {
			return "unknown", -1, errors.New("power capacity")
		}
		p, err := CapacityPercent(current, maxCap)
		if err != nil {
			return "unknown", -1, err
		}
		state := get("Power Source State")
		ac := str("AC Power")
		battery := str("Battery Power")
		defer cfRelease(ac)
		defer cfRelease(battery)
		if state != 0 && cfEqual(state, ac) {
			return "ac", p, nil
		}
		if state != 0 && cfEqual(state, battery) {
			return "battery", p, nil
		}
	}
	return "unknown", -1, errors.New("internal battery unavailable")
}
