package main

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Only fixed installer markers cross the process boundary. Raw command output
// can contain paths, account names or authorization messages and is never kept.
type lidInstallDiagnostic struct {
	step, failure                 string
	exit, scriptExit, commandExit *int
}

type lidInstallFailure struct {
	diagnostic lidInstallDiagnostic
	cause      error
}

func (e *lidInstallFailure) Error() string { return e.cause.Error() }
func (e *lidInstallFailure) Unwrap() error { return e.cause }

var lidInstallMarker = regexp.MustCompile(`(?:^|execution error: )cubbix-(step|error|exit|command-exit)=([a-z0-9-]+)(?: \(-?[0-9]+\))?$`)

func lidAdminResult(output []byte, err error) error {
	if err == nil {
		return nil
	}
	d := lidInstallDiagnostic{step: "authorize", failure: "script-failed"}
	code := -1
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	}
	d.exit = &code
	// AppleScript replaces shell newlines with carriage returns.
	normalized := strings.ReplaceAll(strings.ReplaceAll(string(output), "\r\n", "\n"), "\r", "\n")
	// Match one line at a time so adjacent markers cannot consume each other's
	// newline. The first line may have the interpreter's execution error prefix.
	for _, line := range strings.Split(normalized, "\n") {
		m := lidInstallMarker.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch m[1] {
		case "step":
			switch m[2] {
			case "copy", "verify", "replace", "plist", "bootout", "terminate", "unload-wait", "bootstrap", "confirm":
				d.step = m[2]
			}
		case "error":
			switch m[2] {
			case "unload-timeout", "bootstrap-failed", "confirm-failed":
				d.failure = m[2]
			}
		case "exit", "command-exit":
			if v, e := strconv.Atoi(m[2]); e == nil && v >= 0 && v <= 255 {
				if m[1] == "exit" {
					d.scriptExit = &v
				} else {
					d.commandExit = &v
				}
			}
		}
	}
	cause := errors.New("LidServiceBroken")
	if strings.Contains(normalized, "(-128)") {
		cause = errLidDenied
		d.failure = "authorization-cancelled"
	}
	return &lidInstallFailure{diagnostic: d, cause: cause}
}
