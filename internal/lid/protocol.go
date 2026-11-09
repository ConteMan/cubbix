// Package lid implements the bounded, single-owner lid-sleep lease.
package lid

import (
	"errors"
	"regexp"
	"time"
)

const (
	Protocol = 1
	// Version is the helper implementation revision, independent of Protocol.
	// Bump it whenever helper behavior changes so installed copies get replaced.
	Version             = "5"
	SocketPath          = "/var/run/com.conteman.cubbix.helper.sock"
	JournalPath         = "/Library/Application Support/com.conteman.cubbix.helper/session.json"
	ClientRequirement   = `anchor apple generic and certificate leaf[subject.OU] = "P93GN4N3DC" and identifier "com.conteman.cubbix"`
	HelperRequirement   = `anchor apple generic and certificate leaf[subject.OU] = "P93GN4N3DC"`
	MaxLine             = 8192
	MaxMinutes          = 525659
	Lease               = 15 * time.Second
	HardLimit           = 12 * time.Hour
	BatteryInterval     = 30 * time.Second
	batteryUnknownLimit = 3
	readFailureLimit    = 3
	// ObserveInterval is the read-only poll, riding the server's 1s tick.
	// Write-backs are separately gated by RewriteGap so observation does not
	// amplify a write war with another power tool.
	ObserveInterval     = 1 * time.Second
	RewriteGap          = 10 * time.Second
	RewriteWindow       = 10 * time.Minute
	RewriteWindowLimit  = 3
	RewriteSessionLimit = 12
	RewriteFailLimit    = 3
	ReadTimeout         = 5 * time.Second
	// Allow a bounded read, a queued request and response serialization.
	CallTimeout = 14 * time.Second
)

type Request struct {
	V            int        `json:"v"`
	ID           string     `json:"id"`
	Op           string     `json:"op"`
	Client       string     `json:"client,omitempty"`
	Deadline     *time.Time `json:"deadline,omitempty"`
	MinPercent   int        `json:"minPercent,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	AdoptForeign bool       `json:"adoptForeign,omitempty"`
}
type Response struct {
	V          int        `json:"v"`
	ID         string     `json:"id"`
	OK         bool       `json:"ok"`
	Protocol   int        `json:"protocol"`
	Helper     string     `json:"helper"`
	State      string     `json:"state"`
	Value      *int       `json:"value"`
	Power      string     `json:"power"`
	Percent    *int       `json:"percent"`
	MinPercent int        `json:"minPercent"`
	Deadline   *time.Time `json:"deadline"`
	Code       string     `json:"code"`
	Detail     string     `json:"detail,omitempty"`
	Adapter    string     `json:"adapter,omitempty"`
	Rewrites   int        `json:"rewrites,omitempty"`
}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func (r Request) Validate(now time.Time) string {
	if r.V != Protocol {
		return "protocol-mismatch"
	}
	if !validID.MatchString(r.ID) {
		return "bad-request"
	}
	switch r.Op {
	case "hello", "status", "ping", "disable":
		if r.AdoptForeign {
			return "bad-request"
		}
	case "enable":
		if r.Deadline != nil && (!r.Deadline.After(now) || r.Deadline.After(now.Add(MaxMinutes*time.Minute))) {
			return "bad-request"
		}
		if r.MinPercent != 0 && (r.MinPercent < 5 || r.MinPercent > 50) {
			return "bad-request"
		}
	default:
		return "bad-request"
	}
	return ""
}

var ErrUntrusted = errors.New("untrusted peer")
