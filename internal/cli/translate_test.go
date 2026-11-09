package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// TestCLITranslateFrames pins the request/response frame rules of the new
// action: a body frame is allowed, --notify is a usage error, and the JSON
// line stays below MaxLine even with a full state snapshot.
func TestCLITranslateFrames(t *testing.T) {
	cases := []struct {
		name string
		req  Request
		want string
	}{
		{"body", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionTranslate, Bytes: 22, To: TranslateToAuto}, ""},
		{"empty body", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionTranslate}, ""},
		{"to zh", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionTranslate, Bytes: 1, To: TranslateToZh}, ""},
		{"to en", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionTranslate, Bytes: 1, To: TranslateToEn}, ""},
		{"notify", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionTranslate, Bytes: 1, Notify: true}, CodeBadRequest},
		{"bad to", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionTranslate, Bytes: 1, To: "fr"}, CodeBadRequest},
		{"over limit", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionTranslate, Bytes: MaxPayload + 1}, CodeBadRequest},
		{"negative bytes", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionTranslate, Bytes: -1}, CodeBadRequest},
		{"arg", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionTranslate, Bytes: 1, Arg: "hi"}, CodeBadRequest},
		{"page", Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionTranslate, Page: "general"}, CodeBadRequest},
	}
	for _, scenario := range cases {
		if got := scenario.req.Validate(MaxPayload); got != scenario.want {
			t.Fatalf("%s: Validate = %q, want %q", scenario.name, got, scenario.want)
		}
	}
	// Other actions keep their old rules unchanged.
	if got := (Request{V: Protocol, ID: "a", Op: OpRun, Action: ActionJSONNew, Bytes: MaxPayload}).Validate(MaxPayload); got != "" {
		t.Fatalf("json.new regressed: %q", got)
	}

	translation := strings.Repeat("译", 4000)
	response := Response{V: Protocol, ID: "a", OK: true, Bytes: len(translation),
		State: &State{Version: "0.6.1", Tiles: Tiles{Awake: true, JSON: true},
			JSON: JSONState{Windows: 2}},
		Translate: &TranslateResult{Source: "auto", Target: "zh-Hans", Model: "group/model", Clipped: 7},
	}
	line, err := EncodeLine(response)
	if err != nil {
		t.Fatalf("a 4000-character answer does not fit the JSON line: %v", err)
	}
	if len(line) > MaxLine {
		t.Fatalf("JSON line = %d bytes", len(line))
	}
	var back Response
	if err := json.Unmarshal(line[:len(line)-1], &back); err != nil {
		t.Fatal(err)
	}
	if back.Bytes != len(translation) || back.Translate == nil || back.Translate.Clipped != 7 || back.State == nil {
		t.Fatalf("round trip = %+v", back)
	}
	if bytes.Contains(line, []byte(`"text"`)) {
		t.Fatal("the app serialized translate.text into the JSON line")
	}
	result := back.Result()
	if result.Bytes != back.Bytes || result.Translate != back.Translate || result.State != back.State {
		t.Fatal("Response.Result() dropped a translate field")
	}
}

// TestCLITranslateHumanJSON checks the stable --json shape with the text the
// client filled in locally. The text appears exactly once, in translate.text.
func TestCLITranslateHumanJSON(t *testing.T) {
	text := "你好，世界"
	result := Result{V: Protocol, OK: true, State: &State{Version: "0.6.1"},
		Bytes: len(text), Translate: &TranslateResult{Source: "auto", Target: "zh-Hans", Model: "m", Text: text}}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), text) != 1 {
		t.Fatalf("translation not in one stable field: %s", data)
	}
	var back Result
	if err := json.Unmarshal(data, &back); err != nil || back.Translate.Text != text || back.Bytes != len(text) {
		t.Fatalf("round trip = %+v %v", back, err)
	}
}

// TestCLITranslateResponseBodyFrame drives a real socket pair: 4000 Chinese
// characters plus a full state must arrive intact, and both frame limits are
// enforced by the client.
func TestCLITranslateResponseBodyFrame(t *testing.T) {
	path := shortPath(t)
	ln := listener(t, path)
	translation := strings.Repeat("你", 4000)
	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			return
		}
		body := make([]byte, req.Bytes)
		if req.Bytes > 0 {
			if _, err := io.ReadFull(reader, body); err != nil {
				return
			}
		}
		response := Response{V: Protocol, ID: req.ID, OK: true, Bytes: len(translation),
			State:     &State{Version: "0.6.1", Tiles: Tiles{Awake: true, JSON: true}},
			Translate: &TranslateResult{Source: "auto", Target: "zh-Hans", Model: "m"}}
		data, err := EncodeLine(response)
		if err != nil {
			return
		}
		_, _ = conn.Write(data)
		_, _ = conn.Write([]byte(translation))
	}()
	client, err := Dial(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.Do(Request{V: Protocol, ID: "abc", Op: OpRun, Action: ActionTranslate,
		Bytes: 5, To: TranslateToAuto}, []byte("hello"), 3*time.Second)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if !result.OK || result.State == nil || result.Translate == nil {
		t.Fatalf("result = %+v", result)
	}
	if result.Bytes != len(translation) || result.Translate.Text != translation {
		t.Fatalf("body = %d bytes, %d runes; want %d", result.Bytes, len([]rune(result.Translate.Text)), len(translation))
	}
}

func TestCLITranslateBodyFrameGuards(t *testing.T) {
	if _, err := readBodyFrame(bufio.NewReader(strings.NewReader("")), MaxTranslateBody+1); err == nil {
		t.Fatal("an oversized body frame was accepted")
	}
	if _, err := readBodyFrame(bufio.NewReader(bytes.NewReader([]byte{0xff})), 1); err == nil {
		t.Fatal("invalid UTF-8 was accepted")
	}
	if _, err := readBodyFrame(bufio.NewReader(strings.NewReader("abc")), 4); err == nil {
		t.Fatal("a truncated body frame was accepted")
	}
	body, err := readBodyFrame(bufio.NewReader(strings.NewReader("你好")), len("你好"))
	if err != nil || string(body) != "你好" {
		t.Fatalf("body = %q %v", body, err)
	}
}

// TestCLITranslateUnexpectedBody keeps a non-translate response byte-for-byte:
// Bytes must never appear without the translate metadata.
func TestCLITranslateUnexpectedBody(t *testing.T) {
	path := shortPath(t)
	ln := listener(t, path)
	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = bufio.NewReader(conn).ReadBytes('\n')
		data, _ := EncodeLine(Response{V: Protocol, ID: "abc", OK: true, Bytes: 3})
		_, _ = conn.Write(data)
	}()
	client, err := Dial(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Do(Request{V: Protocol, ID: "abc", Op: OpStatus}, nil, time.Second); CodeOf(err) != CodeInternal {
		t.Fatalf("orphan body = %v", err)
	}
}

func TestTranslateResponseValidation(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		action   string
		response Response
		body     string
		code     string
	}{
		{"negative length", ActionTranslate, Response{OK: true, Bytes: -1, Translate: &TranslateResult{}}, "", CodeInternal},
		{"missing metadata", ActionTranslate, Response{OK: true}, "", CodeInternal},
		{"inline text", ActionTranslate, Response{OK: true, Translate: &TranslateResult{Text: "inline"}}, "", CodeInternal},
		{"oversized", ActionTranslate, Response{OK: true, Bytes: MaxTranslateBody + 1, Translate: &TranslateResult{}}, "", CodeInternal},
		{"invalid UTF-8", ActionTranslate, Response{OK: true, Bytes: 1, Translate: &TranslateResult{}}, "\xff", CodeInternal},
		{"short body", ActionTranslate, Response{OK: true, Bytes: 4, Translate: &TranslateResult{}}, "abc", CodeNotRunning},
		{"metadata on status", "", Response{OK: true, Translate: &TranslateResult{}}, "", CodeInternal},
		{"negative status length", "", Response{OK: true, Bytes: -1}, "", CodeInternal},
		{"body on error", ActionTranslate, Response{Code: CodeBusy, Bytes: 3, Translate: &TranslateResult{}}, "abc", CodeInternal},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			client := &Client{r: bufio.NewReader(strings.NewReader(scenario.body))}
			result := scenario.response.Result()
			if err := client.readTranslationBody(Request{Action: scenario.action}, &result); CodeOf(err) != scenario.code {
				t.Fatalf("response validation = %v, want %s", err, scenario.code)
			}
		})
	}
}

func TestTranslateFragmentedResponse(t *testing.T) {
	reader, writer := io.Pipe()
	text := strings.Repeat("你🙂", MaxTranslateBody/len("你🙂"))
	go func() {
		defer writer.Close()
		for _, octet := range []byte(text) {
			if _, err := writer.Write([]byte{octet}); err != nil {
				return
			}
		}
	}()
	defer reader.Close()
	body, err := readBodyFrame(bufio.NewReader(reader), len(text))
	if err != nil || string(body) != text {
		t.Fatalf("fragmented response length=%d, error=%v", len(body), err)
	}
	_, err = readBodyFrame(bufio.NewReader(strings.NewReader("你")), 2)
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatal("a frame ending inside a UTF-8 rune was accepted")
	}
}

func TestTranslateOldAppError(t *testing.T) {
	line, err := EncodeLine(Response{V: Protocol, ID: "old", Code: CodeBadRequest})
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{r: bufio.NewReader(bytes.NewReader(line))}
	result, err := client.readResponse(Request{ID: "old", Action: ActionTranslate})
	if err != nil || result.Code != CodeBadRequest || !strings.Contains(result.Message, "Update and restart") {
		t.Fatalf("old app answer = %+v, %v", result, err)
	}
}
