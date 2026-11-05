package main

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

const updateInterval = 24 * time.Hour

type updatePhase int

const (
	updateIdle updatePhase = iota
	updateChecking
	updateDownloading
	updateCurrent
	updateReady
)

type updateState struct {
	Phase        updatePhase
	Version      string
	ErrorKey     string
	Notes        string
	NotesLoading bool
	NotesError   string
	// CheckedAt is when the last check finished, successfully or not. It
	// survives a restart through the optional configuration key.
	CheckedAt time.Time
}

type updateCandidate struct {
	version string
	install func(context.Context) error
}

type updateCheck func(context.Context) (*updateCandidate, error)

// updateManager owns background work; its snapshots are safe to read from UI.
// Only the UI's explicit restart action may call App.Relaunch.
type updateManager struct {
	mu          sync.Mutex
	state       updateState
	automatic   bool
	busy        bool
	autoRequest bool
	stopped     bool
	cancel      context.CancelFunc
	ctx         context.Context
	stop        context.CancelFunc
	check       updateCheck
	notes       func(context.Context) (string, error)
	changed     func()
	// now is the clock the check time is stamped with; tests replace it.
	now func() time.Time
}

func newUpdateManager(check updateCheck, notes func(context.Context) (string, error), changed func()) *updateManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &updateManager{ctx: ctx, stop: cancel, check: check, notes: notes, changed: changed, now: time.Now}
}

func checkMyGoUpdate(ctx context.Context) (*updateCandidate, error) {
	up, err := mygo.Updater.Check(ctx)
	if err != nil || up == nil {
		return nil, err
	}
	return &updateCandidate{version: up.Version, install: func(ctx context.Context) error { return up.Install(ctx, nil) }}, nil
}

// lastCheck is the completion time of the last finished attempt.
func (u *updateManager) lastCheck() time.Time {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state.CheckedAt
}

// nowTime reads the manager's clock.
func (u *updateManager) nowTime() time.Time {
	if u.now != nil {
		return u.now()
	}
	return time.Now()
}

// restoreCheck re-injects the persisted check time at launch, even when
// automatic updates are off. An unparsable record is simply not shown.
func (u *updateManager) restoreCheck(value string, now time.Time) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Before(time.Unix(0, 0)) || parsed.After(now) {
		return
	}
	u.mu.Lock()
	u.state.CheckedAt = parsed
	u.mu.Unlock()
}

// recordCheck stamps a finished attempt. It runs under the manager lock so a
// progress notification can never read a half-written time.
func (u *updateManager) recordCheck(now time.Time) {
	u.mu.Lock()
	u.state.CheckedAt = now
	u.mu.Unlock()
}

func (u *updateManager) snapshot() updateState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

func (u *updateManager) notify() {
	if u.changed != nil {
		u.changed()
	}
}

func (u *updateManager) start(automatic bool, interval time.Duration) {
	u.setAutomatic(automatic)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				u.request(false)
			case <-u.ctx.Done():
				return
			}
		}
	}()
}

func (u *updateManager) setAutomatic(enabled bool) {
	u.mu.Lock()
	changed := u.automatic != enabled
	u.automatic = enabled
	if !enabled && u.busy && u.autoRequest {
		u.cancel()
	}
	u.mu.Unlock()
	if enabled && changed {
		u.request(false)
	}
}

func (u *updateManager) close() {
	u.mu.Lock()
	u.stopped = true
	u.stop()
	u.mu.Unlock()
}

func (u *updateManager) request(manual bool) {
	u.mu.Lock()
	if u.stopped || u.busy || u.state.Phase == updateReady || (!manual && !u.automatic) {
		u.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(u.ctx, 15*time.Minute)
	u.cancel, u.busy, u.autoRequest = cancel, true, !manual
	u.state.Phase, u.state.ErrorKey = updateChecking, ""
	u.mu.Unlock()
	u.notify()
	go func() {
		defer cancel()
		up, err := u.check(ctx)
		// A checker can return a transport error (or even a result) after
		// cancellation. The request context still owns the cancellation.
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		// A finished attempt is remembered, a cancelled one is not: turning
		// automatic updates off, or quitting, must not invent a check time.
		// A deadline is a finished failure and is remembered.
		if !errors.Is(err, context.Canceled) {
			u.recordCheck(u.nowTime())
			u.notify()
		}
		installed := false
		if err == nil && up != nil {
			// Turning automatic updates off while checking also prevents the download.
			if err = ctx.Err(); err == nil {
				u.mu.Lock()
				u.state.Phase = updateDownloading
				u.mu.Unlock()
				u.notify()
				err = up.install(ctx)
				installed = err == nil
			}
		}
		u.mu.Lock()
		u.busy = false
		switch {
		case installed:
			// A completed swap remains ready even if cancellation raced with it.
			u.state.Phase, u.state.Version = updateReady, up.version
		case errors.Is(err, context.Canceled):
			u.state.Phase = updateIdle
		case err != nil:
			u.state.Phase, u.state.ErrorKey = updateIdle, updateErrorKey(err)
		default:
			u.state.Phase = updateCurrent
		}
		current := u.state.Phase == updateCurrent
		u.mu.Unlock()
		u.notify()
		if current {
			time.AfterFunc(3*time.Second, func() {
				u.mu.Lock()
				if u.state.Phase == updateCurrent {
					u.state.Phase = updateIdle
				}
				u.mu.Unlock()
				u.notify()
			})
		}
	}()
}

func (u *updateManager) loadNotes() {
	u.mu.Lock()
	if u.stopped || u.state.NotesLoading {
		u.mu.Unlock()
		return
	}
	u.state.NotesLoading, u.state.NotesError = true, ""
	u.mu.Unlock()
	u.notify()
	go func() {
		ctx, cancel := context.WithTimeout(u.ctx, time.Minute)
		defer cancel()
		notes, err := u.notes(ctx)
		u.mu.Lock()
		u.state.NotesLoading = false
		if err != nil {
			u.state.NotesError = updateErrorKey(err)
		} else {
			u.state.Notes = notes
		}
		u.mu.Unlock()
		u.notify()
	}()
}

func updateErrorKey(err error) string {
	var network net.Error
	switch {
	case errors.Is(err, mygo.ErrUpdatesDisabled):
		return "UpdateDisabled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &network):
		return "UpdateNetworkError"
	case strings.Contains(err.Error(), "signature"), strings.Contains(err.Error(), "public key"), strings.Contains(err.Error(), "not signed with the app's key"):
		return "UpdateSignatureError"
	case strings.Contains(err.Error(), "404"):
		return "UpdateFeedUnavailable"
	case strings.Contains(err.Error(), "403"), strings.Contains(err.Error(), "429"):
		return "UpdateRateLimited"
	default:
		return "UpdateFailed"
	}
}
