package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/egoist/mygo"
)

// Every test replaces Show before sending; none enters MyGo's real notifier.
func TestBatteryNotificationUsesLocalizedOptions(t *testing.T) {
	a := awakeTestApp(t)
	for _, locale := range []string{"zh-CN", "en"} {
		var sent []mygo.NotificationOptions
		n := newBatteryNotifier()
		n.show = func(opts mygo.NotificationOptions) error { sent = append(sent, opts); return nil }
		n.record = func(err error) { t.Fatalf("unexpected notification error: %v", err) }
		a.notifyBattery = n.send
		a.notifyBattery(a.text(locale, "R05BatteryTitle"), a.text(locale, "R05BatteryBody"))
		if len(sent) != 1 || sent[0].ID != "cubbix.awake.battery" || sent[0].Title != a.text(locale, "R05BatteryTitle") || sent[0].Body != a.text(locale, "R05BatteryBody") {
			t.Fatalf("notification options in %s: %+v", locale, sent)
		}
	}
}

func TestBatteryNotificationDeniedIsRecordedWithoutRetry(t *testing.T) {
	for _, denied := range []error{mygo.ErrNotificationsDenied, fmt.Errorf("notification: %w", mygo.ErrNotificationsDenied)} {
		n := newBatteryNotifier()
		calls, records := 0, 0
		n.show = func(mygo.NotificationOptions) error { calls++; return denied }
		n.record = func(err error) {
			records++
			if !errors.Is(err, mygo.ErrNotificationsDenied) {
				t.Fatal(err)
			}
		}
		p := powerMonitor{notify: func() { n.send("title", "body") }}
		p.sample(false, true)
		p.sample(true, true)
		p.sample(false, true)
		p.sample(true, true)
		n.send("title", "body")
		if calls != 1 || records != 1 || !n.denied {
			t.Fatalf("calls=%d records=%d denied=%v", calls, records, n.denied)
		}
	}
}

func TestBatteryNotificationFailureDoesNotRetry(t *testing.T) {
	n := newBatteryNotifier()
	calls, records := 0, 0
	failure := errors.New("delivery failed")
	n.show = func(mygo.NotificationOptions) error { calls++; return failure }
	n.record = func(err error) {
		records++
		if !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	n.send("title", "body")
	if calls != 1 || records != 1 || n.denied {
		t.Fatalf("calls=%d records=%d denied=%v", calls, records, n.denied)
	}
}
