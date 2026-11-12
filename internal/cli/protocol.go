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
	"unicode"
	"unicode/utf8"
)

// Protocol is the wire version. Bump it only for incompatible semantics;
// adding fields does not bump it.
const Protocol = 1

const (
	// MaxLine bounds the JSON line of a request and of a response.
	MaxLine = 4096
	// MaxPayload bounds the document body that follows a json.new line.
	MaxPayload = 64 << 20 // jsonFileLimit
	// MaxTranslateBody bounds the translation body frame of a response. The
	// app clips its input to 4000 runes, so 64 KiB leaves ample room even for
	// all-non-ASCII output.
	MaxTranslateBody = 64 << 10
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
	ActionTranslate   = "translate"
	ActionQR          = "qr"
	ActionTS          = "ts"
	ActionUUID        = "uuid"
)

// Accepted --to values for the translate action. Empty means auto.
const (
	TranslateToAuto = "auto"
	TranslateToZh   = "zh"
	TranslateToEn   = "en"
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
// ignored. A body frame is allowed for the three actions that carry text:
// json.new, translate and qr.
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
	// To overrides the detected target language for translate.
	To string `json:"to,omitempty"`
	// Count is how many UUIDs a uuid request generates. 0 means the default 1;
	// the accepted range is 1-100.
	Count int `json:"count,omitempty"`
	// UUIDVersion is 4 or 7 for a uuid request. 0 means the default 4.
	UUIDVersion int `json:"uuidVersion,omitempty"`
	// Upper and Plain are the two presentation switches of a uuid request.
	Upper bool `json:"upper,omitempty"`
	Plain bool `json:"plain,omitempty"`
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
	// The devtools fields belong to the two devtools actions alone; any other
	// action carrying one is a request this version does not understand.
	if r.Action != ActionTS && r.Action != ActionUUID && (r.Count != 0 || r.UUIDVersion != 0 || r.Upper || r.Plain) {
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
	case ActionTranslate:
		// The source text travels in a body frame; a translation never shows
		// a hint, so --notify is a usage error rather than a silent no-op.
		if r.Arg != "" || r.Mode != "" || r.Follow != 0 || r.Lid != nil ||
			r.Blank || r.Page != "" || r.Notify {
			return CodeBadRequest
		}
		if r.Bytes < 0 || r.Bytes > maxPayload {
			return CodeBadRequest
		}
		switch r.To {
		case "", TranslateToAuto, TranslateToZh, TranslateToEn:
		default:
			return CodeBadRequest
		}
	case ActionTS:
		// ts carries its whole input in arg: no body frame, no count, no
		// presentation switches and nothing a hint or a window would use.
		if len(r.Arg) > MaxArg || r.Bytes != 0 || r.Mode != "" || r.Follow != 0 || r.Lid != nil ||
			r.Blank || r.Page != "" || r.Notify || r.To != "" || r.Count != 0 ||
			r.UUIDVersion != 0 || r.Upper || r.Plain {
			return CodeBadRequest
		}
	case ActionUUID:
		// uuid accepts count, uuidVersion, upper and plain only: it has no
		// text, so no body frame, and a hint would be meaningless.
		if r.Arg != "" || r.Bytes != 0 || r.Mode != "" || r.Follow != 0 || r.Lid != nil ||
			r.Blank || r.Page != "" || r.Notify || r.To != "" {
			return CodeBadRequest
		}
		if r.Count < 0 || r.Count > 100 {
			return CodeBadRequest
		}
		switch r.UUIDVersion {
		case 0, 4, 7:
		default:
			return CodeBadRequest
		}
	case ActionQR:
		// The text travels in a body frame, as it does for translate. A QR
		// code never shows a hint either, so --notify is a usage error.
		if r.Arg != "" || r.Mode != "" || r.Follow != 0 || r.Lid != nil ||
			r.Blank || r.Page != "" || r.Notify || r.To != "" {
			return CodeBadRequest
		}
		if r.Bytes < 0 || r.Bytes > maxPayload {
			return CodeBadRequest
		}
	default:
		return CodeBadRequest
	}
	return ""
}

// TranslateResult is the small metadata of a translation. Text is filled in
// locally by the client from the response body frame: the app never puts the
// translation into the JSON line, which must stay below MaxLine.
type TranslateResult struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Model   string `json:"model"`
	Clipped int    `json:"clipped"`
	Text    string `json:"text,omitempty"`
}

// QRResult is the small metadata of one symbol. It carries no text and no
// pixels: the tool draws the symbol itself, so the JSON line stays far below
// MaxLine.
type QRResult struct {
	Version int    `json:"version"`
	ECC     string `json:"ecc"`
	Modules int    `json:"modules"`
	Bytes   int    `json:"bytes"`
	// PNG is the path the image was written to. It is set only when --png
	// asked for a file; the image itself never travels in the answer.
	PNG string `json:"png,omitempty"`
}

// Result is what the command line tool prints: the stable --json object.
type Result struct {
	V         int              `json:"v"`
	OK        bool             `json:"ok"`
	Code      string           `json:"code"`
	Message   string           `json:"message"`
	Lines     []string         `json:"lines,omitempty"`
	State     *State           `json:"state"`
	Bytes     int              `json:"bytes,omitempty"`
	Translate *TranslateResult `json:"translate,omitempty"`
	QR        *QRResult        `json:"qr,omitempty"`
}

// Response is a Result with the request id the wire response carries.
type Response struct {
	V         int              `json:"v"`
	ID        string           `json:"id"`
	OK        bool             `json:"ok"`
	Code      string           `json:"code"`
	Message   string           `json:"message"`
	Lines     []string         `json:"lines,omitempty"`
	State     *State           `json:"state"`
	Bytes     int              `json:"bytes,omitempty"`
	Translate *TranslateResult `json:"translate,omitempty"`
	QR        *QRResult        `json:"qr,omitempty"`
}

// Result drops the id for printing. Every module field travels through it: a
// field added only to Response would be lost before the tool sees it.
func (r Response) Result() Result {
	return Result{V: r.V, OK: r.OK, Code: r.Code, Message: r.Message, Lines: r.Lines,
		State: r.State, Bytes: r.Bytes, Translate: r.Translate, QR: r.QR}
}

// NewResult is a successful result with a state.
func NewResult(state *State) Result {
	return Result{V: Protocol, OK: true, State: state}
}

// Failure builds a failed result.
func Failure(code string) Result {
	return Result{V: Protocol, OK: false, Code: code}
}

func ValidTranslateInput(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, character := range string(data) {
		if unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t' {
			return false
		}
	}
	return true
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
