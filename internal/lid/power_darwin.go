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
var iopsAdapterDetails func() uintptr

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
	// A missing symbol is a supported outcome: the adapter stays unknown and
	// the low-battery protection keeps its conservative behaviour.
	iopsAdapterDetails = nil
	if sym, err := purego.Dlsym(io, "IOPSCopyExternalPowerAdapterDetails"); err == nil && sym != 0 {
		purego.RegisterFunc(&iopsAdapterDetails, sym)
	}
}

type SystemPower struct{}

// HasInternalBattery identifies laptop hardware even when its lid is closed.
// UPS and external batteries do not qualify; capacity and AC state are irrelevant.
func (SystemPower) HasInternalBattery() bool {
	found := false
	_ = withPowerSources(func(info, list uintptr) error {
		found = internalBattery(info, list) != 0
		return nil
	})
	return found
}

func withPowerSources(visit func(info, list uintptr) error) error {
	powerOnce.Do(initPower)
	if powerErr != nil {
		return powerErr
	}
	info := iopsInfo()
	if info == 0 {
		return errors.New("power info")
	}
	defer cfRelease(info)
	list := iopsList(info)
	if list == 0 {
		return errors.New("power list")
	}
	defer cfRelease(list)
	return visit(info, list)
}

func powerValue(description uintptr, key string) uintptr {
	k := cfString(0, key, 0x08000100)
	defer cfRelease(k)
	return dictValue(description, k)
}

func internalBattery(info, list uintptr) uintptr {
	internal := cfString(0, "InternalBattery", 0x08000100)
	defer cfRelease(internal)
	for i := int64(0); i < arrayCount(list); i++ {
		d := iopsDescription(info, arrayValue(list, i))
		if d != 0 {
			typ := powerValue(d, "Type")
			if typ != 0 && cfEqual(typ, internal) {
				return d
			}
		}
	}
	return 0
}

// Adapter reports whether IOKit has external adapter details. NULL covers
// both "not attached" and "lookup failed"; either keeps protection enabled.
func (SystemPower) Adapter() AdapterState {
	powerOnce.Do(initPower)
	if powerErr != nil || iopsAdapterDetails == nil {
		return AdapterUnknown
	}
	d := iopsAdapterDetails()
	if d == 0 {
		return AdapterAbsent
	}
	cfRelease(d)
	return AdapterAttached
}

// Sample is the single power read: source kind, trusted capacity and the
// adapter state used by Plugged.
func (SystemPower) Sample() (PowerSample, error) {
	sample := PowerSample{Kind: "unknown", Percent: -1}
	kind, percent := "unknown", -1
	err := withPowerSources(func(info, list uintptr) error {
		d := internalBattery(info, list)
		if d == 0 {
			return errors.New("internal battery unavailable")
		}
		current, maxCap := int64(0), int64(0)
		c, m := powerValue(d, "Current Capacity"), powerValue(d, "Max Capacity")
		if c == 0 || m == 0 || typeID(c) != numberType() || typeID(m) != numberType() || !numberValue(c, 4, unsafe.Pointer(&current)) || !numberValue(m, 4, unsafe.Pointer(&maxCap)) {
			return errors.New("power capacity")
		}
		p, err := CapacityPercent(current, maxCap)
		if err != nil {
			return err
		}
		state := powerValue(d, "Power Source State")
		ac := cfString(0, "AC Power", 0x08000100)
		battery := cfString(0, "Battery Power", 0x08000100)
		defer cfRelease(ac)
		defer cfRelease(battery)
		switch {
		case state != 0 && cfEqual(state, ac):
			kind, percent = "ac", p
		case state != 0 && cfEqual(state, battery):
			kind, percent = "battery", p
		default:
			return errors.New("power state")
		}
		return nil
	})
	sample.Kind, sample.Percent = kind, percent
	sample.Adapter = (SystemPower{}).Adapter()
	return sample, err
}
