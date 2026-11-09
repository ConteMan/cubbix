package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/ConteMan/cubbix/internal/cli"
)

// cliServer is the app's end of the command line protocol: a Unix socket only
// this user may connect to, serving status and the same actions URLs carry.
type cliServer struct {
	path          string
	listener      *net.UnixListener
	inode         os.FileInfo
	slots         chan struct{}
	wg            sync.WaitGroup
	closing       atomic.Bool
	lifecycle     sync.Mutex // serializes WaitGroup.Add with shutdown
	actionTimeout time.Duration
	context       context.Context
	cancel        context.CancelFunc

	// Seams: tests replace them so no socket, no window and no main loop is
	// involved.
	verify     func(*net.UnixConn) error
	apply      func(cli.Request, []byte) cli.Result
	applyAsync func(cli.Request, []byte, func(cli.Result))
	// translateAsync is deliberately separate from applyAsync: a translation
	// must never claim the lid completion that applyCLIAsync owns.
	translateAsync func(context.Context, cli.Request, []byte, func(cli.Result, []byte))
	dispatch       func(func())
	logf           func(string, ...any)
}

const (
	defaultCLIActionTimeout = 15 * time.Second
	// cliTranslateActionTimeout outlasts the client's 60s --timeout ceiling,
	// so the client never receives busy before its own timer fires.
	cliTranslateActionTimeout = 70 * time.Second
)

// startCLIServer listens for the command line tool. It never blocks the app:
// a socket that cannot be created only costs the tool.
func (a *application) startCLIServer() {
	dir := filepath.Dir(a.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Print("cli: support directory unavailable")
		return
	}
	s := &cliServer{
		path:     cli.SocketPath(dir),
		slots:    make(chan struct{}, cli.MaxConcurrent),
		verify:   verifySameUser,
		dispatch: a.urlDispatch,
		logf:     log.Printf,
	}
	s.applyAsync = a.applyCLIAsync
	s.translateAsync = a.applyTranslateCLI
	// A CLI translation assembles its final snapshot on the main thread.
	a.translateCLIDispatch = a.urlDispatch
	if err := s.listen(); err != nil {
		s.logf("cli: socket unavailable")
		return
	}
	a.cli = s
	go s.serve()
}

// listen binds the socket. It follows the single instance lock's order: connect
// first (someone may be serving), then remove a leftover file, then bind.
func (s *cliServer) listen() error {
	s.context, s.cancel = context.WithCancel(context.Background())
	dir := filepath.Dir(s.path)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return errors.New("cli: socket directory unavailable")
	}
	// The shared /tmp fallback is protected by its sticky bit and peer checks.
	if dir != "/tmp" && dir != "/private/tmp" {
		if uid, ok := cli.FileOwner(info); !ok || uid != uint32(os.Getuid()) {
			return errors.New("cli: socket directory belongs to another user")
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 5; attempt++ {
		info, statErr := os.Lstat(s.path)
		if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		if statErr == nil {
			if uid, ok := cli.FileOwner(info); !ok || uid != uint32(os.Getuid()) || info.Mode()&os.ModeSocket == 0 {
				return errors.New("cli: unsafe socket path")
			}
			conn, err := net.DialTimeout("unix", s.path, time.Second)
			if err == nil {
				conn.Close()
				return errors.New("cli: another instance is serving")
			}
			// Only connection refusal proves that an existing socket is stale.
			if !errors.Is(err, syscall.ECONNREFUSED) && !errors.Is(err, syscall.ENOENT) {
				return err
			}
			current, err := os.Lstat(s.path)
			if err == nil {
				if !os.SameFile(current, info) {
					continue
				}
				if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
					return err
				}
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: s.path, Net: "unix"})
		if err == nil {
			listener.SetUnlinkOnClose(false)
			inode, err := os.Lstat(s.path)
			if err != nil {
				listener.Close()
				return err
			}
			s.listener, s.inode = listener, inode
			if uid, ok := cli.FileOwner(inode); !ok || uid != uint32(os.Getuid()) || inode.Mode()&os.ModeSocket == 0 {
				listener.Close()
				return errors.New("cli: unsafe bound socket")
			}
			// Bind first: never chmod an inode left by another process.
			if err := os.Chmod(s.path, 0o600); err != nil {
				s.stop()
				return err
			}
			return nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return errors.New("cli: socket could not be bound")
}

// serve accepts connections until stop.
func (s *cliServer) serve() {
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			return
		}
		// Verify even rejected peers before responding; never read their data.
		if err := s.verify(conn); err != nil {
			conn.Close()
			continue
		}
		// Slow clients occupy one of the bounded slots before any reads.
		s.lifecycle.Lock()
		if s.closing.Load() {
			s.lifecycle.Unlock()
			conn.Close()
			return
		}
		select {
		case s.slots <- struct{}{}:
		default:
			s.lifecycle.Unlock()
			writeBusy(conn)
			conn.Close()
			continue
		}
		s.wg.Add(1)
		s.lifecycle.Unlock()
		go s.handle(conn)
	}
}

// stop closes the listener, lets in-flight requests finish and removes the
// socket only if it is still ours.
func (s *cliServer) stop() {
	if s == nil {
		return
	}
	s.lifecycle.Lock()
	s.closing.Store(true)
	if s.cancel != nil {
		s.cancel()
	}
	s.lifecycle.Unlock()
	if s.listener != nil {
		s.listener.Close()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
	if s.inode == nil {
		return
	}
	if info, err := os.Lstat(s.path); err == nil && os.SameFile(info, s.inode) {
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
			s.logf("cli: socket cleanup failed")
		}
	}
}

// handle serves one connection: verify, read, apply on the main thread, answer.
func (s *cliServer) handle(conn *net.UnixConn) {
	defer s.wg.Done()
	defer func() { <-s.slots }()
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReaderSize(conn, cli.MaxLine+1)
	line, err := readCLILine(reader)
	if err != nil {
		return
	}
	var req cli.Request
	if err := json.Unmarshal(line, &req); err != nil {
		writeResult(conn, cli.Response{V: cli.Protocol, OK: false, Code: cli.CodeBadRequest})
		return
	}
	if code := req.Validate(cli.MaxPayload); code != "" {
		writeResult(conn, cli.Response{V: cli.Protocol, ID: req.ID, OK: false, Code: code})
		return
	}
	var body []byte
	if req.Bytes > 0 {
		body = make([]byte, req.Bytes)
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		if _, err := readFull(reader, body); err != nil {
			writeResult(conn, cli.Response{V: cli.Protocol, ID: req.ID, OK: false, Code: cli.CodeBadRequest})
			return
		}
		if !utf8.Valid(body) {
			writeResult(conn, cli.Response{V: cli.Protocol, ID: req.ID, OK: false, Code: cli.CodeBadRequest})
			return
		}
	}
	ctx := context.Background()
	cancel := func() {}
	var invalidFrame atomic.Bool
	if req.Action == cli.ActionTranslate {
		if reader.Buffered() > 0 {
			writeResult(conn, cli.Response{V: cli.Protocol, ID: req.ID, Code: cli.CodeBadRequest})
			return
		}
		ctx, cancel = context.WithCancel(s.context)
		defer cancel()
		_ = conn.SetReadDeadline(time.Time{})
		go func() {
			if _, err := reader.ReadByte(); err == nil {
				invalidFrame.Store(true)
			}
			cancel()
		}()
	}
	result, responseBody, ran, dispatched := s.applyReqContext(ctx, req, body)
	cancel()
	if invalidFrame.Load() {
		result, responseBody = cli.Failure(cli.CodeBadRequest), nil
	}
	response := cli.Response{V: cli.Protocol, ID: req.ID, OK: result.OK, Code: result.Code,
		Message: result.Message, Lines: result.Lines, State: result.State,
		Bytes: result.Bytes, Translate: result.Translate}
	if !ran {
		response = cli.Response{V: cli.Protocol, ID: req.ID, OK: false, Code: cli.CodeQuitting}
	}
	if req.Action == cli.ActionTranslate {
		writeTranslationResult(conn, response, responseBody)
	} else {
		writeResult(conn, response)
	}
	// A blocked main-thread dispatch keeps its slot until it has returned.
	// Otherwise repeated timeouts could create an unbounded queue of bodies.
	<-dispatched
}

// applyReq runs the request on the main thread and reports whether it ran at
// all: RunOnMain drops the function once the loop is gone. The translate
// action uses its own hook and carries a response body frame alongside the
// result; it never passes through applyAsync / applyCLIAsync.
func (s *cliServer) applyReq(req cli.Request, body []byte) (cli.Result, []byte, bool, <-chan struct{}) {
	return s.applyReqContext(context.Background(), req, body)
}

func (s *cliServer) applyReqContext(ctx context.Context, req cli.Request, body []byte) (cli.Result, []byte, bool, <-chan struct{}) {
	type outcome struct {
		result cli.Result
		body   []byte
		ran    bool
	}
	result := make(chan outcome, 1)
	finished := make(chan struct{})
	var phase atomic.Int32 // 0 queued, 1 started, 2 cancelled
	var replied sync.Once
	send := func(out outcome) { replied.Do(func() { result <- out }) }
	go func() {
		defer close(finished)
		var out outcome
		deferred := false
		s.dispatch(func() {
			if !phase.CompareAndSwap(0, 1) || s.closing.Load() || ctx.Err() != nil {
				return
			}
			if req.Action == cli.ActionTranslate && s.translateAsync != nil {
				deferred = true
				s.translateAsync(ctx, req, body, func(r cli.Result, out []byte) {
					send(outcome{result: r, body: out, ran: true})
				})
				return
			}
			if s.applyAsync != nil {
				deferred = true
				s.applyAsync(req, body, func(r cli.Result) { send(outcome{result: r, ran: true}) })
				return
			}
			out = outcome{result: s.apply(req, body), ran: true}
		})
		if !deferred {
			send(out)
		}
	}()
	timer := time.NewTimer(s.actionTimeoutFor(req))
	defer timer.Stop()
	select {
	case out := <-result:
		return out.result, out.body, out.ran, finished
	case <-timer.C:
		phase.CompareAndSwap(0, 2) // timed-out queued actions must never execute later
		return cli.Failure(cli.CodeBusy), nil, true, finished
	case <-ctx.Done():
		phase.CompareAndSwap(0, 2)
		return cli.Failure(cli.CodeQuitting), nil, true, finished
	}
}

// actionTimeoutFor keeps the default 15s for every existing action and waits
// 70s for translate, which can spend 50s in the model call. An explicit
// actionTimeout (a test seam) still wins.
func (s *cliServer) actionTimeoutFor(req cli.Request) time.Duration {
	if s.actionTimeout > 0 {
		return s.actionTimeout
	}
	if req.Action == cli.ActionTranslate {
		return cliTranslateActionTimeout
	}
	return defaultCLIActionTimeout
}

// writeBusy answers without a request id: the request was never read.
func writeBusy(conn *net.UnixConn) {
	writeResult(conn, cli.Response{V: cli.Protocol, OK: false, Code: cli.CodeBusy})
}

func writeResult(conn *net.UnixConn, response cli.Response) {
	writeResponseLine(conn, response)
}

// writeTranslationResult sends one small JSON line and then exactly
// response.Bytes bytes of UTF-8 translation. Sharing the write deadline with
// both writes keeps a slow client from holding a slot indefinitely.
func writeTranslationResult(conn *net.UnixConn, response cli.Response, body []byte) {
	if response.Bytes != len(body) || len(body) > cli.MaxTranslateBody || !utf8.Valid(body) ||
		response.OK && (len(body) == 0 || response.Translate == nil || response.Translate.Text != "") ||
		!response.OK && (response.Translate != nil || len(body) != 0) {
		writeResult(conn, cli.Response{V: cli.Protocol, ID: response.ID, Code: cli.CodeInternal})
		return
	}
	if !writeResponseLine(conn, response) {
		return
	}
	if response.Bytes > 0 {
		_, _ = conn.Write(body[:response.Bytes])
	}
}

func writeResponseLine(conn *net.UnixConn, response cli.Response) bool {
	line, err := cli.EncodeLine(response)
	valid := err == nil
	if err != nil {
		line, err = cli.EncodeLine(cli.Response{V: cli.Protocol, ID: response.ID, OK: false, Code: cli.CodeInternal})
		if err != nil {
			return false
		}
	}
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(line)
	return valid && err == nil
}

// readCLILine reads one line, refusing anything over the line limit.
func readCLILine(reader *bufio.Reader) ([]byte, error) {
	var out []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		out = append(out, chunk...)
		if len(out) > cli.MaxLine {
			return nil, errors.New("cli: request line too long")
		}
		if err == nil {
			return out[:len(out)-1], nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}

// readFull fills buf from reader.
func readFull(reader *bufio.Reader, buf []byte) (int, error) {
	read := 0
	for read < len(buf) {
		n, err := reader.Read(buf[read:])
		read += n
		if err != nil {
			return read, err
		}
	}
	return read, nil
}

// verifySameUser refuses a connection from another user before reading a byte.
func verifySameUser(conn *net.UnixConn) error {
	uid, err := cli.PeerUID(conn)
	if err != nil {
		return err
	}
	if uid != uint32(os.Getuid()) {
		return errors.New("cli: connection from another user")
	}
	return nil
}

// applyCLI runs one command line request on the main thread. It never writes
// remembered durations, never authorizes anything and stays quiet unless the
// request asked for a hint.
func (a *application) applyCLI(req cli.Request, body []byte) cli.Result {
	if a.quitting {
		return cli.Failure(cli.CodeQuitting)
	}
	previous := a.quiet
	a.quiet = !req.Notify
	defer func() { a.quiet = previous }()
	locale := uiLocale()
	if req.Op == cli.OpStatus {
		state := a.cliState()
		result := cli.NewResult(state)
		result.Lines = a.cliStatusLines(locale, state)
		return result
	}
	switch req.Action {
	case cli.ActionAwake, cli.ActionAwakeStop, cli.ActionAwakeToggle:
		// A close in flight is busy, not disabled: the module is still on until
		// the transaction has confirmed the system is safe.
		if a.moduleClosing {
			return cli.Failure(cli.CodeBusy)
		}
		if !a.config.AwakeEnabled {
			return cli.Failure(cli.CodeDisabled)
		}
	case cli.ActionJSONNew:
		if !a.config.JSONEnabled {
			return cli.Failure(cli.CodeDisabled)
		}
	}
	var err error
	switch req.Action {
	case cli.ActionAwake:
		request := paletteURLRequest{Kind: "awake", Arg: req.Arg, Mode: req.Mode, Follow: req.Follow, CLI: true}
		if req.Lid != nil {
			request.Lid, request.HasLid = *req.Lid, true
		}
		err = a.runURL(request)
	case cli.ActionAwakeStop:
		err = a.runURL(paletteURLRequest{Kind: "awake.stop", CLI: true})
	case cli.ActionAwakeToggle:
		err = a.runCLIToggle()
		if err != nil || !a.lidBusy {
			a.announceAwake(err)
		}
	case cli.ActionJSONNew:
		if len(body) > cli.MaxPayload {
			return cli.Failure(cli.CodeBadRequest)
		}
		err = a.runURL(paletteURLRequest{Kind: "json.new", Text: string(body), CLI: true})
	case cli.ActionPalette:
		err = a.runURL(paletteURLRequest{Kind: "palette", CLI: true})
	case cli.ActionSettings:
		page, _ := settingsPageIndex(req.Page, a)
		err = a.runURL(paletteURLRequest{Kind: "settings", Page: page, CLI: true})
	default:
		return cli.Failure(cli.CodeBadRequest)
	}
	result := cli.Result{V: cli.Protocol, OK: err == nil}
	if err != nil {
		result.Code = cliErrorKey(err)
		if err.Error() == "ModuleBusy" {
			result.Code = cli.CodeBusy
		}
		result.Message = a.text(locale, result.Code)
	} else {
		result.Message = a.cliMessage(locale, req.Action)
	}
	result.State = a.cliState()
	return result
}

// runCLIToggle is the shortcut's toggle with the command line's rule: the lid
// helper may never ask for authorization on this path.
func (a *application) runCLIToggle() error {
	previous := a.lidURL
	a.lidURL = true
	defer func() { a.lidURL = previous }()
	return a.toggleAwakeResult()
}

// cliErrorKey names the failure the tool reports. ProcessEnded keeps its own
// name so a script can tell a finished target process from a failed start.
func cliErrorKey(err error) string {
	if err == nil {
		return ""
	}
	if err.Error() == "ProcessEnded" {
		return "ProcessEnded"
	}
	return paletteErrorKey(err)
}

// cliMessage is the one human sentence a successful action reports.
func (a *application) cliMessage(locale, action string) string {
	switch action {
	case cli.ActionAwake, cli.ActionAwakeToggle:
		state := a.awake.snapshot()
		if !state.Active {
			return a.text(locale, "ToastAwakeOff")
		}
		if a.lid != nil && a.lid.snapshot().watching {
			return a.text(locale, "LidWatching")
		}
		return a.awakeFeedbackMessage(locale, state.Session).Text
	case cli.ActionAwakeStop:
		return a.text(locale, "ToastAwakeOff")
	}
	return ""
}

// cliStatusLines is the human form of `cubbix status`: the same sentence the
// menu bar shows, the JSON window count and the version.
func (a *application) cliStatusLines(locale string, state *cli.State) []string {
	return []string{
		a.awakeStatus(locale, time.Now()),
		fmt.Sprintf(a.text(locale, "CLIStatusJSON"), state.JSON.Windows),
		"Cubbix " + state.Version,
	}
}

// cliState is the snapshot every answer carries. It is read-only: no probe, no
// helper call, no write to the config.
func (a *application) cliState() *cli.State {
	state := &cli.State{
		Version: runningVersion(),
		Tiles:   cli.Tiles{Awake: a.config.AwakeEnabled, JSON: a.config.JSONEnabled},
	}
	snapshot := a.awake.snapshot()
	state.JSON.Windows = len(a.jsonDocuments)
	awake := cli.AwakeState{Error: snapshot.Error}
	if snapshot.Active {
		session := snapshot.Session
		awake.Active = true
		awake.Mode = session.Mode
		awake.Minutes = session.Minutes
		awake.Until = session.Until
		awake.End = cli.FormatEnd(session.End)
		if !session.End.IsZero() {
			remaining := int(math.Ceil(session.End.Sub(time.Now()).Seconds()))
			if remaining < 0 {
				remaining = 0
			}
			awake.RemainingSeconds = &remaining
		}
		if session.PID > 0 {
			awake.Follow = &cli.Follow{PID: session.PID, Name: session.Name}
		}
	}
	awake.Lid = cli.LidState{
		Capable:    a.lidCapable(),
		Available:  a.lid != nil && a.lid.Available(),
		BatteryMin: a.config.AwakeLidBatteryMin,
		Reason:     a.lidReason(),
	}
	if a.lid != nil {
		s := a.lid.snapshot()
		awake.Lid.State = lidStateName(s.state)
		// lidStateName only sees lidState; watching is an explicit override
		// driven by the helper's state, not a new lidState value.
		if s.state == lidOn && s.watching {
			awake.Lid.State = "watching"
		}
		awake.Lid.Adapter = s.adapter
		awake.Lid.Rewrites = s.rewrites
		awake.Lid.LastCode = s.lastCode
		awake.Lid.HelperVersion = s.helperVersion
		awake.Lid.Hello = s.hello
		awake.Lid.InstallStep, awake.Lid.InstallError = s.install.step, s.install.failure
		awake.Lid.InstallExitCode, awake.Lid.InstallScriptExitCode = s.install.exit, s.install.scriptExit
		awake.Lid.InstallCommandExitCode = s.install.commandExit
		r := s.response
		if s.lastFailure != nil {
			r = *s.lastFailure
		}
		awake.Lid.HelperCode, awake.Lid.HelperState = r.Code, r.State
		awake.Lid.HelperValue, awake.Lid.HelperDetail = r.Value, r.Detail
	} else {
		awake.Lid.State = lidStateName(lidOff)
	}
	if a.lidLastCode != "" {
		awake.Lid.LastCode = a.lidLastCode
	}
	state.Awake = awake
	return state
}

// lidStateName maps the controller's states to the JSON enum.
func lidStateName(state lidState) string {
	switch state {
	case lidEnabling:
		return "enabling"
	case lidOn:
		return "on"
	case lidStopping:
		return "stopping"
	case lidRestoreFailed:
		return "restore-failed"
	case lidHelperGone:
		return "helper-gone"
	case lidBlocked:
		return "blocked"
	case lidError:
		return "error"
	}
	return "off"
}

// settingsPageIndex maps the page name to the app's page number. "tiles" is
// kept as an alias for the keep awake page; it is part of the protocol.
func settingsPageIndex(page string, a *application) (int, bool) {
	switch page {
	case "", "general":
		return settingsGeneral, true
	case "about":
		return settingsAbout, true
	case "awake", "tiles":
		return settingsAwake, true
	case "json":
		return settingsJSON, true
	case "translate":
		return settingsTranslate, true
	case "shortcuts":
		return settingsShortcuts, true
	}
	return settingsGeneral, false
}

// applyCLIAsync installs a completion only for this request. enableLid captures
// it before returning, so unrelated requests cannot claim the pending result.
// Both the action and its final snapshot run on the main thread.
func (a *application) applyCLIAsync(req cli.Request, body []byte, reply func(cli.Result)) {
	previous := a.lidCompletion
	a.lidCompletion = func(err error) {
		result := cli.Result{V: cli.Protocol, OK: err == nil, State: a.cliState()}
		if err != nil {
			result.Code = cliErrorKey(err)
			if a.quitting {
				result.Code = cli.CodeQuitting
			}
			result.Message = a.text(uiLocale(), result.Code)
		} else {
			result.Message = a.cliMessage(uiLocale(), req.Action)
		}
		reply(result)
	}
	wasBusy := a.lidBusy
	result := a.applyCLI(req, body)
	pending := !wasBusy && a.lidBusy && result.OK
	a.lidCompletion = previous
	if !pending {
		reply(result)
	}
}
