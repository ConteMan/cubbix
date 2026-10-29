package main

import (
	"errors"
	"log"

	"github.com/egoist/mygo"
)

// The sender is replaceable so tests never display a notification or request
// permission. A denied sender stays quiet for the rest of this app run.
// Production calls send on the main thread, just like the other UI actions.
type batteryNotifier struct {
	show   func(mygo.NotificationOptions) error
	record func(error)
	denied bool
}

func newBatteryNotifier() *batteryNotifier {
	return &batteryNotifier{
		show: func(opts mygo.NotificationOptions) error {
			return mygo.NewNotification(opts).Show()
		},
		record: func(err error) { log.Printf("battery notification: %v", err) },
	}
}

func (n *batteryNotifier) send(title, body string) {
	if n.denied {
		return
	}
	err := n.show(mygo.NotificationOptions{ID: "cubbix.awake.battery", Title: title, Body: body})
	if err == nil {
		return
	}
	if errors.Is(err, mygo.ErrNotificationsDenied) {
		n.denied = true
	}
	n.record(err)
}
