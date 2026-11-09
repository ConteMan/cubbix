// Package cli is the wire protocol between the cubbix command line tool and
// the running app, plus the client that talks it.
//
// It depends on the standard library only: the CLI links it without MyGo.
package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// Protocol is the wire version. Bump it only for incompatible semantics;
// adding fields does not bump it.
const Protocol = 1

const (
	// MaxLine bounds the JSON line of a request and of a response.
	MaxLine = 4096
	// MaxPayload bounds the document body that follows a json.new line.
	MaxPayload = 64 << 20 // jsonFileLimit
	// MaxConcurrent bounds the connections served at once.
	MaxConcurrent = 8
	// SocketName is the socket's file name inside the config directory.
	SocketName = "cli.sock"
	// MaxArg bounds the normalized keep awake argument.
	MaxArg = 128
)

// Operations.
const (
	OpStatus = "status"
	OpRun    = "run"
)

// Actions, matching paletteURLRequest.Kind in the app.
const (
	ActionAwake       = "awake"
	ActionAwakeStop   = "awake.stop"
	ActionAwakeToggle = "awake.toggle"
	ActionJSONNew     = "json.new"
	ActionPalette     = "palette"
	ActionSettings    = "settings"
)

// Codes the app and the client share. The values are stable API.
const (
	CodeOK               = ""
	CodeBadRequest       = "bad-request"
	CodeProtocolMismatch = "protocol-mismatch"
	CodeUntrustedPeer    = "untrusted-peer"
	CodeBusy             = "busy"
	CodeQuitting         = "quitting"
	CodeDisabled         = "disabled"
)

// Codes only the command line tool produces.
const (
	CodeUsage      = "usage"
	CodeInternal   = "internal"
	CodeNotRunning = "not-running"
	CodeRefused    = "refused"
	CodeStale      = "stale-socket"
	CodeTimeout    = "timeout"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Request is one line of the protocol. Fields the selected op does not use are
// ignored, except that a body is only allowed for json.new.
type Request struct {
	V      int    `json:"v"`
	ID     string `json:"id"`
	Op     string `json:"op"`
	Action string `json:"action,omitempty"`
	// Arg is the normalized keep awake argument, without the lid modifier.
	Arg string `json:"arg,omitempty"`
	// Mode is screen or tasks; empty uses the remembered one.
	Mode string `json:"mode,omitempty"`
	// Lid is nil to use the remembered preference, set to force it.
	Lid *bool `json:"lid,omitempty"`
	// Follow is a PID to follow; Arg must then be empty.
	Follow int  `json:"follow,omitempty"`
	Bytes  int  `json:"bytes,omitempty"`
	Blank  bool `json:"blank,omitempty"`
	// Notify keeps the on-screen hint and the system notification for this
	// request, as --notify asks.
	Notify bool `json:"notify,omitempty"`
	// Page is the settings page for the settings action.
	Page string `json:"page,omitempty"`
}

// Validate reports the protocol code of an invalid request, or "".
func (r Request) Validate(maxPayload int) string {
	if r.Bytes < 0 {
		return CodeBadRequest
	}
	if r.V != Protocol {
		return CodeProtocolMismatch
	}
	if !idPattern.MatchString(r.ID) {
		return CodeBadRequest
	}
	switch r.Op {
	case OpStatus:
		if r.Action != "" || r.Bytes != 0 {
			return CodeBadRequest
		}
		return ""
	case OpRun:
	default:
		return CodeBadRequest
	}
	switch r.Action {
	case ActionAwake:
		if len(r.Arg) > MaxArg || len(r.Page) > 32 || r.Follow < 0 || r.Bytes < 0 {
			return CodeBadRequest
		}
		switch r.Mode {
		case "", "screen", "tasks":
		default:
			return CodeBadRequest
		}
		if r.Bytes > 0 {
			return CodeBadRequest
		}
		if r.Follow > 0 && r.Arg != "" {
			return CodeBadRequest
		}
	case ActionAwakeStop, ActionAwakeToggle, ActionPalette:
		if r.Bytes > 0 || r.Arg != "" || r.Follow != 0 {
			return CodeBadRequest
		}
	case ActionJSONNew:
		if r.Bytes < 0 || r.Bytes > maxPayload {
			return CodeBadRequest
		}
	case ActionSettings:
		if r.Bytes > 0 || r.Arg != "" {
			return CodeBadRequest
		}
		if len(r.Page) > 32 {
			return CodeBadRequest
		}
	default:
		return CodeBadRequest
	}
	return ""
}

// Result is what the command line tool prints: the stable --json object.
type Result struct {
	V       int      `json:"v"`
	OK      bool     `json:"ok"`
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Lines   []string `json:"lines,omitempty"`
	State   *State   `json:"state"`
}

// Response is a Result with the request id the wire response carries.
type Response struct {
	V       int      `json:"v"`
	ID      string   `json:"id"`
	OK      bool     `json:"ok"`
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Lines   []string `json:"lines,omitempty"`
	State   *State   `json:"state"`
}

// Result drops the id for printing.
func (r Response) Result() Result {
	return Result{V: r.V, OK: r.OK, Code: r.Code, Message: r.Message, Lines: r.Lines, State: r.State}
}

// NewResult is a successful result with a state.
func NewResult(state *State) Result {
	return Result{V: Protocol, OK: true, State: state}
}

// Failure builds a failed result.
func Failure(code string) Result {
	return Result{V: Protocol, OK: false, Code: code}
}

// State is the snapshot every successful response carries.
type State struct {
	Version string     `json:"version"`
	Tiles   Tiles      `json:"tiles"`
	Awake   AwakeState `json:"awake"`
	JSON    JSONState  `json:"json"`
}

// Tiles reports which tiles are enabled.
type Tiles struct {
	Awake bool `json:"awake"`
	JSON  bool `json:"json"`
}

// AwakeState is the keep awake snapshot.
type AwakeState struct {
	Active           bool     `json:"active"`
	Mode             string   `json:"mode"`
	Minutes          int      `json:"minutes"`
	Until            string   `json:"until"`
	End              *string  `json:"end"`
	RemainingSeconds *int     `json:"remainingSeconds"`
	Follow           *Follow  `json:"follow"`
	Lid              LidState `json:"lid"`
	Error            string   `json:"error"`
}

// Follow is the process a session follows.
type Follow struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

// LidState is the lid keep awake snapshot.
type LidState struct {
	Capable                bool   `json:"capable"`
	Available              bool   `json:"available"`
	State                  string `json:"state"`
	Reason                 string `json:"reason"`
	BatteryMin             int    `json:"batteryMin"`
	Adapter                string `json:"adapter,omitempty"`
	Rewrites               int    `json:"rewrites,omitempty"`
	LastCode               string `json:"lastCode,omitempty"`
	HelperVersion          string `json:"helperVersion,omitempty"`
	Hello                  string `json:"hello,omitempty"`
	HelperCode             string `json:"helperCode,omitempty"`
	HelperState            string `json:"helperState,omitempty"`
	HelperValue            *int   `json:"helperValue,omitempty"`
	HelperDetail           string `json:"helperDetail,omitempty"`
	InstallStep            string `json:"installStep,omitempty"`
	InstallError           string `json:"installError,omitempty"`
	InstallExitCode        *int   `json:"installExitCode,omitempty"`
	InstallScriptExitCode  *int   `json:"installScriptExitCode,omitempty"`
	InstallCommandExitCode *int   `json:"installCommandExitCode,omitempty"`
}

// JSONState reports the open JSON windows.
type JSONState struct {
	Windows int `json:"windows"`
}

// EncodeLine marshals v as one JSON line, capped by MaxLine.
func EncodeLine(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(data)+1 > MaxLine {
		return nil, fmt.Errorf("cli: response over %d bytes", MaxLine)
	}
	return append(data, '\n'), nil
}

// FormatEnd renders a deadline as RFC3339 in UTC, as the JSON field documents.
func FormatEnd(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	value := t.UTC().Format(time.RFC3339)
	return &value
}
