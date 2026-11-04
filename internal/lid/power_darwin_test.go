package lid

import (
	"errors"
	"testing"
)

func TestInternalBatteryHardware(t *testing.T) {
	powerOnce.Do(initPower)
	oldErr, oldInfo, oldList, oldDescription := powerErr, iopsInfo, iopsList, iopsDescription
	oldCount, oldValue, oldDict := arrayCount, arrayValue, dictValue
	oldString, oldEqual, oldRelease := cfString, cfEqual, cfRelease
	t.Cleanup(func() {
		powerErr, iopsInfo, iopsList, iopsDescription = oldErr, oldInfo, oldList, oldDescription
		arrayCount, arrayValue, dictValue = oldCount, oldValue, oldDict
		cfString, cfEqual, cfRelease = oldString, oldEqual, oldRelease
	})
	for _, tc := range []struct {
		name    string
		types   []string
		infoErr bool
		noInfo  bool
		noList  bool
		want    bool
	}{
		{name: "closed lid on AC with unreadable capacity", types: []string{"InternalBattery"}, want: true},
		{name: "desktop without sources"},
		{name: "desktop with UPS", types: []string{"UPS"}},
		{name: "external battery alone", types: []string{"ExternalBattery"}},
		{name: "internal after other sources", types: []string{"UPS", "", "InternalBattery"}, want: true},
		{name: "framework failure", infoErr: true},
		{name: "missing snapshot", noInfo: true},
		{name: "missing list", noList: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			powerErr = nil
			if tc.infoErr {
				powerErr = errors.New("unavailable")
			}
			iopsInfo = func() uintptr {
				if tc.noInfo {
					return 0
				}
				return 10
			}
			iopsList = func(uintptr) uintptr {
				if tc.noList {
					return 0
				}
				return 11
			}
			arrayCount = func(uintptr) int64 { return int64(len(tc.types)) }
			arrayValue = func(_ uintptr, index int64) uintptr { return uintptr(index) + 100 }
			iopsDescription = func(_, source uintptr) uintptr { return source }
			atoms := map[string]uintptr{"Type": 1, "InternalBattery": 2, "UPS": 3, "ExternalBattery": 4}
			cfString = func(_ uintptr, s string, _ uint32) uintptr { return atoms[s] }
			dictValue = func(d, key uintptr) uintptr {
				if key != 1 {
					t.Fatal("hardware detection queried capacity or AC state")
				}
				return atoms[tc.types[d-100]]
			}
			cfEqual = func(a, b uintptr) bool { return a == b }
			released := map[uintptr]int{}
			cfRelease = func(p uintptr) { released[p]++ }
			if got := (SystemPower{}).HasInternalBattery(); got != tc.want {
				t.Fatalf("capable = %v, want %v", got, tc.want)
			}
			if !tc.infoErr && !tc.noInfo && released[10] != 1 {
				t.Fatal("snapshot not released")
			}
			if !tc.infoErr && !tc.noInfo && !tc.noList && released[11] != 1 {
				t.Fatal("source list not released")
			}
		})
	}
}
