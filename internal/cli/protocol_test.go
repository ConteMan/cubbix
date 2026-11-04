package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRequestValidate(t *testing.T) {
	long := make([]byte, MaxArg+1)
	for i := range long {
		long[i] = 'a'
	}
	valid := Request{V: Protocol, ID: "abc", Op: OpRun, Action: ActionAwake}
	cases := []struct {
		name string
		req  Request
		want string
	}{
		{"status", Request{V: Protocol, ID: "abc", Op: OpStatus}, ""},
		{"awake", valid, ""},
		{"awake arg", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionAwake, Arg: "1h30m"}, ""},
		{"awake follow", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionAwake, Follow: 42}, ""},
		{"json body", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionJSONNew, Bytes: 3}, ""},
		{"version", Request{V: 2, ID: "abc", Op: OpStatus}, CodeProtocolMismatch},
		{"empty id", Request{V: Protocol, Op: OpStatus}, CodeBadRequest},
		{"bad id", Request{V: Protocol, ID: "a b", Op: OpStatus}, CodeBadRequest},
		{"long id", Request{V: Protocol, ID: string(make([]byte, 65)), Op: OpStatus}, CodeBadRequest},
		{"unknown op", Request{V: Protocol, ID: "a", Op: "eval"}, CodeBadRequest},
		{"status with action", Request{V: Protocol, ID: "a", Op: OpStatus, Action: ActionAwake}, CodeBadRequest},
		{"status with body", Request{V: Protocol, ID: "a", Op: OpStatus, Bytes: 1}, CodeBadRequest},
		{"run without action", Request{V: Protocol, ID: "a", Op: OpRun}, CodeBadRequest},
		{"unknown action", Request{V: Protocol, ID: "a", Op: OpRun, Action: "shell"}, CodeBadRequest},
		{"body on awake", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionAwake, Bytes: 1}, CodeBadRequest},
		{"negative body on stop", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionAwakeStop, Bytes: -1}, CodeBadRequest},
		{"negative body on settings", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionSettings, Bytes: -1}, CodeBadRequest},
		{"negative body", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionJSONNew, Bytes: -1}, CodeBadRequest},
		{"body over limit", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionJSONNew, Bytes: MaxPayload + 1}, CodeBadRequest},
		{"bad mode", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionAwake, Mode: "lid"}, CodeBadRequest},
		{"negative follow", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionAwake, Follow: -1}, CodeBadRequest},
		{"follow with arg", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionAwake, Follow: 3, Arg: "2h"}, CodeBadRequest},
		{"arg too long", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionAwake, Arg: string(long)}, CodeBadRequest},
		{"arg on stop", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionAwakeStop, Arg: "2h"}, CodeBadRequest},
		{"page too long", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionSettings, Page: string(make([]byte, 33))}, CodeBadRequest},
	}
	for _, scenario := range cases {
		if got := scenario.req.Validate(MaxPayload); got != scenario.want {
			t.Fatalf("%s: Validate = %q, want %q", scenario.name, got, scenario.want)
		}
	}
}

// The golden bytes pin the --json contract: fields only grow, never change.
func TestResultGolden(t *testing.T) {
	end, remaining := "2026-11-04T12:30:00Z", 3599
	state := &State{
		Version: "0.4.0",
		Tiles:   Tiles{Awake: true, JSON: false},
		Awake: AwakeState{
			Active: true, Mode: "screen", Minutes: 60, Until: "",
			End: &end, RemainingSeconds: &remaining,
			Follow: &Follow{PID: 42, Name: "codex"},
			Lid:    LidState{Capable: true, Available: false, State: "on", BatteryMin: 20},
		},
		JSON: JSONState{Windows: 2},
	}
	result := Result{V: Protocol, OK: true, Message: "keep", State: state}
	want := `{"v":1,"ok":true,"code":"","message":"keep","state":{"version":"0.4.0","tiles":{"awake":true,"json":false},"awake":{"active":true,"mode":"screen","minutes":60,"until":"","end":"2026-11-04T12:30:00Z","remainingSeconds":3599,"follow":{"pid":42,"name":"codex"},"lid":{"capable":true,"available":false,"state":"on","reason":"","batteryMin":20},"error":""},"json":{"windows":2}}}`
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("golden changed:\n got %s\nwant %s", data, want)
	}
	for _, code := range []string{CodeUsage, CodeDisabled, CodeQuitting} {
		failure := Failure(code)
		want := `{"v":1,"ok":false,"code":"` + code + `","message":"","state":null}`
		data, err := json.Marshal(failure)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Fatalf("%s golden changed:\n got %s\nwant %s", code, data, want)
		}
	}
}

func TestResponseCarriesID(t *testing.T) {
	response := Response{V: Protocol, ID: "9f2c", OK: true}
	data, err := EncodeLine(response)
	if err != nil {
		t.Fatal(err)
	}
	if data[len(data)-1] != '\n' {
		t.Fatal("a response must end with a newline")
	}
	var back Response
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.ID != "9f2c" || back.V != Protocol || !back.OK {
		t.Fatalf("round trip = %+v", back)
	}
	if result := back.Result(); result.V != Protocol || !result.OK {
		t.Fatalf("result = %+v", result)
	}
}

func TestEncodeLineRefusesOverlong(t *testing.T) {
	long := make([]string, 0, 1)
	long = append(long, string(make([]byte, MaxLine)))
	if _, err := EncodeLine(Response{V: Protocol, Lines: long}); err == nil {
		t.Fatal("an over-long response was encoded")
	}
}

func TestFormatEnd(t *testing.T) {
	if FormatEnd(time.Time{}) != nil {
		t.Fatal("a zero deadline must be null")
	}
	value := FormatEnd(time.Date(2026, 11, 4, 20, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	if value == nil || *value != "2026-11-04T12:30:00Z" {
		t.Fatalf("end = %v", value)
	}
}
