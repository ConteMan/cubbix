//go:build !darwin

package cli

import (
	"fmt"
	"net"
	"os"
)

// PeerUID is only implemented on macOS, where Cubbix runs.
func PeerUID(*net.UnixConn) (uint32, error) {
	return 0, fmt.Errorf("cli: peer credentials are not available on this platform")
}

func ownerUID(os.FileInfo) (uint32, bool) { return 0, false }

// FileOwner reports the uid owning a file, where the platform has one.
func FileOwner(info os.FileInfo) (uint32, bool) { return ownerUID(info) }
