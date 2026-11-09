package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTranslateEndpoint(t *testing.T) {
	cases := []struct {
		base   string
		chat   string
		models string
		err    error
	}{
		{"https://h", "https://h/v1/chat/completions", "https://h/v1/models", nil},
		{"https://h/", "https://h/v1/chat/completions", "https://h/v1/models", nil},
		{"https://h/v1", "https://h/v1/chat/completions", "https://h/v1/models", nil},
		{"https://h/v1/", "https://h/v1/chat/completions", "https://h/v1/models", nil},
		{"https://h/v1/chat/completions", "https://h/v1/chat/completions", "https://h/v1/models", nil},
		{"http://127.0.0.1:9/v1", "http://127.0.0.1:9/v1/chat/completions", "http://127.0.0.1:9/v1/models", nil},
		{"http://localhost/v1", "http://localhost/v1/chat/completions", "http://localhost/v1/models", nil},
		{"http://[::1]:9/v1", "http://[::1]:9/v1/chat/completions", "http://[::1]:9/v1/models", nil},
		{"http://127.1.2.3/v1", "http://127.1.2.3/v1/chat/completions", "http://127.1.2.3/v1/models", nil},
		{"http://LOCALHOST/v1", "http://LOCALHOST/v1/chat/completions", "http://LOCALHOST/v1/models", nil},
		{"http://::1/v1", "", "", errTranslateHTTPRejected},
		{"http://example.com/v1", "", "", errTranslateHTTPRejected},
		{"http://127.0.0.1.evil.com/v1", "", "", errTranslateHTTPRejected},
		{"http://user@127.0.0.1/v1", "", "", errTranslateAddress},
		{"https://h/v1?x=1", "", "", errTranslateAddress},
		{"https://h/v1#frag", "", "", errTranslateAddress},
		{"https://h?", "", "", errTranslateAddress},
		{"ftp://h/v1", "", "", errTranslateAddress},
		{"", "", "", errTranslateAddress},
	}
	for _, c := range cases {
		chat, models, err := translateEndpoints(c.base)
		// An unbracketed IPv6 literal is refused. Depending on the Go
		// toolchain url.Parse may reject it before the loopback check, so
		// either rejection sentinel is acceptable.
		if c.base == "http://::1/v1" {
			if err == nil || chat != "" || models != "" {
				t.Fatalf("%q: an unbracketed IPv6 address was accepted: %q %q %v", c.base, chat, models, err)
			}
			continue
		}
		if !errors.Is(err, c.err) {
			t.Fatalf("%q: error = %v; want %v", c.base, err, c.err)
		}
		if chat != c.chat || models != c.models {
			t.Fatalf("%q: chat = %q, models = %q", c.base, chat, models)
		}
		// The two wrappers must agree with the single rule.
		cfg := translateConfig{BaseURL: c.base}
		if got, gotErr := cfg.endpoint(); got != chat || !errors.Is(gotErr, c.err) {
			t.Fatalf("%q: endpoint() = %q, %v", c.base, got, gotErr)
		}
		if got, gotErr := cfg.modelsEndpoint(); got != models || !errors.Is(gotErr, c.err) {
			t.Fatalf("%q: modelsEndpoint() = %q, %v", c.base, got, gotErr)
		}
		if err != nil && c.base != "" && strings.Contains(err.Error(), c.base) {
			t.Fatalf("address leaked into error: %v", err)
		}
	}
}

func TestTranslateConfigDefaults(t *testing.T) {
	c := defaultConfig()
	if !c.TranslateEnabled || !c.TranslateAutoSpeak || c.TranslateShortcut != defaultTranslateKey {
		t.Fatalf("defaults = %+v", c)
	}
	if c.TranslateBaseURL != "" || c.TranslateModel != "" {
		t.Fatal("a default address or model was baked in")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("version=1\nlanguage='zh-CN'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	missing, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !missing.TranslateAutoSpeak || !missing.TranslateEnabled || missing.TranslateShortcut != defaultTranslateKey {
		t.Fatalf("missing keys: %+v", missing)
	}
	explicit := "version=1\nlanguage='zh-CN'\ntranslate_auto_speak=false\ntranslate_enabled=false\ntranslate_shortcut=''\ntranslate_base_url=' https://h/v1/ '\ntranslate_model=' group/model '\n"
	if err := os.WriteFile(path, []byte(explicit), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TranslateAutoSpeak || loaded.TranslateEnabled || loaded.TranslateShortcut != defaultTranslateKey {
		t.Fatalf("explicit values overwritten: %+v", loaded)
	}
	if loaded.TranslateBaseURL != "https://h/v1" || loaded.TranslateModel != "group/model" {
		t.Fatalf("address/model normalization = %+v", loaded)
	}
	long := "version=1\nlanguage='zh-CN'\ntranslate_base_url='" + strings.Repeat("x", 600) + "'\n"
	if err := os.WriteFile(path, []byte(long), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadConfig(path)
	if err != nil || loaded.TranslateBaseURL != "" {
		t.Fatalf("overlong address kept: %+v %v", loaded, err)
	}
}

func TestParseSSELine(t *testing.T) {
	cases := []struct {
		line  string
		delta string
		done  bool
		err   bool
	}{
		{`data: {"choices":[{"delta":{"content":"你"}}]}`, "你", false, false},
		{"data: [DONE]", "", true, false},
		{"", "", false, false},
		{": ping", "", false, false},
		{`event: message`, "", false, false},
		{`data: {"choices":[{"delta":{"content":"好"},"finish_reason":"stop"}]}`, "好", true, false},
		{`data: {"choices":[]}`, "", false, false},
		{`data: not-json`, "", false, true},
		{`data: {"error":{"message":"bad"}}`, "", false, true},
	}
	for _, c := range cases {
		var delta string
		done, err := parseSSELine([]byte(c.line), &delta)
		if (err != nil) != c.err || delta != c.delta || done != c.done {
			t.Fatalf("%q: delta=%q done=%v err=%v", c.line, delta, done, err)
		}
	}
}

func TestTranslateStreamingAccumulates(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("missing bearer header")
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []string{"你", "好", "，", "世", "界", "。", "再", "见"} {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", chunk)
		}
		fmt.Fprint(w, ": heartbeat\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	cfg := translateConfig{BaseURL: server.URL, Model: "m/x", Key: "k"}
	var deltas []string
	out, err := streamTranslation(context.Background(), cfg, "hello", "auto", "zh-Hans", func(d string) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "你好，世界。再见" || len(deltas) != 8 {
		t.Fatalf("out=%q deltas=%v", out, deltas)
	}
	if gotBody["model"] != "m/x" || gotBody["stream"] != true || gotBody["temperature"] != 0.2 {
		t.Fatalf("request body = %+v", gotBody)
	}
	messages, ok := gotBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %+v", gotBody["messages"])
	}
	user := messages[1].(map[string]any)
	if user["content"] != "hello" {
		t.Fatalf("user text = %+v", user)
	}
}

func TestTranslateErrorKey(t *testing.T) {
	for _, status := range []int{401, 403} {
		if key := translateErrorKey(&translateHTTPError{status: status}); key != "TranslateAuthError" {
			t.Fatalf("status %d = %s", status, key)
		}
	}
	cases := []struct {
		err error
		key string
	}{
		{&translateHTTPError{status: 429}, "TranslateRateLimited"},
		{&translateHTTPError{status: 500}, "TranslateServerError"},
		{&translateHTTPError{status: 400, modelish: true}, "TranslateModelError"},
		{&translateHTTPError{status: 404}, "TranslateServerError"},
		{context.DeadlineExceeded, "TranslateTimeout"},
		{errTranslateNetwork, "TranslateNetworkError"},
		{errTranslateAddress, "TranslateNetworkError"},
		{errTranslateHTTPRejected, "TranslateEndpointHTTPRejected"},
		{errTranslateEmptyResult, "TranslateBadResponse"},
		{errTranslateBadResponse, "TranslateBadResponse"},
		{errors.New("other"), "TranslateServerError"},
	}
	for _, c := range cases {
		if got := translateErrorKey(c.err); got != c.key {
			t.Fatalf("%v = %s; want %s", c.err, got, c.key)
		}
		if strings.Contains(fmt.Sprint(c.err), "http://") {
			t.Fatalf("error carries an address: %v", c.err)
		}
	}
}

func TestTranslateStreamCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"half\"}}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := translateConfig{BaseURL: server.URL, Model: "m", Key: "k"}
	done := make(chan error, 1)
	go func() {
		_, err := streamTranslation(ctx, cfg, "hello", "auto", "zh-Hans", nil)
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled stream reported success")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled stream did not return")
	}
}

func TestFetchTranslateModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("unexpected model request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":"group/xxx"},{"id":"provider/model"},{"id":"  "}]}`)
	}))
	defer server.Close()
	cfg := translateConfig{BaseURL: server.URL, Model: "manual", Key: "k"}
	models, err := fetchTranslateModels(context.Background(), cfg)
	if err != nil || len(models) != 2 || models[0] != "group/xxx" || models[1] != "provider/model" {
		t.Fatalf("models = %v, %v", models, err)
	}
	// A failed list never touches the manual model name or the chat address.
	if cfg.Model != "manual" {
		t.Fatal("manual model was cleared")
	}
	if chat, err := cfg.endpoint(); err != nil || !strings.HasSuffix(chat, "/chat/completions") {
		t.Fatalf("chat endpoint = %q, %v", chat, err)
	}
	for _, code := range []int{500, 401} {
		failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		if _, err := fetchTranslateModels(context.Background(), translateConfig{BaseURL: failing.URL, Key: "k"}); err == nil {
			t.Fatalf("status %d was accepted", code)
		}
		failing.Close()
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	}))
	defer broken.Close()
	if _, err := fetchTranslateModels(context.Background(), translateConfig{BaseURL: broken.URL, Key: "k"}); err == nil {
		t.Fatal("bad JSON was accepted")
	}
	if _, err := fetchTranslateModels(context.Background(), translateConfig{BaseURL: "http://example.com/v1", Key: "k"}); !errors.Is(err, errTranslateHTTPRejected) {
		t.Fatalf("non-loopback http accepted: %v", err)
	}
}

func TestTranslateTestConnection(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Error("missing bearer header")
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()
	cfg := translateConfig{BaseURL: server.URL, Model: "provider/model", Key: "k"}
	if key, err := testTranslateConnection(context.Background(), cfg); err != nil || key != "" {
		t.Fatalf("success = %q, %v", key, err)
	}
	if body["model"] != "provider/model" || body["max_tokens"] != float64(1) || body["stream"] != false {
		t.Fatalf("probe body = %+v", body)
	}
	for _, c := range []struct {
		status int
		key    string
	}{{401, "TranslateAuthError"}, {429, "TranslateRateLimited"}, {400, "TranslateServerError"}} {
		probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
		}))
		key, err := testTranslateConnection(context.Background(), translateConfig{BaseURL: probe.URL, Model: "m", Key: "k"})
		if err != nil || key != c.key {
			t.Fatalf("status %d = %q, %v", c.status, key, err)
		}
		probe.Close()
	}
	modelProbe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, `{"error":{"message":"model not found"}}`)
	}))
	defer modelProbe.Close()
	if key, _ := testTranslateConnection(context.Background(), translateConfig{BaseURL: modelProbe.URL, Model: "m", Key: "k"}); key != "TranslateModelError" {
		t.Fatalf("model error = %q", key)
	}
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
	}))
	defer hang.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if key, _ := testTranslateConnection(ctx, translateConfig{BaseURL: hang.URL, Model: "m", Key: "k"}); key != "TranslateTimeout" {
		t.Fatalf("timeout = %q", key)
	}
	if key, _ := testTranslateConnection(context.Background(), translateConfig{BaseURL: server.URL, Model: "m"}); key != "TranslateNotConfigured" {
		t.Fatalf("missing key = %q", key)
	}
}

func TestBuildTranslateMessages(t *testing.T) {
	zh := buildTranslateMessages("auto", "zh-Hans", "こんにちは", false)
	if !strings.Contains(zh[0].Content, "Simplified Chinese") || !strings.Contains(zh[0].Content, "kana") {
		t.Fatalf("zh system = %q", zh[0].Content)
	}
	en := buildTranslateMessages("zh-Hans", "en", "用 Cubbix", false)
	if !strings.Contains(en[0].Content, "English") {
		t.Fatalf("en system = %q", en[0].Content)
	}
	word := buildTranslateMessages("auto", "zh-Hans", "hello", true)
	if word[0].Content == zh[0].Content {
		t.Fatal("word and sentence share one instruction")
	}
	// A word gets a short Simplified Chinese gloss, not a long dictionary
	// entry with phonetics, parts of speech, examples or etymology.
	for _, fragment := range []string{"Simplified Chinese", "brief", "gloss", "Do not add phonetics", "dictionary-style detail"} {
		if !strings.Contains(word[0].Content, fragment) {
			t.Fatalf("word instruction misses %q: %q", fragment, word[0].Content)
		}
	}
	wordEnglish := buildTranslateMessages("auto", "en", "hello", true)
	if !strings.Contains(wordEnglish[0].Content, "brief English gloss") ||
		strings.Contains(wordEnglish[0].Content, "Simplified Chinese gloss") {
		t.Fatalf("swapped word instruction = %q", wordEnglish[0].Content)
	}
	raw := "line 1\n\"quotes\" and %s"
	messages := buildTranslateMessages("auto", "zh-Hans", raw, false)
	if messages[1].Content != raw {
		t.Fatalf("user text changed: %q", messages[1].Content)
	}
}
