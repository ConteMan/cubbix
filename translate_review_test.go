package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestTranslateStreamEvents(t *testing.T) {
	for _, test := range []struct {
		name   string
		body   string
		output string
		bad    bool
	}{
		{"multiline", "data: {\"choices\":\n: ping\ndata: [{\"delta\":{\"content\":\"你好\"}}]}\n\ndata: [DONE]\n\n", "你好", false},
		{"no final newline", "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}", "你好", false},
		{"done stops callbacks", "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\ndata: [DONE]\n\ndata: invalid\n\n", "你好", false},
		{"finish reason", "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"},\"finish_reason\":\"stop\"}]}\r\n\r\ndata: invalid\r\n\r\n", "你好", false},
		{"large valid chunk", "data: {\"choices\":[{\"delta\":{\"content\":\"" + strings.Repeat("a", 70000) + "\"}}]}\n\n", strings.Repeat("a", 70000), false},
		{"oversized line", "data: " + strings.Repeat("a", translateMaxEventBytes+1), "", true},
		{"oversized event", strings.Repeat("data: "+strings.Repeat("a", 1024)+"\n", 1024) + "\n", "", true},
		{"error JSON", "data: {\"error\":{\"message\":\"private\"}}\n\n", "", true},
		{"empty", "data: {\"choices\":[]}\n\ndata: [DONE]", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			var deltas []string
			output, err := streamTranslation(context.Background(), translateConfig{BaseURL: server.URL}, "hello", "auto", "zh-Hans", func(delta string) {
				deltas = append(deltas, delta)
			})
			if test.bad {
				if translateErrorKey(err) != "TranslateBadResponse" || output != "" {
					t.Fatalf("invalid stream: key=%q, output length=%d", translateErrorKey(err), len(output))
				}
				return
			}
			if err != nil || output != test.output || strings.Join(deltas, "") != test.output || len(deltas) != 1 {
				t.Fatalf("stream: error=%v output length=%d callbacks=%d", err, len(output), len(deltas))
			}
		})
	}
}

func TestTranslateCancelAfterFirstDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"half\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output, err := streamTranslation(ctx, translateConfig{BaseURL: server.URL}, "hello", "auto", "zh-Hans", func(string) { cancel() })
	if !errors.Is(err, context.Canceled) || output != "" {
		t.Fatalf("cancellation reported a completed result: output=%q err=%v", output, err)
	}
}

func TestTranslateBodyTimeouts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	cfg := translateConfig{BaseURL: server.URL, Model: "model", Key: "k"}
	for _, run := range []func(context.Context) string{
		func(ctx context.Context) string {
			_, err := streamTranslation(ctx, cfg, "hello", "auto", "zh-Hans", nil)
			return translateErrorKey(err)
		},
		func(ctx context.Context) string {
			key, _ := testTranslateConnection(ctx, cfg)
			return key
		},
		func(ctx context.Context) string {
			_, err := fetchTranslateModels(ctx, cfg)
			return translateErrorKey(err)
		},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		key := run(ctx)
		cancel()
		if key != "TranslateTimeout" {
			t.Fatalf("body timeout mapped to %q", key)
		}
	}
}

func TestTranslateRejectsRedirect(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			forwarded := make(chan struct{}, 1)
			target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded <- struct{}{} }))
			defer target.Close()
			redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			}))
			defer redirect.Close()
			cfg := translateConfig{BaseURL: redirect.URL, Key: "k"}
			var err error
			if method == http.MethodGet {
				_, err = fetchTranslateModels(context.Background(), cfg)
			} else {
				_, err = streamTranslation(context.Background(), cfg, "source", "auto", "zh-Hans", nil)
			}
			if err == nil {
				t.Fatal("redirect was accepted")
			}
			select {
			case <-forwarded:
				t.Fatal("redirect forwarded credentials or source text")
			default:
			}
		})
	}
}

func TestTranslateAdditionalAddressRules(t *testing.T) {
	for _, base := range []string{"https://h#", "https://h/v1?#", "https://::1/v1", "https://:9/v1", "http://localhost.example/v1", "http://[::ffff:127.0.0.1]/v1"} {
		if _, _, err := translateEndpoints(base); err == nil {
			t.Fatalf("accepted address %q", base)
		}
	}
	chat, models, err := translateEndpoints("https://h/proxy%3Ftenant/v1")
	if err != nil || chat != "https://h/proxy%3Ftenant/v1/chat/completions" || models != "https://h/proxy%3Ftenant/v1/models" {
		t.Fatal("escaped path was not preserved")
	}
	for _, err := range []error{&net.DNSError{Err: "unavailable"}, &url.Error{Op: "Post", URL: "private", Err: errors.New("failed")}} {
		if key := translateErrorKey(err); key != "TranslateNetworkError" {
			t.Fatalf("transport error mapped to %q", key)
		}
	}
}

func TestTranslateLargeModelListAndInvalidProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"data":[`)
			for index := 0; index < 500; index++ {
				if index != 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"id":"provider/model-%d"}`, index)
			}
			fmt.Fprint(w, "]}")
			return
		}
		fmt.Fprint(w, `{"choices":[null]}`)
	}))
	defer server.Close()
	cfg := translateConfig{BaseURL: server.URL, Model: "manual", Key: "k"}
	models, err := fetchTranslateModels(context.Background(), cfg)
	if err != nil || len(models) != 500 {
		t.Fatalf("large list: count=%d err=%v", len(models), err)
	}
	if key, _ := testTranslateConnection(context.Background(), cfg); key != "TranslateBadResponse" {
		t.Fatalf("invalid choice reported success: %q", key)
	}
}

func TestTranslateQueuedGeneration(t *testing.T) {
	a := translateTestApp(t)
	a.translateSecret.(*fakeTranslateSecrets).value = []byte("k")
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	var queued []func()
	a.translateRunOnMain = func(fn func()) { queued = append(queued, fn) }
	a.translateStreamFn = func(_ context.Context, _ translateConfig, text, _, _ string, delta func(string)) (string, error) {
		delta(text)
		if text == "old" {
			return "", errTranslateNetwork
		}
		return text, nil
	}
	s := a.ensureTranslateState()
	s.input = "old"
	a.submitTranslate()
	s.input = "new"
	a.submitTranslate()
	for _, fn := range queued {
		fn()
	}
	if s.phase != translateDone || s.output != "new" || s.errorKey != "" {
		t.Fatalf("old completion overwrote new result: phase=%v output=%q error=%q", s.phase, s.output, s.errorKey)
	}
	queued = nil
	s.input = "old"
	a.submitTranslate()
	a.closeTranslate()
	for _, fn := range queued {
		fn()
	}
	if s.phase != translateLoading || s.output != "" || s.errorKey != "" {
		t.Fatal("completion changed the state after closing")
	}
}

func TestTranslateInputInteraction(t *testing.T) {
	a := translateTestApp(t)
	a.translateSecret.(*fakeTranslateSecrets).value = []byte("k")
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	var targets []string
	a.translateStreamFn = func(_ context.Context, _ translateConfig, text, _, target string, delta func(string)) (string, error) {
		targets = append(targets, target)
		delta("translated")
		return "translated", nil
	}
	s := a.ensureTranslateState()
	s.focus = true
	s.input = "hello"
	tt := ui.NewTester(a.translateView, 544, 300)
	tt.SetLocale("en")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	if s.phase != translateDone || s.input != "hello" || targets[0] != "zh-Hans" {
		t.Fatal("Return did not submit the input")
	}
	a.swapTranslateDirection()
	if targets[1] != "en" {
		t.Fatal("swap did not change the request target")
	}
	tt.Frame()
	tt.Type(" world")
	if s.directionSwapped {
		t.Fatal("new input retained the swapped direction")
	}
	tt.Key(ui.Shift, ui.KeyEnter)
	if len(targets) != 2 || !strings.Contains(s.input, "\n") {
		t.Fatal("Shift+Return submitted instead of inserting a newline")
	}
	tt.Type(strings.Repeat("a", maxTranslateInput+20))
	tt.Key(0, ui.KeyEnter)
	if len([]rune(s.input)) != maxTranslateInput || s.clipped == 0 || s.phase != translateDone {
		t.Fatal("overlong input was not clipped and translated")
	}
	dropped := s.clipped
	tt.Frame()
	if len([]rune(s.input)) != maxTranslateInput {
		t.Fatal("editor overwrote the clipped bound value")
	}
	if !tt.HasText(fmt.Sprintf(a.text("en", "TranslateTooLong"), maxTranslateInput)) {
		t.Fatal("truncation notice does not show the translated character count")
	}
	a.submitTranslate()
	if s.clipped != dropped || s.phase != translateDone {
		t.Fatal("retry lost the truncation overlay")
	}
}

func TestTranslateKeyRowDisclosure(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			a := translateTestApp(t)
			a.page, a.translateKeySaved = settingsTranslate, true
			a.translateSecret.(*fakeTranslateSecrets).value = []byte("old")
			tt := ui.NewTester(a.view, 720, 444)
			tt.SetLocale(locale)
			tt.SetDark(dark)
			tt.Frame()
			for _, key := range []string{"TranslateKeySaved", "TranslateKeyReplace", "TranslateKeyClear"} {
				bounds, ok := tt.Find(a.text(locale, key))
				if !ok || bounds.X < 0 || bounds.X+bounds.W > 696 || bounds.W <= 0 {
					t.Fatalf("key control clipped: locale=%s dark=%v key=%s bounds=%+v", locale, dark, key, bounds)
				}
			}
			if tt.HasText(a.text(locale, "TranslateKeyPlaceholder")) {
				t.Fatal("saved key row still has an input")
			}
			if err := tt.Click(a.text(locale, "TranslateKeyReplace")); err != nil {
				t.Fatal(err)
			}
			if !tt.Focused(a.text(locale, "TranslateAPIKey")) || !a.translateKeyEditing {
				t.Fatal("Replace did not focus the masked input")
			}
			tt.Key(0, ui.KeyEnter)
			if !a.translateKeySaved {
				t.Fatal("empty replacement deleted the saved key")
			}
			tt.Type("replacement")
			tt.Key(0, ui.KeyEnter)
			if !a.translateKeySaved || a.translateKeyEditing || a.translateKeyDraft != "" {
				t.Fatal("save did not collapse and clear the input")
			}
			if err := tt.Click(a.text(locale, "TranslateKeyClear")); err != nil {
				t.Fatal(err)
			}
			if a.translateKeySaved || tt.HasText(a.text(locale, "TranslateKeyReplace")) || tt.HasText(a.text(locale, "TranslateKeyClear")) {
				t.Fatal("missing key row contains saved-key actions")
			}
		}
	}
}

func TestTranslateSecretFailurePreservesSavedState(t *testing.T) {
	a := translateTestApp(t)
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	secrets.value, secrets.setErr = []byte("old"), errors.New("private keychain detail")
	a.translateKeySaved, a.translateKeyEditing = true, true
	a.setTranslateAPIKey("replacement")
	if !a.translateKeySaved || !a.translateKeyEditing || a.translateKeyError != "TranslateKeyError" || a.translateKeyDraft != "" {
		t.Fatal("failed replacement lost the saved key state")
	}
	a.setTranslateAPIKey(strings.Repeat("a", mygo.MaxSecretSize+1))
	if a.translateKeyError != "TranslateKeyError" || secrets.sets != 0 {
		t.Fatal("overlong key was not rejected inline")
	}
}

type blockingTranslateSecrets struct {
	started chan struct{}
	release chan struct{}
}

func (store *blockingTranslateSecrets) Get(string) ([]byte, error) {
	return nil, mygo.ErrSecretNotFound
}
func (store *blockingTranslateSecrets) Set(string, []byte) error {
	store.started <- struct{}{}
	<-store.release
	return nil
}
func (store *blockingTranslateSecrets) Delete(key string) error { return store.Set(key, nil) }

func TestTranslateSecretWritesUseWorkerAndMainDispatch(t *testing.T) {
	for _, value := range []string{"new", ""} {
		a := translateTestApp(t)
		store := &blockingTranslateSecrets{started: make(chan struct{}, 1), release: make(chan struct{})}
		a.translateSecret, a.translateAsync = store, true
		finished := make(chan func(), 1)
		a.translateRunOnMain = func(fn func()) { finished <- fn }
		a.setTranslateAPIKey(value)
		if !a.translateKeyWriting {
			t.Fatal("write did not return with a busy state")
		}
		select {
		case <-store.started:
		case <-time.After(time.Second):
			close(store.release)
			t.Fatal("worker did not begin")
		}
		close(store.release)
		select {
		case finish := <-finished:
			if !a.translateKeyWriting {
				t.Fatal("worker changed the UI before main dispatch")
			}
			finish()
		case <-time.After(time.Second):
			t.Fatal("worker did not dispatch its result")
		}
		if a.translateKeyWriting || a.translateKeySaved != (value != "") {
			t.Fatal("main dispatch did not commit the write result")
		}
	}
}

func TestTranslateModelNameSlash(t *testing.T) {
	for _, name := range []string{"group/xxx", "provider/model", "a/b/c"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Model != name {
				t.Error("model name changed in the request")
			}
			fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
		}))
		_, err := testTranslateConnection(context.Background(), translateConfig{BaseURL: server.URL, Model: name, Key: "k"})
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
