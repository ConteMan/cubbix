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
	Version             = "3"
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
)

type Request struct {
	V          int        `json:"v"`
	ID         string     `json:"id"`
	Op         string     `json:"op"`
	Client     string     `json:"client,omitempty"`
	Deadline   *time.Time `json:"deadline,omitempty"`
	MinPercent int        `json:"minPercent,omitempty"`
	Reason     string     `json:"reason,omitempty"`
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
	case "enable":
		if r.Deadline != nil && (!r.Deadline.After(now) || r.Deadline.After(now.Add(MaxMinutes*time.Minute))) {
			return "bad-request"
		}
		if r.MinPercent != 0 && (r.MinPercent < 10 || r.MinPercent > 50) {
			return "bad-request"
		}
	default:
		return "bad-request"
	}
	return ""
}

var ErrUntrusted = errors.New("untrusted peer")
