package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func awaitUpdate(t *testing.T, u *updateManager, changes <-chan struct{}, match func(updateState) bool) updateState {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		state := u.snapshot()
		if match(state) {
			return state
		}
		select {
		case <-changes:
		case <-deadline.C:
			t.Fatalf("update timed out: %+v", state)
		}
	}
}

func testUpdateManager(t *testing.T, check updateCheck) (*updateManager, <-chan struct{}) {
	t.Helper()
	changes := make(chan struct{}, 64)
	u := newUpdateManager(check, nil, func() {
		select {
		case changes <- struct{}{}:
		default:
		}
	})
	t.Cleanup(u.close)
	return u, changes
}

func TestManualUpdateDownloadsWithAutomaticOff(t *testing.T) {
	var installs atomic.Int32
	u, changes := testUpdateManager(t, func(context.Context) (*updateCandidate, error) {
		return &updateCandidate{"0.0.2", func(context.Context) error { installs.Add(1); return nil }}, nil
	})
	u.request(false)
	if u.snapshot().Phase != updateIdle {
		t.Fatal("automatic check ran while disabled")
	}
	u.request(true)
	state := awaitUpdate(t, u, changes, func(s updateState) bool { return s.Phase == updateReady })
	if state.Version != "0.0.2" || installs.Load() != 1 {
		t.Fatalf("ready=%+v installs=%d", state, installs.Load())
	}
	u.request(true)
	if installs.Load() != 1 {
		t.Fatal("reinstalled while waiting for restart")
	}
}

func TestAutomaticStartupAndPeriodicChecks(t *testing.T) {
	var checks atomic.Int32
	u, changes := testUpdateManager(t, func(context.Context) (*updateCandidate, error) { checks.Add(1); return nil, nil })
	u.start(true, 10*time.Millisecond)
	awaitUpdate(t, u, changes, func(s updateState) bool { return checks.Load() >= 2 })
	u.setAutomatic(false)
	awaitUpdate(t, u, changes, func(s updateState) bool { return s.Phase == updateCurrent || s.Phase == updateIdle })
	before := checks.Load()
	timer := time.NewTimer(40 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	if checks.Load() != before {
		t.Fatal("periodic checks continued after disabling automatic updates")
	}
}

func TestDisableDuringCheckPreventsDownload(t *testing.T) {
	var installs atomic.Int32
	u, changes := testUpdateManager(t, func(ctx context.Context) (*updateCandidate, error) {
		<-ctx.Done()
		return &updateCandidate{"0.0.2", func(context.Context) error { installs.Add(1); return nil }}, nil
	})
	u.setAutomatic(true)
	u.setAutomatic(false)
	awaitUpdate(t, u, changes, func(s updateState) bool { return s.Phase == updateIdle })
	if installs.Load() != 0 {
		t.Fatal("download started after automatic updates were disabled")
	}
}

func TestDisableCancelsAutomaticDownloadButNotManual(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(map[bool]string{false: "automatic", true: "manual"}[manual], func(t *testing.T) {
			started, finish := make(chan struct{}), make(chan struct{})
			u, changes := testUpdateManager(t, func(context.Context) (*updateCandidate, error) {
				return &updateCandidate{"0.0.2", func(ctx context.Context) error {
					close(started)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-finish:
						return nil
					}
				}}, nil
			})
			if manual {
				u.request(true)
			} else {
				u.setAutomatic(true)
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("download did not start")
			}
			u.setAutomatic(false)
			if manual {
				close(finish)
				awaitUpdate(t, u, changes, func(s updateState) bool { return s.Phase == updateReady })
			} else {
				awaitUpdate(t, u, changes, func(s updateState) bool { return s.Phase == updateIdle })
			}
		})
	}
}

func TestFailedInstallCannotBecomeReadyAndCanRetry(t *testing.T) {
	var attempts atomic.Int32
	u, changes := testUpdateManager(t, func(context.Context) (*updateCandidate, error) {
		return &updateCandidate{"0.0.2", func(context.Context) error {
			if attempts.Add(1) == 1 {
				return errors.New("invalid update signature")
			}
			return nil
		}}, nil
	})
	u.request(true)
	state := awaitUpdate(t, u, changes, func(s updateState) bool { return s.ErrorKey != "" })
	if state.Phase == updateReady || state.ErrorKey != "UpdateSignatureError" {
		t.Fatalf("failed install: %+v", state)
	}
	u.request(true)
	awaitUpdate(t, u, changes, func(s updateState) bool { return s.Phase == updateReady })
}

func TestConcurrentChecksCoalesce(t *testing.T) {
	var checks atomic.Int32
	done := make(chan struct{})
	u, changes := testUpdateManager(t, func(context.Context) (*updateCandidate, error) { checks.Add(1); <-done; return nil, nil })
	u.request(true)
	for range 10 {
		u.request(true)
	}
	close(done)
	awaitUpdate(t, u, changes, func(s updateState) bool { return s.Phase == updateCurrent })
	if checks.Load() != 1 {
		t.Fatalf("duplicate checks = %d", checks.Load())
	}
}

func TestNotesFailureIsVisibleAndRetryable(t *testing.T) {
	u, changes := testUpdateManager(t, nil)
	u.notes = func(context.Context) (string, error) { return "", context.DeadlineExceeded }
	u.loadNotes()
	state := awaitUpdate(t, u, changes, func(s updateState) bool { return s.NotesError != "" })
	if state.NotesError != "UpdateNetworkError" {
		t.Fatalf("notes failure = %+v", state)
	}
	u.notes = func(context.Context) (string, error) { return "## 0.0.2\n\nNotes", nil }
	u.loadNotes()
	awaitUpdate(t, u, changes, func(s updateState) bool { return !s.NotesLoading && s.Notes != "" && s.NotesError == "" })
}
