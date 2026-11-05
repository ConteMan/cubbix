package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type awakeSession struct {
	Lid      bool      `toml:"lid,omitempty"`
	Mode     string    `toml:"mode"`
	End      time.Time `toml:"end"`
	Minutes  int       `toml:"minutes,omitempty"`
	Until    string    `toml:"until,omitempty"`
	PID      int       `toml:"pid,omitempty"`
	Identity string    `toml:"identity,omitempty"`
	Name     string    `toml:"name,omitempty"`
}

type awakeState struct {
	Session awakeSession
	Active  bool
	Error   string
}

type awakeChild interface {
	Wait() error
	Kill() error
}
type commandChild struct{ *exec.Cmd }

func (c commandChild) Kill() error { return c.Process.Kill() }

type awakeController struct {
	mu         sync.Mutex
	state      awakeState
	child      awakeChild
	childDone  chan struct{}
	generation uint64
	launch     func([]string) (awakeChild, error)
	alive      func(int, string) bool
	now        func() time.Time
	changed    func()
	beforeStop func()
}

func newAwakeController() *awakeController {
	return &awakeController{now: time.Now, alive: processAlive, launch: func(args []string) (awakeChild, error) {
		cmd := exec.Command("/usr/bin/caffeinate", args...)
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return commandChild{cmd}, nil
	}}
}

func awakeArgs(s awakeSession, owner int, now time.Time) ([]string, error) {
	args := []string{}
	switch s.Mode {
	case "screen":
		args = append(args, "-d", "-i")
	case "tasks":
		args = append(args, "-i")
	default:
		return nil, errors.New("invalid mode")
	}
	if owner <= 0 {
		return nil, errors.New("invalid owner")
	}
	if !s.End.IsZero() {
		remaining := s.End.Sub(now)
		if remaining <= 0 {
			return nil, errors.New("expired")
		}
		args = append(args, "-t", strconv.FormatInt(int64(math.Ceil(remaining.Seconds())), 10))
	}
	return append(args, "-w", strconv.Itoa(owner)), nil
}

func (k *awakeController) snapshot() awakeState { k.mu.Lock(); defer k.mu.Unlock(); return k.state }
func (k *awakeController) notify() {
	if k.changed != nil {
		k.changed()
	}
}
func (k *awakeController) stopLocked() {
	k.generation++
	if k.child != nil {
		_ = k.child.Kill()
		k.child = nil
	}
	k.childDone = nil
	k.state.Active = false
}

// stopConfirmed stops the ordinary assertion without touching the lid and
// reports whether the child process really exited. A caller that already
// restored the system sleep setting uses this instead of stop, so the lid
// restore is never repeated from beforeStop.
func (k *awakeController) stopConfirmed(timeout time.Duration) error {
	k.mu.Lock()
	child, done := k.child, k.childDone
	generation := k.generation
	k.state.Session.Lid = false
	k.mu.Unlock()
	if child != nil {
		if err := child.Kill(); err != nil {
			// Wait may already have observed an exit. Otherwise retain the
			// child and its active state so another close can retry the kill.
			select {
			case <-done:
			default:
				return errors.New("AwakeStopError")
			}
		}
		if done == nil {
			return errors.New("AwakeStopError")
		}
	}
	if done != nil {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			return errors.New("AwakeStopError")
		}
	}
	k.mu.Lock()
	if k.generation != generation {
		k.mu.Unlock()
		return errors.New("AwakeStopError")
	}
	k.generation++
	k.child, k.childDone = nil, nil
	k.state.Active = false
	k.state.Error = ""
	k.mu.Unlock()
	k.notify()
	return nil
}
func (k *awakeController) setBeforeStop(fn func()) { k.mu.Lock(); k.beforeStop = fn; k.mu.Unlock() }
func (k *awakeController) stop() {
	k.mu.Lock()
	before := k.beforeStop
	k.mu.Unlock()
	if before != nil {
		before()
	}
	k.mu.Lock()
	k.stopLocked()
	k.state.Error = ""
	k.mu.Unlock()
	k.notify()
}
func (k *awakeController) start(s awakeSession) error {
	args, err := awakeArgs(s, os.Getpid(), k.now())
	if err == nil && s.PID > 0 && !k.alive(s.PID, s.Identity) {
		err = errors.New("target ended")
	}
	k.mu.Lock()
	if err != nil {
		k.mu.Unlock()
		return err
	}
	k.stopLocked()
	child, err := k.launch(args)
	k.state = awakeState{Session: s, Active: err == nil}
	if err != nil {
		k.state.Error = "AwakeStartError"
		k.mu.Unlock()
		k.notify()
		return err
	}
	k.child = child
	done := make(chan struct{})
	k.childDone = done
	generation := k.generation
	k.mu.Unlock()
	k.notify()
	go func() {
		err := child.Wait()
		// Signal process exit before notification: the main thread may be
		// confirming this stop while changed is waiting for RunOnMain.
		close(done)
		k.mu.Lock()
		if generation != k.generation {
			k.mu.Unlock()
			return
		}
		k.child = nil
		k.state.Active = false
		if err != nil || s.End.IsZero() || k.now().Before(s.End) {
			k.state.Error = "AwakeExitError"
		}
		k.mu.Unlock()
		k.notify()
	}()
	return nil
}

// Poll also checks wall-clock deadlines after wake and guards against PID reuse.
// A deadline that passes ends normally; a target that went away is an error
// the panel shows, so the two are never folded into one branch.
func (k *awakeController) poll() {
	k.mu.Lock()
	s := k.state
	changed := false
	if s.Active {
		expired := !s.Session.End.IsZero() && !k.now().Before(s.Session.End)
		targetGone := s.Session.PID > 0 && !k.alive(s.Session.PID, s.Session.Identity)
		if expired || targetGone {
			if before := k.beforeStop; before != nil {
				k.mu.Unlock()
				before()
				k.mu.Lock()
			}
			k.stopLocked()
			if expired {
				k.state.Error = ""
			} else {
				k.state.Error = "ProcessEnded"
			}
			changed = true
		}
	}
	k.mu.Unlock()
	if changed {
		k.notify()
	}
}

func processIdentity(pid int) string {
	data, err := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "stat=,lstart=").Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(data))
	if len(fields) < 6 || strings.Contains(fields[0], "Z") {
		return ""
	}
	return strings.Join(fields[1:], " ")
}

// psOutput runs ps with an argument vector: never a shell, never a format
// string built from input. Tests replace it.
var psOutput = func(args ...string) ([]byte, error) {
	return exec.Command("/bin/ps", args...).Output()
}

// processByPID looks up one process for --follow, including daemons without a
// terminal: the picker's table drops those, so it cannot be reused here.
//
// The identity string must be byte for byte the one processIdentity builds,
// because awakeController.start compares them and would stop the session with
// "target ended" otherwise.
func processByPID(pid int) (runningProcess, bool) {
	if pid <= 0 {
		return runningProcess{}, false
	}
	data, err := psOutput("-p", strconv.Itoa(pid), "-o", "stat=,lstart=,comm=")
	if err != nil {
		return runningProcess{}, false
	}
	fields := strings.Fields(string(data))
	// stat, then the five words of lstart, then comm.
	if len(fields) < 7 || strings.Contains(fields[0], "Z") {
		return runningProcess{}, false
	}
	identity := strings.Join(fields[1:6], " ")
	name := filepath.Base(strings.Join(fields[6:], " "))
	if name == "" {
		name = strconv.Itoa(pid)
	}
	return runningProcess{PID: pid, Name: name, Identity: identity}, true
}

func processAlive(pid int, identity string) bool {
	return pid > 0 && identity != "" && syscall.Kill(pid, 0) == nil && processIdentity(pid) == identity
}

func restoreSession(s *awakeSession, now time.Time, alive func(int, string) bool) *awakeSession {
	if s == nil || (!s.End.IsZero() && !s.End.After(now)) || (s.PID > 0 && !alive(s.PID, s.Identity)) {
		return nil
	}
	copy := *s
	return &copy
}

func customDuration(hours, minutes string) (time.Duration, error) {
	h, e1 := strconv.Atoi(hours)
	m, e2 := strconv.Atoi(minutes)
	if e1 != nil || e2 != nil || h < 0 || h > 8760 || m < 0 || m > 59 || h+m == 0 {
		return 0, fmt.Errorf("invalid duration")
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}
