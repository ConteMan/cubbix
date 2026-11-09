package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
)

// translateCLIApp is a headless app with a fake keychain and a configured
// endpoint. Every test injects a stream seam, so no case opens a window,
// touches the real keychain, registers a shortcut or sends a network request.
func translateCLIApp(t *testing.T) *application {
	t.Helper()
	a := cliTestApp(t)
	a.translateSecret = newFakeTranslateSecrets()
	a.translateSecret.(*fakeTranslateSecrets).value = []byte("k")
	a.config.TranslateBaseURL = "https://h/v1"
	a.config.TranslateModel = "group/model"
	return a
}

func translateCLIRequest(to string) cli.Request {
	return cli.Request{V: cli.Protocol, ID: "t", Op: cli.OpRun, Action: cli.ActionTranslate, To: to}
}

// A successful translation needs no window at all: call the hook directly.
func runTranslateCLI(t *testing.T, a *application, req cli.Request, body []byte) (cli.Result, []byte) {
	t.Helper()
	var result cli.Result
	var out []byte
	calls := 0
	a.applyTranslateCLI(req, body, func(r cli.Result, b []byte) {
		calls++
		result, out = r, b
	})
	if calls != 1 {
		t.Fatalf("reply called %d times", calls)
	}
	return result, out
}

func TestCommandPrefixRawArg(t *testing.T) {
	a := translateCLIApp(t)
	commands := a.commands()
	cases := []struct {
		query, arg, raw, id string
	}{
		{"翻译 Hello iOS", "hello ios", "Hello iOS", "translate.open"},
		{"翻译 Cubbix", "cubbix", "Cubbix", "translate.open"},
		{"翻译 macOS", "macos", "macOS", "translate.open"},
		{"translate Hello iOS", "hello ios", "Hello iOS", "translate.open"},
		{"常亮 1h", "1h", "1h", "awake.start"},
	}
	for _, scenario := range cases {
		prefix, ok := splitCommandQuery(a, "zh-CN", scenario.query, commands)
		if !ok || prefix.Command.ID != scenario.id {
			t.Fatalf("split(%q) = %+v ok=%v", scenario.query, prefix, ok)
		}
		if prefix.Arg != scenario.arg || prefix.RawArg != scenario.raw {
			t.Fatalf("split(%q): Arg=%q RawArg=%q; want %q / %q",
				scenario.query, prefix.Arg, prefix.RawArg, scenario.arg, scenario.raw)
		}
	}
}

func TestRefreshPaletteParamText(t *testing.T) {
	a := translateCLIApp(t)
	p := &paletteState{}
	p.query = "翻译 Hello iOS"
	a.refreshPalette(p, "zh-CN")
	if p.request.Command == nil || p.request.Command.ID != "translate.open" || p.request.Arg != "Hello iOS" {
		t.Fatalf("paramText request = %+v", p.request)
	}
	if p.request.Awake.Error != "" {
		t.Fatalf("a text command parsed a keep-awake argument: %+v", p.request.Awake)
	}

	awakePalette := &paletteState{}
	awakePalette.query = "常亮 45m"
	a.refreshPalette(awakePalette, "zh-CN")
	if awakePalette.request.Awake.Minutes != 45 || awakePalette.request.Arg != "45m" {
		t.Fatalf("paramAwake regressed: %+v", awakePalette.request)
	}

	bare := &paletteState{}
	bare.query = "翻译"
	a.refreshPalette(bare, "zh-CN")
	if bare.request.Command == nil || bare.request.Command.ID != "translate.open" || bare.request.Arg != "" {
		t.Fatalf("bare translate request = %+v", bare.request)
	}

	a.config.TranslateEnabled = false
	if findCommand(a.commands(), "translate.open") != nil {
		t.Fatal("a closed module still offers the translate command")
	}
}

func TestTranslateCommandStartsOriginalText(t *testing.T) {
	a := translateCLIApp(t)
	var seen string
	a.translateStreamFn = func(_ context.Context, _ translateConfig, text, _, _ string, _ func(string)) (string, error) {
		seen = text
		return "译文", nil
	}
	cmd := findCommand(a.commands(), "translate.open")
	if cmd == nil || cmd.Param != paramText {
		t.Fatalf("translate.open = %+v", cmd)
	}
	if a.text("zh-CN", commandCategory(cmd)) == "" {
		t.Fatal("translate command has no localized category")
	}
	if err := cmd.Run(a, request{Command: cmd, Arg: "Hello iOS"}); err != nil {
		t.Fatal(err)
	}
	s := a.ensureTranslateState()
	if s.input != "Hello iOS" || s.visible != true {
		t.Fatalf("panel did not open with the raw text: %+v", s)
	}
	if seen != "Hello iOS" || s.output != "译文" || s.phase != translateDone {
		t.Fatalf("translation did not start: seen=%q output=%q phase=%v", seen, s.output, s.phase)
	}
}

func TestCLITranslateGeneration(t *testing.T) {
	a := translateCLIApp(t)
	a.translateAsync = true
	a.lidCompletion = nil
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondDone := make(chan struct{})
	firstDone := make(chan struct{})
	a.translateStreamFn = func(_ context.Context, _ translateConfig, text, _, _ string, _ func(string)) (string, error) {
		if text == "one" {
			close(firstStarted)
			<-releaseFirst
		}
		return "T:" + text, nil
	}
	var mu sync.Mutex
	counts := map[string]int{}
	results := map[string]cli.Result{}
	bodies := map[string]string{}
	reply := func(name string, done chan struct{}) func(cli.Result, []byte) {
		return func(r cli.Result, body []byte) {
			mu.Lock()
			counts[name]++
			results[name], bodies[name] = r, string(body)
			mu.Unlock()
			close(done)
		}
	}
	a.applyTranslateCLI(translateCLIRequest(""), []byte("one"), reply("one", firstDone))
	<-firstStarted
	a.applyTranslateCLI(translateCLIRequest(""), []byte("two"), reply("two", secondDone))
	<-secondDone
	close(releaseFirst)
	<-firstDone

	if counts["one"] != 1 || counts["two"] != 1 {
		t.Fatalf("reply counts = %v", counts)
	}
	if results["one"].OK || results["one"].Code != cli.CodeBusy {
		t.Fatalf("the late generation was not dropped: %+v", results["one"])
	}
	// The app keeps the translation in the body frame; only the client fills
	// Translate.Text from it.
	if !results["two"].OK || results["two"].Translate == nil || results["two"].Translate.Text != "" ||
		results["two"].Bytes != len("T:two") || bodies["two"] != "T:two" {
		t.Fatalf("new generation = %+v body=%q", results["two"], bodies["two"])
	}
	if a.lidCompletion != nil || a.lidBusy {
		t.Fatal("a translation touched the lid completion path")
	}
}

func TestCLITranslateFailuresAndDirection(t *testing.T) {
	a := translateCLIApp(t)
	a.translateStreamFn = func(_ context.Context, _ translateConfig, _, _, _ string, _ func(string)) (string, error) {
		return "ok", nil
	}
	// The module switch and an empty body are application failures (exit 4).
	a.config.TranslateEnabled = false
	if result, _ := runTranslateCLI(t, a, translateCLIRequest(""), []byte("hello")); result.Code != cli.CodeDisabled {
		t.Fatalf("closed module = %+v", result)
	}
	a.config.TranslateEnabled = true
	result, _ := runTranslateCLI(t, a, translateCLIRequest(""), nil)
	if result.Code != "TranslateNoSelection" || result.Message == "" {
		t.Fatalf("empty body = %+v", result)
	}
	// A missing key is "not configured", never a network call.
	a.translateSecret.(*fakeTranslateSecrets).value = nil
	result, _ = runTranslateCLI(t, a, translateCLIRequest(""), []byte("hello"))
	if result.Code != "TranslateNotConfigured" || result.Message == "" {
		t.Fatalf("unconfigured = %+v", result)
	}
	a.translateSecret.(*fakeTranslateSecrets).value = []byte("k")

	// --to overrides the detected direction, including for Chinese text that
	// would otherwise be translated into English.
	var source, target string
	a.translateStreamFn = func(_ context.Context, _ translateConfig, _, s, tg string, _ func(string)) (string, error) {
		source, target = s, tg
		return "译文", nil
	}
	for _, scenario := range []struct {
		to, want string
	}{
		{"", "en"}, // auto: Chinese input goes to English
		{cli.TranslateToZh, "zh-Hans"},
		{cli.TranslateToEn, "en"},
	} {
		result, body := runTranslateCLI(t, a, translateCLIRequest(scenario.to), []byte("你好"))
		if !result.OK || result.Translate == nil || result.Translate.Target != scenario.want || target != scenario.want || string(body) != "译文" {
			t.Fatalf("--to %q = %+v source=%q target=%q", scenario.to, result, source, target)
		}
		if result.Bytes != len(body) || result.Translate.Text != "" || result.State == nil {
			t.Fatalf("--to %q server result = %+v", scenario.to, result)
		}
	}

	// A model timeout is an application key (exit 4), not the client's
	// protocol timeout (exit 5).
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		return "", context.DeadlineExceeded
	}
	result, _ = runTranslateCLI(t, a, translateCLIRequest(""), []byte("hello"))
	if result.Code != "TranslateTimeout" || result.Message == "" {
		t.Fatalf("model timeout = %+v", result)
	}
	// Error objects never carry the endpoint or the key.
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		return "", errors.New("post https://h/v1/chat/completions: bearer k")
	}
	if result, _ := runTranslateCLI(t, a, translateCLIRequest(""), []byte("hello")); strings.Contains(result.Message, "https://") || result.Code == "" {
		t.Fatalf("error leaked the endpoint: %+v", result)
	}
}

func TestCLITranslateClipped(t *testing.T) {
	a := translateCLIApp(t)
	var seen string
	a.translateStreamFn = func(_ context.Context, _ translateConfig, text, _, _ string, _ func(string)) (string, error) {
		seen = text
		return "ok", nil
	}
	source := strings.Repeat("原", 4001)
	result, body := runTranslateCLI(t, a, translateCLIRequest(""), []byte(source))
	if !result.OK || result.Translate == nil || result.Translate.Clipped != 1 || string(body) != "ok" {
		t.Fatalf("clipped result = %+v body=%q", result, body)
	}
	if len([]rune(seen)) != 4000 {
		t.Fatalf("the model received %d runes, want 4000", len([]rune(seen)))
	}
}

// TestCLITranslateLongBodyEndToEnd is the acceptance case for the response
// frame: 4000 Chinese characters plus a full state must reach the client.
func TestCLITranslateLongBodyEndToEnd(t *testing.T) {
	a := translateCLIApp(t)
	translation := strings.Repeat("译", 4000)
	var received string
	a.translateStreamFn = func(_ context.Context, cfg translateConfig, text, _, _ string, _ func(string)) (string, error) {
		received = text
		if cfg.Key != "k" || cfg.Model != "group/model" {
			t.Fatalf("config = %+q", cfg)
		}
		return translation, nil
	}
	s, path := testServer(t, a)
	s.translateAsync = a.applyTranslateCLI
	client := dial(t, path)
	source := strings.Repeat("原", 4000)
	req := translateCLIRequest("auto")
	req.Bytes = len(source)
	result, err := client.Do(req, []byte(source), 5*time.Second)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if received != source {
		t.Fatalf("the app received %d runes, want 4000", len([]rune(received)))
	}
	if !result.OK || result.State == nil || result.Translate == nil {
		t.Fatalf("result = %+v", result)
	}
	if result.Bytes != len(translation) || result.Translate.Text != translation {
		t.Fatalf("translation = %d bytes, want %d", len(result.Translate.Text), len(translation))
	}
	if result.Translate.Clipped != 0 || result.Translate.Model != "group/model" || result.Translate.Target != "en" {
		t.Fatalf("metadata = %+v", result.Translate)
	}
}

// TestCLITranslateResponseFields pins the server-side timeout override and
// proves writeResult no longer drops the new fields.
func TestCLITranslateResponseFields(t *testing.T) {
	s := &cliServer{}
	if got := s.actionTimeoutFor(cli.Request{Action: cli.ActionTranslate}); got != cliTranslateActionTimeout || got < 60*time.Second {
		t.Fatalf("translate action timeout = %v", got)
	}
	if got := s.actionTimeoutFor(cli.Request{Action: cli.ActionAwake}); got != defaultCLIActionTimeout {
		t.Fatalf("other action timeout = %v", got)
	}
	s.actionTimeout = time.Second
	if got := s.actionTimeoutFor(cli.Request{Action: cli.ActionTranslate}); got != time.Second {
		t.Fatalf("explicit test timeout = %v", got)
	}

	dir := shortDir(t)
	path := filepath.Join(dir, "fields.sock")
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	response := cli.Response{V: cli.Protocol, ID: "x", OK: true, Bytes: 6,
		State:     &cli.State{Version: "0.6.1"},
		Translate: &cli.TranslateResult{Source: "auto", Target: "zh-Hans", Model: "m", Clipped: 2}}
	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		writeResult(conn, response)
	}()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	if len(line) > cli.MaxLine {
		t.Fatalf("response line = %d bytes", len(line))
	}
	var back cli.Response
	if err := json.Unmarshal(line, &back); err != nil {
		t.Fatal(err)
	}
	if back.Bytes != 6 || back.Translate == nil || back.Translate.Clipped != 2 || back.Translate.Text != "" || back.State == nil {
		t.Fatalf("writeResult dropped a field: %+v", back)
	}
}
