package main

import (
	"context"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ConteMan/cubbix/internal/cli"
)

// translateCLIRuntime holds everything captured on the main thread before a
// translation worker starts. Keeping it in the request context means a worker
// never reads the shared config or a seam another request might replace.
type translateCLIRuntime struct {
	cfg    translateConfig
	stream translateStreamFunc
	secret translateSecretStore
}

type translateCLIRuntimeKey struct{}

// applyTranslateCLI is the translate hook of cliServer. It runs on the main
// thread; each request owns its context and reply, separate from lidCompletion
// and applyCLIAsync. The HTTP call itself happens on a worker.
func (a *application) applyTranslateCLI(ctx context.Context, req cli.Request, body []byte, reply func(cli.Result, []byte)) {
	var once sync.Once
	send := func(result cli.Result, out []byte) {
		once.Do(func() { reply(result, out) })
	}
	if a.quitting {
		send(cli.Failure(cli.CodeQuitting), nil)
		return
	}
	if !a.config.TranslateEnabled {
		send(cli.Failure(cli.CodeDisabled), nil)
		return
	}
	if len(body) == 0 {
		// bytes == 0 means "translate the AX selection of the frontmost app".
		// It never opens a panel: the CLI is for scripts.
		a.startTranslateCLISelection(ctx, req, send)
		return
	}
	if !cli.ValidTranslateInput(body) || strings.TrimSpace(string(body)) == "" {
		send(cli.Failure(cli.CodeBadRequest), nil)
		return
	}
	a.startTranslateCLI(ctx, req, body, send)
}

// startTranslateCLI captures the main-thread state, then runs the model call
// on a worker. It is shared by an explicit body frame and the selection path.
func (a *application) startTranslateCLI(ctx context.Context, req cli.Request, body []byte, send func(cli.Result, []byte)) {
	runtime := translateCLIRuntime{
		cfg: translateConfig{
			BaseURL: a.config.TranslateBaseURL,
			Model:   strings.TrimSpace(a.config.TranslateModel),
		},
		stream: a.translateStreamFunction(),
		secret: a.translateSecret,
	}
	ctx = context.WithValue(ctx, translateCLIRuntimeKey{}, runtime)
	text, to := string(body), req.To
	a.runTranslateWorker(func() {
		result, out := a.runCLITranslate(ctx, text, to)
		if ctx.Err() != nil {
			return
		}
		a.translateCLIMain(func() {
			if ctx.Err() != nil || a.quitting {
				return
			}
			if result.OK {
				result.State = a.cliState()
			} else if result.Message == "" && strings.HasPrefix(result.Code, "Translate") {
				result.Message = a.text(uiLocale(), result.Code)
			}
			send(result, out)
		})
	})
}

// runCLITranslate clips, detects the direction, streams the translation and
// returns the result plus its body frame. It builds no window and refreshes no
// UI state: the caller assembles the final snapshot on the main thread.
func (a *application) runCLITranslate(ctx context.Context, text, to string) (cli.Result, []byte) {
	runtime, _ := ctx.Value(translateCLIRuntimeKey{}).(translateCLIRuntime)
	if ctx.Err() != nil {
		return cli.Failure(cli.CodeQuitting), nil
	}
	clipped, dropped := clipTranslateInput(text)
	source, target := detectDirection(clipped)
	switch to {
	case cli.TranslateToZh:
		source, target = "auto", "zh-Hans"
	case cli.TranslateToEn:
		source, target = "auto", "en"
	}
	cfg := runtime.cfg
	key, err := readTranslateAPIKey(runtime.secret)
	if err != nil {
		return cli.Failure("TranslateKeyError"), nil
	}
	cfg.Key = key
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" || key == "" {
		return cli.Failure("TranslateNotConfigured"), nil
	}
	// The model gets at most 50s, below the server's 70s wait. Clients using
	// --timeout 60s can receive TranslateTimeout (exit 4) before their deadline.
	ctx, cancel := context.WithTimeout(ctx, translateHTTPTimeout)
	defer cancel()
	if ctx.Err() != nil {
		return cli.Failure(translateErrorKey(ctx.Err())), nil
	}
	stream := runtime.stream
	if stream == nil {
		stream = streamTranslation
	}
	var deltas strings.Builder
	oversized := false
	out, err := stream(ctx, cfg, clipped, source, target, func(delta string) {
		if ctx.Err() != nil {
			return
		}
		if deltas.Len()+len(delta) > cli.MaxTranslateBody {
			oversized = true
			cancel()
			return
		}
		deltas.WriteString(delta)
	})
	if oversized {
		return cli.Failure("TranslateBadResponse"), nil
	}
	if err != nil {
		return cli.Failure(translateErrorKey(err)), nil
	}
	if ctx.Err() != nil {
		return cli.Failure(translateErrorKey(ctx.Err())), nil
	}
	if out == "" {
		out = deltas.String()
	}
	body := []byte(out)
	if len(body) > cli.MaxTranslateBody || !utf8.Valid(body) || len(body) == 0 {
		return cli.Failure("TranslateBadResponse"), nil
	}
	result := cli.Result{
		V:     cli.Protocol,
		OK:    true,
		Bytes: len(body),
		Translate: &cli.TranslateResult{
			Source:  source,
			Target:  target,
			Model:   cfg.Model,
			Clipped: dropped,
		},
	}
	return result, body
}

// translateCLIMain returns to the main thread for the final result. The seam
// is set when the CLI server starts; headless tests call through directly.
func (a *application) translateCLIMain(fn func()) {
	if a.translateCLIDispatch != nil {
		a.translateCLIDispatch(fn)
		return
	}
	fn()
}
