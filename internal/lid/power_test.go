package lid

import (
	"testing"
	"time"
)

// Plugged is the single v1 formula: AC power or an adapter description is
// enough. NULL details and binding failures both keep protection enabled.
func TestPluggedTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
		a    AdapterState
		want bool
	}{
		{"ac-and-attached", "ac", AdapterAttached, true},
		{"ac-and-null-details", "ac", AdapterAbsent, true},
		{"ac-and-unknown", "ac", AdapterUnknown, true},
		{"battery-attached", "battery", AdapterAttached, true},
		{"battery-absent", "battery", AdapterAbsent, false},
		{"battery-unknown", "battery", AdapterUnknown, false},
		{"unknown-kind-attached", "unknown", AdapterAttached, true},
		{"unknown-kind-absent", "unknown", AdapterAbsent, false},
		{"unknown-kind-unknown", "unknown", AdapterUnknown, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Plugged(PowerSample{Kind: tc.kind, Adapter: tc.a}); got != tc.want {
				t.Fatalf("Plugged = %v, want %v", got, tc.want)
			}
			if label := AdapterLabel(tc.a); label == "" {
				t.Fatal("empty adapter label")
			}
		})
	}
	if AdapterLabel(AdapterAttached) != "yes" || AdapterLabel(AdapterAbsent) != "no" || AdapterLabel(AdapterUnknown) != "unknown" {
		t.Fatal("adapter diagnostic labels changed")
	}
}

// The helper's 30s sampling only restores when unplugged and at/below the
// configured limit. Every case keeps the 15s lease alive with short pings.
func TestBatteryProtectionUsesPlugged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    string
		adapter AdapterState
		percent int
		min     int
		restore bool
	}{
		{"battery-attached-low", "battery", AdapterAttached, 5, 20, false},
		{"battery-absent-low", "battery", AdapterAbsent, 5, 20, true},
		{"ac-low", "ac", AdapterAbsent, 5, 20, false},
		{"unknown-adapter-low", "battery", AdapterUnknown, 5, 20, true},
		{"five-percent-four", "battery", AdapterAbsent, 4, 5, true},
		{"five-percent-six", "battery", AdapterAbsent, 6, 5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, s, p, now := fixture(t)
			r := request("enable")
			r.MinPercent = tc.min
			if out := e.Handle(1, r); !out.OK {
				t.Fatal(out)
			}
			p.kind, p.p, p.adapter = tc.kind, tc.percent, tc.adapter
			for range 6 {
				advance(now, 5*time.Second)
				e.Handle(1, request("ping"))
			}
			if tc.restore {
				if s.v != 0 || e.journal != nil || e.code != "battery-low" {
					t.Fatal("protection did not restore", s.v, e.code)
				}
			} else if s.v != 1 || e.journal == nil || e.state != "on-ours" {
				t.Fatal("plugged session was ended", s.v, e.state, e.code)
			}
		})
	}
}

// The enable preflight uses the same Plugged rule as the helper's tick.
func TestEnablePreflightUsesPlugged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    string
		adapter AdapterState
		percent int
		ok      bool
	}{
		{"battery-attached", "battery", AdapterAttached, 15, true},
		{"battery-absent", "battery", AdapterAbsent, 15, false},
		{"adapter-unknown", "battery", AdapterUnknown, 15, false},
		{"ac-with-null-details", "ac", AdapterAbsent, 15, true},
		{"ac-unknown-charge", "ac", AdapterAttached, -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, s, p, _ := fixture(t)
			p.kind, p.p, p.adapter = tc.kind, tc.percent, tc.adapter
			r := e.Handle(1, request("enable"))
			if r.OK != tc.ok {
				t.Fatalf("ok = %v, want %v: %+v", r.OK, tc.ok, r)
			}
			if !tc.ok {
				if r.Code != "battery-low" || e.journal != nil || len(s.writes) != 0 {
					t.Fatal("rejected enable left a liability", r, s.writes)
				}
			} else if r.Adapter == "" {
				t.Fatal("missing adapter diagnostic")
			}
		})
	}
}
