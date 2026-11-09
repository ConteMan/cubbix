// Command cubbix talks to the running Cubbix app over its Unix socket.
//
// It links the standard library, internal/cli and internal/awake only: it must
// run without MyGo, without the window server and without the app itself.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
)

// Exit statuses, documented in `cubbix help`.
const (
	exitOK       = 0
	exitInternal = 1
	exitUsage    = 2
	exitUnavail  = 3
	exitRejected = 4
	exitTimeout  = 5
)

const (
	launchPoll  = 50 * time.Millisecond
	appBundleID = "com.conteman.cubbix"
)

// conn is what the tool needs of a connection to the app.
type conn interface {
	Do(cli.Request, []byte, time.Duration) (cli.Result, error)
	Close() error
}

// env is everything the tool touches outside itself, so tests can stub it.
type env struct {
	stdout, stderr io.Writer
	dial           func(path string, timeout time.Duration) (conn, error)
	launch         func(target string, byBundleID bool) error
	executable     func() (string, error)
	stdin          func() (io.Reader, bool)
	sleep          func(time.Duration)
}

func live() *env {
	return &env{
		stdout: os.Stdout,
		stderr: os.Stderr,
		dial: func(path string, timeout time.Duration) (conn, error) {
			return cli.Dial(path, timeout)
		},
		launch: func(target string, byBundleID bool) error {
			args := []string{"-g"}
			if byBundleID {
				args = append(args, "-b", target)
			} else {
				args = append(args, "-a", target)
			}
			return exec.Command("/usr/bin/open", args...).Run()
		},
		executable: os.Executable,
		stdin: func() (io.Reader, bool) {
			info, err := os.Stdin.Stat()
			if err != nil {
				return os.Stdin, true
			}
			return os.Stdin, info.Mode()&os.ModeCharDevice != 0
		},
		sleep: time.Sleep,
	}
}

func main() { os.Exit(run(os.Args[1:], live())) }

// invocation is a parsed command line.
type invocation struct {
	command    string // awake.on, awake.off, awake.toggle, awake.status, status, json.new, palette, settings, translate, version, help
	flags      flags
	awake      awakeOptions
	body       bodyOptions
	page       string
	to         string // translate target override
	textArg    string // the positional TEXT of translate
	hasTextArg bool
	helpTopic  string // `cubbix help translate`
}

type flags struct {
	json     bool
	notify   bool
	noLaunch bool
	timeout  time.Duration
	socket   string
}

type awakeOptions struct {
	minutes int
	hasMin  bool
	until   string
	follow  int
	mode    string
	lid     *bool
	arg     string // normalized grammar for the app
}

type bodyOptions struct {
	text    string
	file    string
	blank   bool
	hasText bool
	hasFile bool
}

// usageError is a mistake in the command line itself: exit 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

func run(args []string, e *env) int {
	inv, err := parse(args)
	if err != nil {
		return report(e, invocation{}, wantsJSON(args), failure(err))
	}
	switch inv.command {
	case "help":
		if inv.helpTopic == "translate" {
			fmt.Fprint(e.stdout, translateHelpText)
		} else {
			fmt.Fprint(e.stdout, helpText)
		}
		return exitOK
	case "version":
		return printVersion(e, inv)
	}
	request, payload, err := buildRequest(inv, e)
	if err != nil {
		return report(e, inv, inv.flags.json, failure(err))
	}
	c, err := connect(e, inv)
	if err != nil {
		return report(e, inv, inv.flags.json, failure(err))
	}
	defer c.Close()
	result, err := c.Do(request, payload, inv.flags.timeout)
	if err != nil {
		return report(e, inv, inv.flags.json, failure(err))
	}
	if inv.command == "awake.status" && len(result.Lines) > 1 {
		result.Lines = result.Lines[:1]
	}
	return report(e, inv, inv.flags.json, result)
}

// wantsJSON reports whether --json appeared, even when parsing failed after it:
// the contract is one JSON object whatever the exit status.
func wantsJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--json" {
			return true
		}
	}
	return false
}

// failure turns an error into the result to print. A usage mistake keeps its own
// code: it never reached the app.
func failure(err error) cli.Result {
	result := cli.Failure(cli.CodeOf(err))
	var usage *usageError
	if errors.As(err, &usage) {
		result = cli.Failure(cli.CodeUsage)
	}
	if err != nil {
		result.Message = err.Error()
	}
	return result
}

// report prints one outcome and returns the exit status.
func report(e *env, inv invocation, asJSON bool, result cli.Result) int {
	if result.Code == cli.CodeOK && !result.OK {
		result.Code = cli.CodeInternal
	}
	if !result.OK && result.Message == "" {
		result.Message = fallbackMessage(result.Code, inv.command)
	}
	if asJSON {
		data, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintln(e.stderr, "cubbix: could not encode the answer")
			return exitInternal
		}
		fmt.Fprintln(e.stdout, string(data))
		return exitCode(result.Code)
	}
	if !result.OK {
		fmt.Fprintln(e.stderr, result.Message)
		return exitCode(result.Code)
	}
	if inv.command == "translate" {
		// Human output is the response body frame verbatim, including any
		// line breaks the model produced; --json prints the whole object.
		if result.Translate != nil {
			fmt.Fprint(e.stdout, result.Translate.Text)
		}
		return exitOK
	}
	for _, line := range result.Lines {
		fmt.Fprintln(e.stdout, line)
	}
	if result.Message != "" {
		fmt.Fprintln(e.stdout, result.Message)
	}
	return exitOK
}

// exitCode maps a code to the documented status.
func exitCode(code string) int {
	switch code {
	case cli.CodeOK:
		return exitOK
	case cli.CodeUsage:
		return exitUsage
	case cli.CodeNotRunning, cli.CodeRefused, cli.CodeStale, cli.CodeQuitting:
		return exitUnavail
	case cli.CodeTimeout:
		return exitTimeout
	case cli.CodeBadRequest, cli.CodeProtocolMismatch, cli.CodeUntrustedPeer, cli.CodeInternal:
		return exitInternal
	}
	return exitRejected
}

// fallbackMessage is the English sentence for a code that carries none.
func fallbackMessage(code, command string) string {
	switch code {
	case cli.CodeUsage:
		return `Unknown command. See "cubbix help".`
	case cli.CodeNotRunning:
		return "Cubbix is not running."
	case cli.CodeTimeout:
		return "Cubbix did not answer in time."
	case cli.CodeQuitting:
		return "Cubbix is quitting."
	case cli.CodeDisabled:
		switch {
		case strings.HasPrefix(command, "json"):
			return "The JSON module is disabled."
		case command == "translate":
			return "The translate module is disabled."
		}
		return "The keep-awake module is disabled."
	case cli.CodeBusy:
		return "Cubbix is busy with another request."
	}
	return code
}

// buildRequest turns the invocation into the wire request and its body.
func buildRequest(inv invocation, e *env) (cli.Request, []byte, error) {
	request := cli.Request{V: cli.Protocol, ID: newID(), Notify: inv.flags.notify}
	var payload []byte
	switch inv.command {
	case "status", "awake.status":
		request.Op = cli.OpStatus
	case "awake.on":
		request.Op, request.Action = cli.OpRun, cli.ActionAwake
		request.Arg, request.Mode, request.Follow, request.Lid = inv.awake.arg, inv.awake.mode, inv.awake.follow, inv.awake.lid
	case "awake.off":
		request.Op, request.Action = cli.OpRun, cli.ActionAwakeStop
	case "awake.toggle":
		request.Op, request.Action = cli.OpRun, cli.ActionAwakeToggle
	case "json.new":
		body, err := readBody(inv.body, e)
		if err != nil {
			return request, nil, err
		}
		payload = body
		request.Op, request.Action = cli.OpRun, cli.ActionJSONNew
		request.Bytes, request.Blank = len(body), len(body) == 0
	case "palette":
		request.Op, request.Action = cli.OpRun, cli.ActionPalette
	case "settings":
		request.Op, request.Action, request.Page = cli.OpRun, cli.ActionSettings, inv.page
	case "translate":
		body, err := readTranslateBody(inv, e)
		if err != nil {
			return request, nil, err
		}
		payload = body
		request.Op, request.Action = cli.OpRun, cli.ActionTranslate
		request.Bytes, request.To = len(body), inv.to
	default:
		return request, nil, usagef("Unknown command %q.", inv.command)
	}
	return request, payload, nil
}

// readTranslateBody reads the source text of translate: an argument, --text,
// a file, or stdin when it is a pipe. With no source at all on an interactive
// terminal, bytes == 0 asks the app to translate the current AX selection.
// An explicitly empty text stays a usage error.
func readTranslateBody(inv invocation, e *env) ([]byte, error) {
	sources := 0
	for _, present := range []bool{inv.body.hasText, inv.body.hasFile, inv.hasTextArg} {
		if present {
			sources++
		}
	}
	if sources > 1 {
		return nil, usagef("--text, --file and TEXT are mutually exclusive.")
	}
	var data []byte
	var err error
	switch {
	case inv.body.hasText:
		data = []byte(inv.body.text)
	case inv.body.hasFile:
		data, err = readBody(bodyOptions{file: inv.body.file, hasFile: true}, e)
	case inv.textArg == "-":
		data, err = readStdin(e)
	case inv.hasTextArg:
		data = []byte(inv.textArg)
	default:
		reader, terminal := e.stdin()
		if terminal {
			// No body frame: translate the frontmost application's selection.
			return nil, nil
		}
		data, err = io.ReadAll(io.LimitReader(reader, cli.MaxPayload+1))
	}
	if err != nil {
		return nil, usagef("Could not read translation text.")
	}
	if len(data) > cli.MaxPayload {
		return nil, usagef("The text exceeds the 64 MiB limit.")
	}
	if !cli.ValidTranslateInput(data) {
		return nil, usagef("Translation input must be UTF-8 text without binary control characters.")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, usagef("translate needs text. See \"cubbix help translate\".")
	}
	return data, nil
}

// readBody reads the document a json.new carries: --text, a file, or stdin when
// it is a pipe.
func readBody(body bodyOptions, e *env) ([]byte, error) {
	if body.hasText || body.text != "" {
		return checkSize([]byte(body.text))
	}
	if body.blank {
		return nil, nil
	}
	if body.hasFile || body.file != "" {
		if body.file == "-" {
			return readStdin(e)
		}
		info, err := os.Stat(body.file)
		if err != nil {
			return nil, usagef("Reading %s: %v", body.file, err)
		}
		if !info.Mode().IsRegular() {
			return nil, usagef("Reading %s: not a regular file", body.file)
		}
		if info.Size() > cli.MaxPayload {
			return nil, usagef("Reading %s: file exceeds the 64 MiB limit", body.file)
		}
		file, err := os.Open(body.file)
		if err != nil {
			return nil, usagef("Reading %s: %v", body.file, err)
		}
		defer file.Close()
		info, err = file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return nil, usagef("Reading %s: not a regular file", body.file)
		}
		data, err := io.ReadAll(io.LimitReader(file, cli.MaxPayload+1))
		if err != nil {
			return nil, usagef("Reading %s: %v", body.file, err)
		}
		return checkSize(data)
	}
	reader, terminal := e.stdin()
	if terminal {
		return nil, nil // a blank document, never a prompt
	}
	data, err := io.ReadAll(io.LimitReader(reader, cli.MaxPayload+1))
	if err != nil {
		return nil, usagef("Reading standard input: %v", err)
	}
	return checkSize(data)
}

func readStdin(e *env) ([]byte, error) {
	reader, _ := e.stdin()
	data, err := io.ReadAll(io.LimitReader(reader, cli.MaxPayload+1))
	if err != nil {
		return nil, usagef("Reading standard input: %v", err)
	}
	return checkSize(data)
}

func checkSize(data []byte) ([]byte, error) {
	if len(data) > cli.MaxPayload {
		return nil, usagef("The document exceeds the 64 MiB limit.")
	}
	return data, nil
}

// connect dials the app, launching it when it is not running.
func connect(e *env, inv invocation) (conn, error) {
	path, err := socketPath(inv.flags.socket)
	if err != nil {
		return nil, &cli.Error{Code: cli.CodeNotRunning, Err: err}
	}
	// A connect that has not answered in a few seconds is not going to: the
	// wait budget belongs to the launch polling below.
	dialTimeout := min(inv.flags.timeout, 3*time.Second)
	c, err := e.dial(path, dialTimeout)
	if err == nil {
		return c, nil
	}
	if inv.flags.noLaunch || inv.command == "status" || inv.command == "awake.status" || !cli.CanLaunch(err) {
		return nil, err
	}
	launchApp(e)
	deadline := time.Now().Add(inv.flags.timeout)
	for {
		e.sleep(launchPoll)
		c, err = e.dial(path, dialTimeout)
		if err == nil {
			return c, nil
		}
		if !cli.CanLaunch(err) {
			return nil, err
		}
		if !time.Now().Before(deadline) {
			return nil, &cli.Error{Code: cli.CodeNotRunning,
				Msg: "Cubbix did not start in time (" + inv.flags.timeout.String() + ").", Err: err}
		}
	}
}

// launchApp starts the app bundle this tool belongs to, so the app that comes up
// is the one whose CLI this is; the bundle id is the fallback.
func launchApp(e *env) {
	if app, ok := executableBundle(e); ok {
		if err := e.launch(app, false); err == nil {
			return
		}
	}
	_ = e.launch(appBundleID, true)
}

func socketPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	dir, err := cli.ConfigDir()
	if err != nil {
		return "", err
	}
	return cli.SocketPath(dir), nil
}

// executableBundle is the .app this tool lives in, if any.
func executableBundle(e *env) (string, bool) {
	path, err := e.executable()
	if err != nil || path == "" {
		return "", false
	}
	if resolved, err := evalSymlinks(path); err == nil {
		path = resolved
	}
	return bundleOf(path)
}

// bundleOf finds the .app in an executable path.
func bundleOf(path string) (string, bool) {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if strings.HasSuffix(part, ".app") {
			return strings.Join(parts[:index+1], "/"), true
		}
	}
	return "", false
}

func printVersion(e *env, inv invocation) int {
	bundle, ok := executableBundle(e)
	version := "unknown"
	if ok {
		version = plistVersion(bundle)
	}
	label := "Cubbix " + version
	if ok {
		label += " (" + bundle + ")"
	}
	if inv.flags.json {
		data, err := json.Marshal(cli.Result{V: cli.Protocol, OK: true, Message: label})
		if err != nil {
			return exitInternal
		}
		fmt.Fprintln(e.stdout, string(data))
		return exitOK
	}
	if ok {
		fmt.Fprintln(e.stdout, "Cubbix "+version)
		fmt.Fprintln(e.stdout, bundle)
		return exitOK
	}
	fmt.Fprintln(e.stdout, label)
	return exitOK
}

// newID makes a request id: the answer must carry it back.
func newID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
