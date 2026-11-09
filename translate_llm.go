package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Translation talks to an OpenAI-compatible chat endpoint. This file holds
// only standard-library code: the address rule, the prompt, the SSE reader
// and the two small settings-page probes. Nothing here knows about windows,
// the keychain or the locale catalog.

const (
	translateTemperature      = 0.2
	translateHTTPTimeout      = 50 * time.Second
	translateProbeTimeout     = 10 * time.Second
	translateMaxResponseBytes = 4096
)

type translateConfig struct {
	BaseURL string
	Model   string
	Key     string
}

// translateEndpoints is the single address rule used by the chat stream, the
// model list and the connection test. It normalizes one user-supplied Base
// URL and returns both addresses on the same prefix.
//
//   - a trailing slash or a trailing /chat/completions is stripped;
//   - a bare host gets the /v1 prefix;
//   - chat is Base + /chat/completions, models is Base + /models;
//   - query strings and fragments are always rejected;
//   - https is accepted everywhere, http only on loopback hosts.
//
// Errors are sentinels: they never carry the address, the key or the text.
func translateEndpoints(base string) (chat string, models string, err error) {
	raw := strings.TrimSpace(base)
	u, parseErr := url.Parse(raw)
	if parseErr != nil || u.Scheme == "" || u.Host == "" || u.Opaque != "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", "", errTranslateAddress
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && translateLoopbackAllowed(u):
	default:
		if u.Scheme == "http" {
			return "", "", errTranslateHTTPRejected
		}
		return "", "", errTranslateAddress
	}
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(path, "/chat/completions") {
		path = strings.TrimRight(strings.TrimSuffix(path, "/chat/completions"), "/")
	}
	if path == "" {
		path = "/v1"
	}
	prefix := u.Scheme + "://" + u.Host + path
	return prefix + "/chat/completions", prefix + "/models", nil
}

// translateLoopbackAllowed applies the http exception. The hostname is read
// with url.URL.Hostname(): an IPv6 literal is only a hostname when it is
// bracketed, so the unbracketed form is refused.
func translateLoopbackAllowed(u *url.URL) bool {
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	if addr.Is4() {
		return addr.As4()[0] == 127
	}
	return addr.Is6() && addr.IsLoopback() && strings.HasPrefix(u.Host, "[")
}

// isLoopbackHost reports whether a bare hostname is one of the loopback
// literals. IPv6 needs the brackets checked by translateLoopbackAllowed, so
// this helper is deliberately only about the host text.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	if addr.Is4() {
		return addr.As4()[0] == 127
	}
	return addr.Is6() && addr.IsLoopback()
}

func (c translateConfig) endpoint() (string, error) {
	chat, _, err := translateEndpoints(c.BaseURL)
	return chat, err
}

func (c translateConfig) modelsEndpoint() (string, error) {
	_, models, err := translateEndpoints(c.BaseURL)
	return models, err
}

var (
	errTranslateAddress        = errors.New("translate: invalid endpoint address")
	errTranslateHTTPRejected   = errors.New("translate: http endpoint is not loopback")
	errTranslateNetwork        = errors.New("translate: network failure")
	errTranslateBadResponse    = errors.New("translate: unreadable response")
	errTranslateEmptyResult    = errors.New("translate: empty result")
	errTranslateModelsUnusable = errors.New("translate: model list unusable")
)

type translateHTTPError struct {
	status   int
	modelish bool
}

func (e *translateHTTPError) Error() string { return fmt.Sprintf("translate: status %d", e.status) }

type translateMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// buildTranslateMessages keeps the source text in the user message verbatim.
// The system message only constrains the target language; it never interpolates
// the text, so punctuation and quotes cannot change the instruction.
func buildTranslateMessages(source, target, text string, word bool) []translateMessage {
	task := "Translate the text into natural English. Output only the translation."
	if target == "zh-Hans" {
		task = "Translate the text into Simplified Chinese. Output only the translation. " +
			"Text containing Japanese kana or Korean Hangul is still translated into Simplified Chinese."
	}
	shape := "Keep the punctuation, line breaks and numbers of the whole text."
	if word {
		shape = "This is one word or a short phrase: give its most common meaning, with no explanation."
	}
	sourceHint := "Detect the source language."
	if source == "zh-Hans" {
		sourceHint = "The source is Simplified Chinese."
	}
	return []translateMessage{
		{Role: "system", Content: task + " " + shape + " " + sourceHint},
		{Role: "user", Content: text},
	}
}

func translateHTTPClient() *http.Client {
	return &http.Client{Timeout: translateHTTPTimeout}
}

func translateRequest(ctx context.Context, cfg translateConfig, address string, body []byte, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return nil, errTranslateAddress
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := translateHTTPClient().Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		// An http error wraps the address. Deliberately do not keep it: no URL,
		// key or request body ever reaches a log or the interface.
		return nil, errTranslateNetwork
	}
	return resp, nil
}

// translateStatusError turns a non-2xx answer into an error carrying only the
// status and whether the small response body points at the model name.
func translateStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, translateMaxResponseBytes))
	resp.Body.Close()
	lower := strings.ToLower(string(body))
	return &translateHTTPError{status: resp.StatusCode, modelish: strings.Contains(lower, "model")}
}

// streamTranslation accumulates the SSE deltas and calls onDelta for each one.
// A canceled request returns an error and never reports a half sentence as a
// finished translation.
func streamTranslation(ctx context.Context, cfg translateConfig, text, source, target string, onDelta func(string)) (string, error) {
	address, err := cfg.endpoint()
	if err != nil {
		return "", err
	}
	messages := buildTranslateMessages(source, target, text, isSingleEnglishWord(text))
	payload, err := json.Marshal(map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"temperature": translateTemperature,
		"stream":      true,
	})
	if err != nil {
		return "", errTranslateBadResponse
	}
	resp, err := translateRequest(ctx, cfg, address, payload, "text/event-stream")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", translateStatusError(resp)
	}
	var out strings.Builder
	reader := bufio.NewReader(resp.Body)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			var delta string
			done, parseErr := parseSSELine(bytes.TrimRight(line, "\r\n"), &delta)
			if parseErr != nil {
				return "", parseErr
			}
			if delta != "" {
				out.WriteString(delta)
				if onDelta != nil {
					onDelta(delta)
				}
			}
			if done {
				break
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
				return "", readErr
			}
			return "", errTranslateNetwork
		}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if out.Len() == 0 {
		return "", errTranslateEmptyResult
	}
	return out.String(), nil
}

// parseSSELine reads one data line of an OpenAI-compatible stream. Comments,
// blank lines and heartbeats are ignored; [DONE] and finish_reason end it.
func parseSSELine(line []byte, delta *string) (done bool, err error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" || strings.HasPrefix(trimmed, ":") {
		return false, nil
	}
	if !strings.HasPrefix(trimmed, "data:") {
		return false, nil
	}
	data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if data == "[DONE]" {
		return true, nil
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return false, errTranslateBadResponse
	}
	if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
		return false, errTranslateBadResponse
	}
	if len(chunk.Choices) == 0 {
		return false, nil
	}
	choice := chunk.Choices[0]
	if choice.Delta.Content != "" {
		*delta = choice.Delta.Content
	}
	return choice.FinishReason != "", nil
}

// translateErrorKey maps a failure to a resource key. The interface never
// shows the endpoint's own error text, and the error object carries no URL.
func translateErrorKey(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "TranslateTimeout"
	case errors.Is(err, errTranslateHTTPRejected):
		return "TranslateEndpointHTTPRejected"
	case errors.Is(err, errTranslateAddress), errors.Is(err, errTranslateNetwork), errors.Is(err, context.Canceled):
		return "TranslateNetworkError"
	case errors.Is(err, errTranslateBadResponse), errors.Is(err, errTranslateEmptyResult):
		return "TranslateBadResponse"
	}
	var status *translateHTTPError
	if errors.As(err, &status) {
		switch status.status {
		case 401, 403:
			return "TranslateAuthError"
		case 429:
			return "TranslateRateLimited"
		case 400, 404:
			if status.modelish {
				return "TranslateModelError"
			}
			return "TranslateServerError"
		default:
			return "TranslateServerError"
		}
	}
	return "TranslateServerError"
}

// fetchTranslateModels reads the OpenAI-compatible model list. A failure is
// returned without any address detail; the settings row keeps manual typing
// available either way.
func fetchTranslateModels(ctx context.Context, cfg translateConfig) ([]string, error) {
	address, err := cfg.modelsEndpoint()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errTranslateAddress
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	resp, err := translateHTTPClient().Do(req)
	if err != nil {
		return nil, errTranslateNetwork
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, translateMaxResponseBytes))
		return nil, errTranslateModelsUnusable
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, translateMaxResponseBytes)).Decode(&payload); err != nil {
		return nil, errTranslateModelsUnusable
	}
	var models []string
	for _, item := range payload.Data {
		if name := strings.TrimSpace(item.ID); name != "" {
			models = append(models, name)
		}
	}
	return models, nil
}

// testTranslateConnection verifies the address, key and model with one small
// non-streaming request. It returns a resource key, empty on success.
func testTranslateConnection(ctx context.Context, cfg translateConfig) (string, error) {
	address, err := cfg.endpoint()
	if err != nil {
		return translateErrorKey(err), nil
	}
	if strings.TrimSpace(cfg.Model) == "" || strings.TrimSpace(cfg.Key) == "" {
		return "TranslateNotConfigured", nil
	}
	payload, err := json.Marshal(map[string]any{
		"model":       cfg.Model,
		"messages":    buildTranslateMessages("auto", "zh-Hans", "hello", false),
		"max_tokens":  1,
		"stream":      false,
		"temperature": translateTemperature,
	})
	if err != nil {
		return "TranslateBadResponse", nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, translateProbeTimeout)
	defer cancel()
	resp, err := translateRequest(probeCtx, cfg, address, payload, "application/json")
	if err != nil {
		return translateErrorKey(err), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return translateErrorKey(translateStatusError(resp)), nil
	}
	var answer struct {
		Choices []json.RawMessage `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, translateMaxResponseBytes)).Decode(&answer); err != nil || len(answer.Choices) == 0 {
		resp.Body.Close()
		return "TranslateBadResponse", nil
	}
	resp.Body.Close()
	return "", nil
}
