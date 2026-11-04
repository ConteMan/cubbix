package lid

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CommandRunner func(context.Context, string, ...string) ([]byte, error)
type PMSet struct {
	Run       CommandRunner
	Logf      func(string, ...any)
	mu        sync.Mutex
	readKnown bool
	readValue int
}

func ParseSleep(data []byte) (int, error) {
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "SleepDisabled" {
			v, err := strconv.Atoi(f[1])
			if err == nil && (v == 0 || v == 1) {
				return v, nil
			}
		}
	}
	return -1, errors.New("sleep setting unavailable")
}
func (p *PMSet) Read() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, err := p.Run(ctx, "/usr/bin/pmset", "-g")
	if err != nil {
		p.logRead("command-failed", -1)
		return -1, err
	}
	v, err := ParseSleep(bytes.TrimSpace(data))
	result := "ok"
	if err != nil {
		result = "parse-failed"
	}
	p.logRead(result, v)
	return v, err
}
func (p *PMSet) logRead(result string, v int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Logf != nil && (result != "ok" || !p.readKnown || p.readValue != v) {
		p.Logf("pmset read result=%s value=%d", result, v)
	}
	p.readKnown, p.readValue = result == "ok", v
}
func (p *PMSet) Set(v int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// No request value can become a command argument.
	var err error
	switch v {
	case 0:
		_, err = p.Run(ctx, "/usr/bin/pmset", "-a", "disablesleep", "0")
	case 1:
		_, err = p.Run(ctx, "/usr/bin/pmset", "-a", "disablesleep", "1")
	default:
		return errors.New("invalid sleep value")
	}
	if p.Logf != nil {
		result := "ok"
		if err != nil {
			result = "command-failed"
		}
		p.Logf("pmset write value=%d result=%s", v, result)
	}
	return err
}
