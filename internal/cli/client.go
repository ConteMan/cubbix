package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unicode/utf8"
)

// Error is a failure with a stable code, mapped to an exit status by the tool.
type Error struct {
	Code string
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Err }

func failf(code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// CodeOf reports the code of a failure, or CodeInternal for one without a code.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternal
}

// CanLaunch reports whether a dial failure means "Cubbix is not running", which
// the tool may answer by starting the app. A socket this user cannot open is a
// different problem and must not start a second copy.
func CanLaunch(err error) bool {
	switch CodeOf(err) {
	case CodeNotRunning, CodeStale:
		return true
	}
	return false
}

// ConfigDir is the app's support directory, where the socket lives.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "cubbix"), nil
}

// SocketPath returns the socket path inside configDir, or a short fallback when
// that path does not fit in sun_path (104 bytes).
//
// The fallback depends on the uid and the literal /tmp only: $TMPDIR differs
// between a terminal and a process started by `open`, and both sides must
// compute the same path.
func SocketPath(configDir string) string {
	primary := filepath.Join(configDir, SocketName)
	if len(primary) < 100 {
		return primary
	}
	return fmt.Sprintf("/tmp/cubbix-cli-%d.sock", os.Getuid())
}

// Client is one connection to the app.
type Client struct {
	conn *net.UnixConn
	r    *bufio.Reader
}

// Dial connects to path and verifies who serves it before writing anything.
func Dial(path string, timeout time.Duration) (*Client, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSocket == 0 {
		return nil, failf(CodeRefused, "cli: path is not a socket")
	}
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return nil, dialError(err)
	}
	unix, ok := conn.(*net.UnixConn)
	if !ok {
		conn.Close()
		return nil, failf(CodeInternal, "cli: unexpected connection type")
	}
	if err := VerifyServer(unix, path); err != nil {
		unix.Close()
		return nil, err
	}
	return &Client{conn: unix, r: bufio.NewReaderSize(unix, MaxLine+1)}, nil
}

// dialError classifies a failed connect.
func dialError(err error) error {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.ENOENT):
		// A leftover socket file (or none): the app is not serving.
		return &Error{Code: CodeStale, Err: err}
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return &Error{Code: CodeRefused, Msg: "the socket is not reachable by this user", Err: err}
	case errors.Is(err, os.ErrDeadlineExceeded):
		return &Error{Code: CodeTimeout, Err: err}
	}
	return &Error{Code: CodeNotRunning, Err: err}
}

// VerifyServer refuses to talk to anything but this user's own app: the socket
// file must belong to us, and the process on the other end must be us too
// before a body (which may hold a document) is sent.
func socketInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, &Error{Code: CodeStale, Err: err}
	}
	if info.Mode()&os.ModeSocket == 0 {
		return nil, failf(CodeRefused, "%s is not a socket", path)
	}
	if uid, ok := ownerUID(info); !ok || uid != uint32(os.Getuid()) {
		return nil, failf(CodeRefused, "%s belongs to another user", path)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return nil, failf(CodeRefused, "%s is writable by other users", path)
	}
	return info, nil
}

func VerifyServer(conn *net.UnixConn, path string) error {
	if _, err := socketInfo(path); err != nil {
		return err
	}
	uid, err := PeerUID(conn)
	if err != nil {
		return &Error{Code: CodeRefused, Msg: "the socket's owner could not be verified", Err: err}
	}
	if uid != uint32(os.Getuid()) {
		return failf(CodeRefused, "%s is served by another user", path)
	}
	return nil
}

// Do sends one request with its optional body and reads the response.
func (c *Client) Do(req Request, payload []byte, timeout time.Duration) (Result, error) {
	line, err := EncodeLine(req)
	if err != nil {
		return Result{}, &Error{Code: CodeInternal, Err: err}
	}
	if err := c.conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return Result{}, &Error{Code: CodeInternal, Err: err}
	}
	_, writeErr := c.conn.Write(line)
	if writeErr == nil && len(payload) > 0 {
		_, writeErr = c.conn.Write(payload)
	}
	// An overloaded server answers before reading the request and closes.
	// Its busy response remains authoritative even if our upload hit EPIPE.
	result, readErr := c.readResponse(req)
	if readErr == nil {
		if bodyErr := c.readTranslationBody(&result); bodyErr == nil {
			return result, nil
		} else {
			readErr = bodyErr
		}
	}
	if writeErr != nil {
		code := CodeNotRunning
		if errors.Is(writeErr, os.ErrDeadlineExceeded) {
			code = CodeTimeout
		}
		return Result{}, &Error{Code: code, Err: writeErr}
	}
	return Result{}, readErr
}

// readTranslationBody reads the response body frame that follows a translate
// JSON line and fills in Translate.Text locally. Other actions have neither
// Bytes nor Translate, so their wire format and timing stay unchanged.
func (c *Client) readTranslationBody(result *Result) error {
	if result.Translate == nil {
		if result.Bytes > 0 {
			return &Error{Code: CodeInternal, Msg: "Cubbix sent a malformed answer"}
		}
		return nil
	}
	if result.Bytes > MaxTranslateBody {
		return &Error{Code: CodeInternal, Msg: "Cubbix sent an oversized answer"}
	}
	data, err := readBodyFrame(c.r, result.Bytes)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrDeadlineExceeded):
			return &Error{Code: CodeTimeout, Msg: "Cubbix did not answer in time", Err: err}
		case errors.Is(err, io.EOF):
			return &Error{Code: CodeNotRunning, Msg: "Cubbix closed the connection", Err: err}
		default:
			return &Error{Code: CodeInternal, Msg: "Cubbix sent a malformed answer", Err: err}
		}
	}
	result.Translate.Text = string(data)
	return nil
}

// readBodyFrame reads exactly size bytes, with its own limit and UTF-8 check.
// It is the client counterpart of the app's readFull.
func readBodyFrame(r *bufio.Reader, size int) ([]byte, error) {
	if size < 0 || size > MaxTranslateBody {
		return nil, fmt.Errorf("cli: response body over %d bytes", MaxTranslateBody)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	if !utf8.Valid(buf) {
		return nil, errors.New("cli: response body is not UTF-8")
	}
	return buf, nil
}

func (c *Client) readResponse(req Request) (Result, error) {
	data, err := readLine(c.r, MaxLine)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return Result{}, &Error{Code: CodeTimeout, Msg: "Cubbix did not answer in time", Err: err}
		}
		if errors.Is(err, io.EOF) {
			return Result{}, &Error{Code: CodeNotRunning, Msg: "Cubbix closed the connection", Err: err}
		}
		return Result{}, &Error{Code: CodeInternal, Err: err}
	}
	var response Response
	if err := json.Unmarshal(data, &response); err != nil {
		return Result{}, &Error{Code: CodeInternal, Msg: "Cubbix sent a malformed answer", Err: err}
	}
	if response.ID != req.ID && !(response.ID == "" && !response.OK && response.Code == CodeBusy) {
		return Result{}, &Error{Code: CodeInternal, Msg: "Cubbix answered another request"}
	}
	if response.V != Protocol {
		return Result{}, &Error{Code: CodeProtocolMismatch, Msg: "Cubbix speaks another protocol version"}
	}
	return response.Result(), nil
}

// Close closes the connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// readLine reads up to max bytes up to the newline, without buffering more than
// it is allowed to accept.
func readLine(r *bufio.Reader, max int) ([]byte, error) {
	var out []byte
	for {
		chunk, err := r.ReadSlice('\n')
		out = append(out, chunk...)
		if len(out) > max {
			return nil, fmt.Errorf("cli: line over %d bytes", max)
		}
		if err == nil {
			return out[:len(out)-1], nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}
