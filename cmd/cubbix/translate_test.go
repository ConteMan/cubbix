package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
)

type translateStubConn struct {
	result cli.Result
	err    error
	req    cli.Request
	body   []byte
}

func (c *translateStubConn) Do(req cli.Request, body []byte, _ time.Duration) (cli.Result, error) {
	c.req, c.body = req, append([]byte(nil), body...)
	return c.result, c.err
}

func (c *translateStubConn) Close() error { return nil }

func translateEnv(t *testing.T, result cli.Result, stdin string, terminal bool) (*env, *bytes.Buffer, *bytes.Buffer, *translateStubConn) {
	t.Helper()
	stub := &translateStubConn{result: result}
	e, stdout, stderr, _ := testEnv(t, func(string, time.Duration) (conn, error) { return stub, nil })
	e.stdin = func() (io.Reader, bool) { return strings.NewReader(stdin), terminal }
	return e, stdout, stderr, stub
}

func TestParseTranslate(t *testing.T) {
	inv, err := parse([]string{"translate", "Hello iOS"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.command != "translate" || inv.textArg != "Hello iOS" || inv.to != "" || inv.flags.timeout != 30*time.Second {
		t.Fatalf("translate = %+v", inv)
	}
	inv, err = parse([]string{"translate", "--to=zh", "--text", "你好", "--json"})
	if err != nil || inv.to != "zh" || inv.body.text != "你好" || !inv.flags.json || inv.flags.timeout != 30*time.Second {
		t.Fatalf("translate options = %+v %v", inv, err)
	}
	inv, err = parse([]string{"translate", "--timeout", "60s", "Hi"})
	if err != nil || inv.flags.timeout != 60*time.Second {
		t.Fatalf("--timeout 60s = %+v %v", inv, err)
	}
	inv, err = parse([]string{"translate", "-"})
	if err != nil || inv.textArg != "-" {
		t.Fatalf("- = %+v %v", inv, err)
	}
	inv, err = parse([]string{"help", "translate"})
	if err != nil || inv.command != "help" || inv.helpTopic != "translate" {
		t.Fatalf("help translate = %+v %v", inv, err)
	}
	for _, args := range [][]string{
		{"translate", "--notify"},
		{"translate", "--blank"},
		{"translate", "--to", "fr"},
		{"translate", "--minutes", "5"},
		{"translate", "--page", "general"},
		{"translate", "--timeout", "61s"},
		{"translate", "one", "two"},
		{"help", "translate", "extra"},
	} {
		if _, err := parse(args); err == nil {
			t.Fatalf("%v was accepted", args)
		} else if _, ok := err.(*usageError); !ok {
			t.Fatalf("%v: not a usage error: %v", args, err)
		}
	}
}

func TestTranslateBuildRequestBody(t *testing.T) {
	cases := []struct {
		args     []string
		stdin    string
		terminal bool
		want, to string
	}{
		{[]string{"translate", "Hello iOS"}, "", true, "Hello iOS", ""},
		{[]string{"translate", "--text", "Hi", "--to", "zh"}, "", true, "Hi", "zh"},
		{[]string{"translate", "--to=en", "Hi"}, "", true, "Hi", "en"},
		{[]string{"translate"}, "piped\n", false, "piped\n", ""},
		{[]string{"translate", "-"}, "piped", false, "piped", ""},
		{[]string{"translate", "--file", "-"}, "from file", false, "from file", ""},
	}
	for _, scenario := range cases {
		inv, err := parse(scenario.args)
		if err != nil {
			t.Fatalf("%v: %v", scenario.args, err)
		}
		e, _, _, _ := testEnv(t, nil)
		e.stdin = func() (io.Reader, bool) { return strings.NewReader(scenario.stdin), scenario.terminal }
		req, body, err := buildRequest(inv, e)
		if err != nil {
			t.Fatalf("%v: %v", scenario.args, err)
		}
		if req.Action != cli.ActionTranslate || req.Op != cli.OpRun || req.To != scenario.to ||
			req.Bytes != len(body) || string(body) != scenario.want || req.Notify {
			t.Fatalf("%v = %+v body=%q", scenario.args, req, body)
		}
		if req.Validate(cli.MaxPayload) != "" {
			t.Fatalf("%v built an invalid request: %+v", scenario.args, req)
		}
	}
	for _, args := range [][]string{
		{"translate", "--text="},
		{"translate", "--text", "hi", "--file", "-"},
		{"translate", "hi", "--text", "there"},
	} {
		inv, err := parse(args)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		e, _, _, _ := testEnv(t, nil)
		e.stdin = func() (io.Reader, bool) { return strings.NewReader(""), true }
		if _, _, err := buildRequest(inv, e); err == nil {
			t.Fatalf("%v was accepted", args)
		} else if _, ok := err.(*usageError); !ok {
			t.Fatalf("%v: not a usage error: %v", args, err)
		}
	}
	// No text and an interactive stdin is a usage error (exit 2), not a panel.
	e, _, _, conn := translateEnv(t, cli.NewResult(&cli.State{}), "", true)
	if code := run([]string{"translate"}, e); code != exitUsage {
		t.Fatalf("empty translate exit = %d", code)
	}
	if conn.req.Action != "" {
		t.Fatal("an empty translation connected to the app")
	}
}

func TestTranslateHelpAndExitCodes(t *testing.T) {
	e, stdout, _, _ := testEnv(t, nil)
	if code := run([]string{"help", "translate"}, e); code != exitOK ||
		!strings.Contains(stdout.String(), "--to auto|zh|en") ||
		!strings.Contains(stdout.String(), "default 30s") {
		t.Fatalf("help translate = %d %q", code, stdout.String())
	}
	stdout.Reset()
	if code := run([]string{"help"}, e); code != exitOK || !strings.Contains(stdout.String(), "translate [TEXT]") {
		t.Fatalf("help = %d %q", code, stdout.String())
	}
	for code, want := range map[string]int{
		cli.CodeOK:               exitOK,
		cli.CodeUsage:            exitUsage,
		cli.CodeTimeout:          exitTimeout,
		cli.CodeDisabled:         exitRejected,
		"TranslateTimeout":       exitRejected,
		"TranslateNoSelection":   exitRejected,
		"TranslateNotConfigured": exitRejected,
	} {
		if got := exitCode(code); got != want {
			t.Fatalf("exitCode(%q) = %d, want %d", code, got, want)
		}
	}
	if !strings.Contains(fallbackMessage(cli.CodeDisabled, "translate"), "translate") {
		t.Fatal("a disabled translate module reported the keep-awake module")
	}
}

func TestTranslateHumanAndJSONOutput(t *testing.T) {
	translation := "第一行\n第二行"
	result := cli.Result{V: cli.Protocol, OK: true, State: &cli.State{Version: "0.6.1"},
		Bytes: len(translation), Translate: &cli.TranslateResult{
			Source: "auto", Target: "zh-Hans", Model: "m", Text: translation}}
	e, stdout, stderr, conn := translateEnv(t, result, "", true)
	if code := run([]string{"translate", "Hello iOS"}, e); code != exitOK ||
		stdout.String() != translation || stderr.Len() != 0 {
		t.Fatalf("human = %d %q %q", code, stdout.String(), stderr.String())
	}
	if conn.req.Action != cli.ActionTranslate || conn.req.Bytes != len("Hello iOS") || string(conn.body) != "Hello iOS" {
		t.Fatalf("request = %+v body=%q", conn.req, conn.body)
	}

	e, stdout, stderr, _ = translateEnv(t, result, "", true)
	if code := run([]string{"translate", "Hello iOS", "--json"}, e); code != exitOK || stderr.Len() != 0 {
		t.Fatalf("json = %d %q", code, stderr.String())
	}
	line := strings.TrimSuffix(stdout.String(), "\n")
	if strings.Contains(line, "\n") {
		t.Fatalf("--json printed more than one line: %q", stdout.String())
	}
	var back cli.Result
	if err := json.Unmarshal([]byte(line), &back); err != nil {
		t.Fatalf("%q: %v", line, err)
	}
	if !back.OK || back.Translate == nil || back.Translate.Text != translation || back.Bytes != len(translation) || back.State == nil {
		t.Fatalf("json result = %+v", back)
	}

	failure := cli.Failure("TranslateTimeout")
	failure.Message = "接口响应超时"
	e, stdout, stderr, _ = translateEnv(t, failure, "", true)
	if code := run([]string{"translate", "Hello"}, e); code != exitRejected || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "接口响应超时") {
		t.Fatalf("failed translation = %d %q %q", code, stdout.String(), stderr.String())
	}
}
