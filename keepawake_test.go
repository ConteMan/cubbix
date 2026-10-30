package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeAwakeChild struct {
	done chan error
	once sync.Once
}

func (f *fakeAwakeChild) Wait() error { return <-f.done }
func (f *fakeAwakeChild) Kill() error {
	f.once.Do(func() { f.done <- errors.New("killed") })
	return nil
}
func fakeAwake(now time.Time) (*awakeController, *fakeAwakeChild) {
	k := newAwakeController()
	f := &fakeAwakeChild{done: make(chan error, 1)}
	k.now = func() time.Time { return now }
	k.alive = func(int, string) bool { return true }
	first := true
	k.launch = func([]string) (awakeChild, error) {
		if first {
			first = false
			return f, nil
		}
		return &fakeAwakeChild{done: make(chan error, 1)}, nil
	}
	return k, f
}
func eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}
func TestAwakeArguments(t *testing.T) {
	now := time.Now()
	for _, mode := range []string{"screen", "tasks"} {
		for _, duration := range []time.Duration{0, 30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 90 * time.Minute} {
			s := awakeSession{Mode: mode, PID: 321, Identity: "target"}
			if duration > 0 {
				s.End = now.Add(duration)
			}
			got, err := awakeArgs(s, 123, now)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"-i"}
			if mode == "screen" {
				want = []string{"-d", "-i"}
			}
			if duration > 0 {
				want = append(want, "-t", strconv.Itoa(int(duration.Seconds())))
			}
			want = append(want, "-w", "123")
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("args=%v want %v", got, want)
			}
		}
	}
	for _, s := range []awakeSession{{Mode: "invalid"}, {Mode: "screen", End: now.Add(-time.Second)}} {
		if _, err := awakeArgs(s, 123, now); err == nil {
			t.Fatal("invalid session accepted")
		}
	}
}
func TestAwakeDeadlineFollowAndCrash(t *testing.T) {
	now := time.Now()
	t.Run("deadline", func(t *testing.T) {
		k, _ := fakeAwake(now)
		if err := k.start(awakeSession{Mode: "screen", End: now.Add(time.Second)}); err != nil {
			t.Fatal(err)
		}
		k.now = func() time.Time { return now.Add(2 * time.Second) }
		k.poll()
		if s := k.snapshot(); s.Active || s.Error != "" {
			t.Fatal(s)
		}
	})
	t.Run("follow ends or PID reused", func(t *testing.T) {
		k, _ := fakeAwake(now)
		if err := k.start(awakeSession{Mode: "tasks", PID: 12, Identity: "original"}); err != nil {
			t.Fatal(err)
		}
		k.alive = func(int, string) bool { return false }
		k.poll()
		// A target that went away is an error the panel shows; a deadline
		// that passed is not.
		if s := k.snapshot(); s.Active || s.Error != "ProcessEnded" {
			t.Fatal(s)
		}
	})
	t.Run("unexpected exit", func(t *testing.T) {
		for _, exit := range []error{nil, errors.New("crash")} {
			k, f := fakeAwake(now)
			if err := k.start(awakeSession{Mode: "tasks"}); err != nil {
				t.Fatal(err)
			}
			f.done <- exit
			eventually(t, time.Second, func() bool { return !k.snapshot().Active })
			if k.snapshot().Error != "AwakeExitError" {
				t.Fatal(k.snapshot())
			}
		}
	})
	t.Run("stale child", func(t *testing.T) {
		k, first := fakeAwake(now)
		_ = k.start(awakeSession{Mode: "screen"})
		second := &fakeAwakeChild{done: make(chan error, 1)}
		k.launch = func([]string) (awakeChild, error) { return second, nil }
		_ = k.start(awakeSession{Mode: "tasks"})
		_ = first
		time.Sleep(20 * time.Millisecond)
		if !k.snapshot().Active || k.snapshot().Error != "" {
			t.Fatal(k.snapshot())
		}
		k.stop()
	})
	t.Run("launch failure", func(t *testing.T) {
		k, _ := fakeAwake(now)
		k.launch = func([]string) (awakeChild, error) { return nil, errors.New("failed") }
		if k.start(awakeSession{Mode: "screen"}) == nil || k.snapshot().Active || k.snapshot().Error == "" {
			t.Fatal(k.snapshot())
		}
	})
}
func TestAwakeSessionRestore(t *testing.T) {
	now := time.Now()
	s := &awakeSession{Mode: "tasks", End: now.Add(10 * time.Minute), PID: 12, Identity: "original"}
	restored := restoreSession(s, now.Add(3*time.Minute), func(int, string) bool { return true })
	if restored == nil || restored.End.Sub(now.Add(3*time.Minute)) != 7*time.Minute {
		t.Fatal(restored)
	}
	if restoreSession(s, now.Add(11*time.Minute), func(int, string) bool { return true }) != nil {
		t.Fatal("expired session restored")
	}
	if restoreSession(s, now, func(int, string) bool { return false }) != nil {
		t.Fatal("ended target restored")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	config := defaultConfig()
	config.AwakeSession = s
	config.RestoreAwake = true
	if err := saveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || !loaded.RestoreAwake || (loaded.AwakeSession == nil || !loaded.AwakeSession.End.Equal(s.End) || loaded.AwakeSession.PID != s.PID || loaded.AwakeSession.Identity != s.Identity || loaded.AwakeSession.Mode != s.Mode) {
		t.Fatalf("round trip: loaded=%+v want=%+v %v", loaded.AwakeSession, s, err)
	}
}
func TestAwakeTimeInputs(t *testing.T) {
	now := time.Date(2026, 10, 29, 23, 0, 0, 0, time.Local)
	end, err := untilTime("02:30", now)
	if err != nil || end.Day() != 30 || end.Sub(now) != 210*time.Minute {
		t.Fatal(end, err)
	}
	if _, err := untilTime("25:00", now); err == nil {
		t.Fatal("invalid time accepted")
	}
	if d, err := customDuration("1", "30"); err != nil || d != 90*time.Minute {
		t.Fatal(d, err)
	}
	for _, pair := range [][2]string{{"0", "0"}, {"-1", "30"}, {"1", "60"}, {"abc", "1"}, {"8761", "0"}} {
		if _, err := customDuration(pair[0], pair[1]); err == nil {
			t.Fatal(pair)
		}
	}
}
func assertions(t *testing.T) string {
	t.Helper()
	data, err := exec.Command("/usr/bin/pmset", "-g", "assertions").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func hasAssertion(output string, pid int) bool {
	return strings.Contains(output, "pid "+strconv.Itoa(pid)+"(")
}

// A short-lived owner subprocess exercises exactly the production -w policy.
func TestAwakeOwnerHelper(t *testing.T) {
	if os.Getenv("CUBBIX_AWAKE_OWNER_HELPER") != "1" {
		t.Skip("owner subprocess only")
	}
	args, err := awakeArgs(awakeSession{Mode: "screen", End: time.Now().Add(3 * time.Second)}, os.Getpid(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/usr/bin/caffeinate", args...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("CUBBIX_AWAKE_PID_FILE"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
}
func TestAwakeOwnerSIGKILLReleasesAssertions(t *testing.T) {
	if _, err := os.Stat("/usr/bin/caffeinate"); err != nil {
		t.Skip("macOS only")
	}
	path := filepath.Join(t.TempDir(), "pid")
	owner := exec.Command(os.Args[0], "-test.run=^TestAwakeOwnerHelper$")
	owner.Env = append(os.Environ(), "CUBBIX_AWAKE_OWNER_HELPER=1", "CUBBIX_AWAKE_PID_FILE="+path)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Process.Kill(); _ = owner.Wait() }()
	var pid int
	eventually(t, time.Second, func() bool { data, _ := os.ReadFile(path); pid, _ = strconv.Atoi(string(data)); return pid > 0 })
	eventually(t, time.Second, func() bool { return hasAssertion(assertions(t), pid) })
	if err := owner.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	eventually(t, 1500*time.Millisecond, func() bool { return !hasAssertion(assertions(t), pid) })
	t.Log("SIGKILL owner: this caffeinate PID's assertions were released")
}
func TestAwakeRealModesStopAndExpiry(t *testing.T) {
	for _, mode := range []string{"screen", "tasks"} {
		k := newAwakeController()
		defer k.stop()
		if err := k.start(awakeSession{Mode: mode, End: time.Now().Add(time.Second)}); err != nil {
			t.Fatal(err)
		}
		k.mu.Lock()
		pid := k.child.(commandChild).Process.Pid
		k.mu.Unlock()
		eventually(t, time.Second, func() bool { return hasAssertion(assertions(t), pid) })
		if mode == "screen" {
			k.stop()
		} else {
			eventually(t, 2*time.Second, func() bool { return !k.snapshot().Active })
		}
		eventually(t, time.Second, func() bool { return !hasAssertion(assertions(t), pid) })
		if k.snapshot().Error != "" {
			t.Fatal(k.snapshot())
		}
	}
}
func TestPowerTransitionUsesReplaceableSender(t *testing.T) {
	var calls int
	p := powerMonitor{notify: func() { calls++ }}
	p.sample(true, true)
	p.sample(false, true)
	p.sample(true, true)
	p.sample(true, true)
	p.sample(false, false)
	p.sample(true, false)
	if calls != 1 {
		t.Fatalf("notifications=%d", calls)
	}
}

func TestRealFollowStopsAndReleasesAssertions(t *testing.T) {
	target := exec.Command("/bin/sleep", "2")
	if err := target.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = target.Process.Kill(); _ = target.Wait() }()
	identity := processIdentity(target.Process.Pid)
	if identity == "" || !processAlive(target.Process.Pid, identity) || processAlive(target.Process.Pid, "different start") {
		t.Fatal("process identity validation failed")
	}
	k := newAwakeController()
	defer k.stop()
	if err := k.start(awakeSession{Mode: "tasks", PID: target.Process.Pid, Identity: identity, End: time.Now().Add(3 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	k.mu.Lock()
	pid := k.child.(commandChild).Process.Pid
	k.mu.Unlock()
	eventually(t, time.Second, func() bool { return hasAssertion(assertions(t), pid) })
	_ = target.Process.Kill()
	_ = target.Wait()
	k.poll()
	if k.snapshot().Active {
		t.Fatal("follow session survived target exit")
	}
	eventually(t, time.Second, func() bool { return !hasAssertion(assertions(t), pid) })
}
