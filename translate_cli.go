package main

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ConteMan/cubbix/internal/cli"
)

// translateCLIRuntime holds everything captured on the main thread before a
// translation worker starts. Keeping it in the request context means a worker
// never reads the shared config or a seam another request might replace.
type translateCLIRuntime struct {
	cfg    translateConfig
	stream translateStreamFunc
}

type translateCLIRuntimeKey struct{}

// applyTranslateCLI is the translate hook of cliServer. It runs on the main
// thread, owns its own generation and reply, and never touches lidCompletion
// or applyCLIAsync. The HTTP call itself happens on a worker.
func (a *application) applyTranslateCLI(req cli.Request, body []byte, reply func(cli.Result, []byte)) {
	if a.quitting {
		reply(cli.Failure(cli.CodeQuitting), nil)
		return
	}
	if !a.config.TranslateEnabled {
		reply(cli.Failure(cli.CodeDisabled), nil)
		return
	}
	if len(body) == 0 {
		// bytes == 0 would mean "translate the current selection". That path
		// belongs to a later step; a script must never make the app open a
		// panel, so this is an ordinary application failure (exit 4).
		result := cli.Failure("TranslateNoSelection")
		result.Message = a.text(uiLocale(), "TranslateNoSelection")
		reply(result, nil)
		return
	}
	generation := atomic.AddUint64(&a.translateCLIGeneration, 1)
	runtime := translateCLIRuntime{
		cfg: translateConfig{
			BaseURL: a.config.TranslateBaseURL,
			Model:   strings.TrimSpace(a.config.TranslateModel),
		},
		stream: a.translateStreamFunction(),
	}
	ctx := context.WithValue(context.Background(), translateCLIRuntimeKey{}, runtime)
	text, to := string(body), req.To
	var once sync.Once
	send := func(result cli.Result, out []byte) {
		once.Do(func() { reply(result, out) })
	}
	a.runTranslateWorker(func() {
		result, out := a.runCLITranslate(ctx, text, to, generation)
		a.translateCLIMain(func() {
			// A late result is dropped: it may not answer for a newer request.
			// Each request still gets exactly one reply.
			if atomic.LoadUint64(&a.translateCLIGeneration) != generation {
				send(cli.Failure(cli.CodeBusy), nil)
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
func (a *application) runCLITranslate(ctx context.Context, text, to string, gen uint64) (cli.Result, []byte) {
	runtime, _ := ctx.Value(translateCLIRuntimeKey{}).(translateCLIRuntime)
	if atomic.LoadUint64(&a.translateCLIGeneration) != gen {
		return cli.Failure(cli.CodeBusy), nil
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
	key, err := a.translateAPIKey()
	if err != nil {
		return cli.Failure("TranslateKeyError"), nil
	}
	cfg.Key = key
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" || key == "" {
		return cli.Failure("TranslateNotConfigured"), nil
	}
	// The model gets at most 50s, below the server's 70s wait. A slower model
	// yields an explicit TranslateTimeout (exit 4), not a client timeout (5).
	ctx, cancel := context.WithTimeout(ctx, translateHTTPTimeout)
	defer cancel()
	stream := runtime.stream
	if stream == nil {
		stream = streamTranslation
	}
	var deltas strings.Builder
	out, err := stream(ctx, cfg, clipped, source, target, func(delta string) {
		if atomic.LoadUint64(&a.translateCLIGeneration) != gen {
			cancel()
			return
		}
		deltas.WriteString(delta)
	})
	if err != nil {
		if atomic.LoadUint64(&a.translateCLIGeneration) != gen {
			return cli.Failure(cli.CodeBusy), nil
		}
		return cli.Failure(translateErrorKey(err)), nil
	}
	if out == "" {
		out = deltas.String()
	}
	body := []byte(out)
	if len(body) > cli.MaxTranslateBody {
		return cli.Failure(cli.CodeInternal), nil
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
