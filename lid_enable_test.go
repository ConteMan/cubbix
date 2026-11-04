package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
	"github.com/ConteMan/cubbix/internal/lid"
)

type lidEnableReply struct {
	lidConnection
	reply lid.Response
}

func (c lidEnableReply) Call(r lid.Request) (lid.Response, error) {
	if r.Op == "enable" {
		return c.reply, nil
	}
	return c.lidConnection.Call(r)
}

type lidMarkerBlocked struct {
	lidConnection
	marker string
}

func (c lidMarkerBlocked) Call(r lid.Request) (lid.Response, error) {
	out, err := c.lidConnection.Call(r)
	if r.Op == "enable" && err == nil {
		if mkErr := os.Mkdir(c.marker, 0700); mkErr != nil {
			return out, mkErr
		}
	}
	return out, err
}

func TestLidEnableFailureDiagnosticsSurvivePolling(t *testing.T) {
	one, zero := 1, 0
	for _, tc := range []struct {
		name, code string
		reply      lid.Response
	}{
		{"read", "pmset-read-failed", lid.Response{}},
		{"helper-failure", "helper-pmset-failed", lid.Response{Code: "pmset-failed", State: "off", Value: &zero, Detail: "journal-save-failed"}},
		{"helper-busy", "helper-busy", lid.Response{Code: "busy", State: "on-ours", Value: &one}},
		{"state", "helper-invalid-enable-response", lid.Response{OK: true, Code: "ok", State: "off", Value: &one}},
		{"nil-value", "helper-invalid-enable-response", lid.Response{OK: true, Code: "ok", State: "on-ours"}},
		{"zero-value", "helper-invalid-enable-response", lid.Response{OK: true, Code: "ok", State: "on-ours", Value: &zero}},
		{"marker", "marker-write-failed", lid.Response{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, sleep, _, svc, conn := lidTestApp(t)
			if tc.name == "read" {
				a.lid.read = func() (int, error) { return -1, errors.New("read") }
			} else if tc.name == "marker" {
				a.lid.marker = filepath.Join(t.TempDir(), "blocked-marker")
				a.lid.dial = func() (lidConnection, error) { return lidMarkerBlocked{conn, a.lid.marker}, nil }
			} else {
				reply := tc.reply
				reply.Helper = lid.Version
				a.lid.dial = func() (lidConnection, error) { return lidEnableReply{conn, reply}, nil }
			}
			if err := a.lid.enable(time.Time{}, 20, true); err == nil || err.Error() != "LidEnableFailed" {
				t.Fatal(err)
			}
			before := a.cliState().Awake.Lid
			if before.LastCode != tc.code || svc.registers != 0 {
				t.Fatalf("diagnostics: %+v", before)
			}
			a.lid.Inspect()
			after := a.cliState().Awake.Lid
			if after.LastCode != before.LastCode || after.HelperCode != before.HelperCode || after.HelperDetail != before.HelperDetail {
				t.Fatalf("poll erased failure: before=%+v after=%+v", before, after)
			}
			if tc.name == "marker" && sleep.value != 0 {
				t.Fatal("marker failure left setting enabled")
			}
		})
	}
}

func TestLidOldHelperRequiresManualUpdate(t *testing.T) {
	a, sleep, _, svc, conn := lidTestApp(t)
	conn.helloVersion = "1"
	a.lid.Inspect()
	if a.lidReason() != "LidHelperUpdate" || a.lid.snapshot().state != lidBlocked || svc.registers != 0 {
		t.Fatal(a.lid.snapshot())
	}
	if err := a.lid.enable(time.Time{}, 20, false); err == nil || err.Error() != "LidHelperUpdate" {
		t.Fatal(err)
	}
	s := a.cliState().Awake.Lid
	if s.Hello != "version-mismatch" || s.HelperVersion != "1" || s.LastCode != "helper-version-mismatch" || len(sleep.writes) != 0 || svc.registers != 0 {
		t.Fatalf("quiet update: %+v", s)
	}
	svc.onRegister = func() {
		if a.lid.snapshot().reason != "LidHelperUpdate" || sleep.value != 0 {
			t.Fatal("update lacked explanation or recovery")
		}
		conn.helloVersion = ""
	}
	if err := a.lid.enable(time.Time{}, 20, true); err != nil {
		t.Fatal(err)
	}
	if svc.registers != 1 || sleep.value != 1 || a.cliState().Awake.Lid.LastCode != "" || a.cliState().Awake.Lid.Hello != "ok" {
		t.Fatal("update did not enable")
	}
}

func TestLidBusyHasDiagnostic(t *testing.T) {
	a, _, _, _, _ := lidTestApp(t)
	a.lidBusy = true
	yes := true
	result := a.applyCLI(cli.Request{Op: cli.OpRun, Action: cli.ActionAwake, Arg: "2m", Lid: &yes}, nil)
	if result.OK || result.Code != "LidEnableFailed" || result.State.Awake.Lid.LastCode != "lid-busy" {
		t.Fatalf("busy: %+v", result)
	}
}

func TestCLIServerWaitsForDeferredReply(t *testing.T) {
	callback := make(chan func(cli.Result), 1)
	s := &cliServer{dispatch: func(run func()) { run() }, applyAsync: func(_ cli.Request, _ []byte, reply func(cli.Result)) { callback <- reply }}
	done := make(chan cli.Result, 1)
	go func() {
		result, ran, finished := s.applyReq(cli.Request{}, nil)
		<-finished
		if !ran {
			result = cli.Failure("not-ran")
		}
		done <- result
	}()
	reply := <-callback
	select {
	case r := <-done:
		t.Fatalf("early reply: %+v", r)
	default:
	}
	reply(cli.Failure("LidEnableFailed"))
	select {
	case r := <-done:
		if r.OK || r.Code != "LidEnableFailed" {
			t.Fatal(r)
		}
	case <-time.After(time.Second):
		t.Fatal("completion not delivered")
	}
}

func TestCLIAsyncHelperFailureAndUpdate(t *testing.T) {
	for _, failure := range []string{"pmset", "update"} {
		t.Run(failure, func(t *testing.T) {
			a, _, _, svc, conn := lidTestApp(t)
			a.lidAsync = true
			if failure == "update" {
				conn.helloVersion = "1"
			} else {
				zero := 0
				a.lid.dial = func() (lidConnection, error) {
					return lidEnableReply{conn, lid.Response{Helper: lid.Version, Code: "pmset-failed", State: "off", Value: &zero, Detail: "journal-save-failed"}}, nil
				}
			}
			callbacks := make(chan func(), 1)
			a.urlDispatch = func(run func()) { callbacks <- run }
			results := make(chan cli.Result, 1)
			yes := true
			a.applyCLIAsync(cli.Request{Op: cli.OpRun, Action: cli.ActionAwake, Arg: "2m", Lid: &yes}, nil, func(r cli.Result) { results <- r })
			select {
			case r := <-results:
				t.Fatalf("early reply: %+v", r)
			default:
			}
			select {
			case run := <-callbacks:
				run()
			case <-time.After(time.Second):
				t.Fatal("no completion")
			}
			r := <-results
			want := "LidEnableFailed"
			if failure == "update" {
				want = "LidHelperUpdate"
			}
			if r.OK || r.Code != want || svc.registers != 0 || a.awake.snapshot().Session.Lid || r.State.Awake.Lid.LastCode == "" {
				t.Fatalf("failure: %+v", r)
			}
			if !a.awake.snapshot().Active {
				t.Fatal("lid failure stopped ordinary awake")
			}
			if failure == "pmset" && r.State.Awake.Lid.HelperDetail != "journal-save-failed" {
				t.Fatal("lost helper diagnosis")
			}
		})
	}
}
