package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type awakeSession struct {
	Mode     string    `toml:"mode"`
	End      time.Time `toml:"end"`
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
	generation uint64
	launch     func([]string) (awakeChild, error)
	alive      func(int, string) bool
	now        func() time.Time
	changed    func()
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
	k.state.Active = false
}
func (k *awakeController) stop() {
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
	generation := k.generation
	k.mu.Unlock()
	k.notify()
	go func() {
		err := child.Wait()
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
func (k *awakeController) poll() {
	k.mu.Lock()
	s := k.state
	if s.Active && ((!s.Session.End.IsZero() && !k.now().Before(s.Session.End)) || (s.Session.PID > 0 && !k.alive(s.Session.PID, s.Session.Identity))) {
		k.stopLocked()
		k.state.Error = ""
	}
	changed := s.Active != k.state.Active
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

func untilTime(value string, now time.Time) (time.Time, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return time.Time{}, err
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, now.Location())
	if !end.After(now) {
		end = end.AddDate(0, 0, 1)
	}
	return end, nil
}

func customDuration(hours, minutes string) (time.Duration, error) {
	h, e1 := strconv.Atoi(hours)
	m, e2 := strconv.Atoi(minutes)
	if e1 != nil || e2 != nil || h < 0 || h > 8760 || m < 0 || m > 59 || h+m == 0 {
		return 0, fmt.Errorf("invalid duration")
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}
