package main

import "testing"

func TestLidLegacyStatusWithInstalledFiles(t *testing.T) {
	for _, tc := range []struct {
		native int64
		want   lidServiceStatus
	}{
		{0, svcNotRunning},
		{1, svcEnabled},
		{2, svcRequiresApproval},
		{3, svcNotRunning},
		{-1, svcBroken},
		{4, svcBroken},
	} {
		if got := legacyLidStatus(tc.native); got != tc.want {
			t.Errorf("native status %d: got %d, want %d", tc.native, got, tc.want)
		}
	}
}
