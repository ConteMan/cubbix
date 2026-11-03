package lid

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

type CommandRunner func(context.Context, string, ...string) ([]byte, error)
type PMSet struct{ Run CommandRunner }

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
func (p PMSet) Read() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, err := p.Run(ctx, "/usr/bin/pmset", "-g")
	if err != nil {
		return -1, err
	}
	return ParseSleep(bytes.TrimSpace(data))
}
func (p PMSet) Set(v int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// No request value can become a command argument.
	switch v {
	case 0:
		_, err := p.Run(ctx, "/usr/bin/pmset", "-a", "disablesleep", "0")
		return err
	case 1:
		_, err := p.Run(ctx, "/usr/bin/pmset", "-a", "disablesleep", "1")
		return err
	}
	return errors.New("invalid sleep value")
}
