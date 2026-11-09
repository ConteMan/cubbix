package lid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelperLogRotationAndUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "helper.log")
	logger, err := OpenHelperLog(path)
	if err != nil {
		t.Fatal(err)
	}
	w := logger.Writer().(*HelperLog)
	w.limit = 128
	for i := 0; i < 20; i++ {
		if _, err = w.Write([]byte(strings.Repeat("x", 40) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{path, path + ".1"} {
		info, err := os.Stat(p)
		if err != nil || info.Size() > 128 || info.Mode().Perm() != 0644 {
			t.Fatal(info, err)
		}
	}
	outside := filepath.Join(dir, "outside")
	if err = os.WriteFile(outside, []byte("unchanged"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenHelperLog(path); err == nil {
		t.Fatal("followed log symlink")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "unchanged" {
		t.Fatal("changed target")
	}
	unsafeDir := filepath.Join(dir, "unsafe")
	os.Mkdir(unsafeDir, 0755)
	os.Chmod(unsafeDir, 0777)
	if _, err = OpenHelperLog(filepath.Join(unsafeDir, "helper.log")); err == nil {
		t.Fatal("accepted writable log directory")
	}
}

func TestHelperLogsOnlyOperationalFields(t *testing.T) {
	var logs bytes.Buffer
	logf := func(format string, args ...any) { fmt.Fprintf(&logs, format+"\n", args...) }
	e, s, _, _ := fixture(t)
	e.Logf = logf
	s.badOne = true
	req := request("enable")
	req.Client = "secret-client"
	req.Reason = "secret-reason"
	req.ID = "secret-id"
	r := e.Handle(1, req)
	if r.OK || r.Code != "pmset-failed" || r.Detail != "enable-readback-failed" || s.v != 0 {
		t.Fatalf("rollback: %+v", r)
	}
	text := logs.String()
	for _, want := range []string{"op=enable", "code=pmset-failed", "state=off", "value=0", "detail=enable-readback-failed", "restore"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	for _, secret := range []string{req.ID, req.Client, req.Reason} {
		if strings.Contains(text, secret) {
			t.Fatal("leaked request content")
		}
	}
	logs.Reset()
	req.Op = "secret-invalid-op"
	e.Handle(1, req)
	if strings.Contains(logs.String(), req.Op) || !strings.Contains(logs.String(), "op=invalid") {
		t.Fatal(logs.String())
	}
}

func TestPMSetReadAndWriteDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		data, result string
		fail         bool
		value        int
	}{
		{"System-wide power settings:\n SleepDisabled\t\t0\nCurrently in use:\n sleep 1 (sleep prevented by test)", "ok", false, 0},
		{"System-wide power settings:\n SleepDisabled\t\t1\nCurrently in use:\n sleep 1", "ok", false, 1},
		{"SleepDisabled 2", "parse-failed", false, -1},
		{"SleepDisabled 0 extra", "parse-failed", false, -1},
		{"sleep 1", "parse-failed", false, -1},
		{"private-output", "command-failed", true, -1},
	} {
		t.Run(tc.result+tc.data[:min(5, len(tc.data))], func(t *testing.T) {
			var logs bytes.Buffer
			p := PMSet{Run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				if tc.fail {
					return nil, errors.New("private-error")
				}
				return []byte(tc.data), nil
			}, Logf: func(f string, a ...any) { fmt.Fprintf(&logs, f+"\n", a...) }}
			value, err := p.Read()
			if value != tc.value || (err != nil) != (tc.result != "ok") || !strings.Contains(logs.String(), "result="+tc.result) {
				t.Fatal(value, err, logs.String())
			}
			p.Set(1)
			p.Set(0)
			if !strings.Contains(logs.String(), "write value=1") || !strings.Contains(logs.String(), "write value=0") || strings.Contains(logs.String(), "private-") {
				t.Fatal(logs.String())
			}
		})
	}
}

func TestPollingLogsOnlyChangesAndErrors(t *testing.T) {
	var logs bytes.Buffer
	logf := func(f string, a ...any) { fmt.Fprintf(&logs, f+"\n", a...) }
	value, failed := 0, false
	p := &PMSet{Run: func(context.Context, string, ...string) ([]byte, error) {
		if failed {
			return nil, errors.New("private-output")
		}
		return []byte(fmt.Sprint("SleepDisabled ", value)), nil
	}, Logf: logf}
	e, _, _, now := fixture(t)
	e.Sleep, e.Logf = p, logf
	// Observation rides a 1s interval; advance the fake clock for each poll.
	ping := func() Response {
		*now = now.Add(ObserveInterval)
		return e.Handle(1, request("ping"))
	}
	ping()
	logs.Reset()
	for range 20 {
		ping()
	}
	if logs.Len() != 0 {
		t.Fatal("stable polling was noisy", logs.String())
	}
	value = 1
	ping()
	if strings.Count(logs.String(), "pmset read") != 1 || !strings.Contains(logs.String(), "op=ping") || !strings.Contains(logs.String(), "state=on-foreign") {
		t.Fatal(logs.String())
	}
	logs.Reset()
	for range 20 {
		ping()
	}
	if logs.Len() != 0 {
		t.Fatal(logs.String())
	}
	failed = true
	ping()
	if !strings.Contains(logs.String(), "command-failed") {
		t.Fatal(logs.String())
	}
	logs.Reset()
	failed = false
	ping()
	if !strings.Contains(logs.String(), "pmset read result=ok value=1") {
		t.Fatal("missing read recovery", logs.String())
	}
}

func TestPollingDeduplicatesFailuresAndRecovers(t *testing.T) {
	var logs bytes.Buffer
	logf := func(f string, a ...any) { fmt.Fprintf(&logs, f+"\n", a...) }
	for _, value := range []int{0, 1} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			failed, malformed := true, false
			p := &PMSet{Run: func(context.Context, string, ...string) ([]byte, error) {
				if failed {
					return nil, errors.New("read")
				}
				if malformed {
					return []byte("invalid"), nil
				}
				return []byte(fmt.Sprint("SleepDisabled ", value)), nil
			}, Logf: logf}
			e, _, _, now := fixture(t)
			e.Sleep, e.Logf = p, logf
			ping := func() Response {
				*now = now.Add(ObserveInterval)
				return e.Handle(1, request("ping"))
			}
			logs.Reset()
			ping()
			logs.Reset()
			for range 250 {
				ping()
			}
			if logs.Len() != 0 {
				t.Fatal("identical failure flooded logs", logs.String())
			}
			failed, malformed = false, true
			ping()
			if strings.Count(logs.String(), "result=parse-failed") != 1 {
				t.Fatal("changed read failure was not logged once", logs.String())
			}
			logs.Reset()
			malformed = false
			r := ping()
			want := "ok"
			if value == 1 {
				want = "foreign-setting"
			}
			if !r.OK || r.Code != want || r.Value == nil || *r.Value != value {
				t.Fatal("successful read retained failure", r)
			}
			if !strings.Contains(logs.String(), "pmset read result=ok") || !strings.Contains(logs.String(), "code="+want) {
				t.Fatal("recovery was not logged", logs.String())
			}
			logs.Reset()
			ping()
			if logs.Len() != 0 {
				t.Fatal("stable recovery was noisy", logs.String())
			}
		})
	}
}

func TestRepeatedNonOKResponsesLogOnlyTransitions(t *testing.T) {
	e, _, _, _ := fixture(t)
	var logs bytes.Buffer
	e.Logf = func(f string, a ...any) { fmt.Fprintf(&logs, f+"\n", a...) }
	r := request("ping")
	r.V++
	e.Handle(1, r)
	logs.Reset()
	for _, op := range []string{"ping", "hello", "status", "enable", "disable"} {
		r.Op = op
		for range 20 {
			e.Handle(1, r)
		}
	}
	if logs.Len() != 0 {
		t.Fatal("repeated non-ok response was noisy", logs.String())
	}
	r.V = Protocol
	r.ID = "!"
	e.Handle(1, r)
	if strings.Count(logs.String(), "code=bad-request") != 1 {
		t.Fatal("missing changed failure code", logs.String())
	}
}

func TestReadFailureClearsWithoutLosingTerminationCode(t *testing.T) {
	for _, code := range []string{"ok", "battery-low", "battery-unknown", "max-duration"} {
		t.Run(code, func(t *testing.T) {
			failed := true
			e, _, _, now := fixture(t)
			e.code = code
			e.Sleep = &PMSet{Run: func(context.Context, string, ...string) ([]byte, error) {
				if failed {
					return nil, errors.New("read")
				}
				return []byte("SleepDisabled 0"), nil
			}}
			ping := func() Response {
				*now = now.Add(ObserveInterval)
				return e.Handle(1, request("ping"))
			}
			if r := ping(); r.Code != "pmset-failed" {
				t.Fatal(r)
			}
			failed = false
			if r := ping(); r.Code != code || r.Value == nil || *r.Value != 0 {
				t.Fatal("read recovery lost the previous termination code", r)
			}
		})
	}
}

// Group 80 is inherited from an admin-owned parent on macOS. All operations
// below are injected; this test never changes real ownership or requires root.
func TestJournalInheritedGroupRepair(t *testing.T) {
	for _, tc := range []struct {
		name           string
		uid, gid, euid uint32
		wantCalls      int
		fail, wantErr  bool
	}{
		{"root-inherited-admin", 0, 80, 0, 1, false, false},
		{"root-wheel", 0, 0, 0, 0, false, false},
		{"foreign-owner", 501, 80, 0, 0, false, true},
		{"repair-failed", 0, 80, 0, 1, true, true},
		{"unprivileged-tests", 501, 80, 501, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := normalizeJournalGroup(tc.uid, tc.gid, tc.euid, func() error {
				calls++
				if tc.fail {
					return errors.New("chown failed")
				}
				return nil
			})
			if (err != nil) != tc.wantErr || calls != tc.wantCalls {
				t.Fatal(err, calls)
			}
		})
	}
}

func TestJournalRejectsUnsafeMetadataAndLinks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "journal")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session.json")
	journal := FileJournal{Path: path}
	if err := journal.Save(Journal{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Load(); err == nil {
		t.Fatal("accepted readable journal")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Load(); err == nil {
		t.Fatal("accepted hard-linked journal")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(alias, path); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Load(); err == nil {
		t.Fatal("followed journal symlink")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if err := (FileJournal{Path: filepath.Join(link, "other")}).Save(Journal{}); err == nil {
		t.Fatal("followed directory symlink")
	}
}
