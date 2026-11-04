package main

import (
	"testing"
	"time"
)

func TestQuitDeadlineSurvivesRepeatedRequests(t *testing.T) {
	expired := make(chan bool, 2)
	g := &quitWatchdog{budget: 20 * time.Millisecond, expire: func(restart bool) { expired <- restart }}
	g.arm(false)
	g.arm(true)
	g.arm(false)
	select {
	case restart := <-expired:
		if !restart {
			t.Fatal("restart intent lost after a later quit request")
		}
	case <-time.After(time.Second):
		t.Fatal("quit deadline did not fire")
	}
	g.arm(true)
	select {
	case <-expired:
		t.Fatal("repeated quit scheduled a second fallback")
	case <-time.After(40 * time.Millisecond):
	}
}

func TestFallbackRejectsUnpackagedExecutable(t *testing.T) {
	if err := relaunchAfterExit("relative-executable", 0); err == nil {
		t.Fatal("unpackaged restart accepted")
	}
}
