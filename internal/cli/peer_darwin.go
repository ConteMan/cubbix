//go:build darwin

package cli

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"unsafe"
)

// SOL_LOCAL and LOCAL_PEERCRED from the SDK's sys/un.h.
const (
	solLocal      = 0
	localPeercred = 0x001
)

// xucred from the SDK's sys/ucred.h: version, uid, group count, padding, then
// the groups. 76 bytes in all, with cr_groups at offset 12.
type xucred struct {
	Version uint32
	UID     uint32
	NGroups int16
	_       uint16
	Groups  [16]uint32
}

// PeerUID returns the uid of the process on the other end of conn.
func PeerUID(conn *net.UnixConn) (uint32, error) {
	if conn == nil {
		return 0, fmt.Errorf("cli: no connection")
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var (
		cred    xucred
		size    = uint32(unsafe.Sizeof(cred))
		callErr error
	)
	if err := raw.Control(func(fd uintptr) {
		_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, solLocal, localPeercred,
			uintptr(unsafe.Pointer(&cred)), uintptr(unsafe.Pointer(&size)), 0)
		if errno != 0 {
			callErr = errno
		}
	}); err != nil {
		return 0, err
	}
	if callErr != nil {
		return 0, callErr
	}
	if size < uint32(unsafe.Offsetof(cred.Groups)) {
		return 0, fmt.Errorf("cli: peer credential too short (%d bytes)", size)
	}
	return cred.UID, nil
}

// ownerUID returns the owner of a file.
func ownerUID(info os.FileInfo) (uint32, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, false
	}
	return stat.Uid, true
}

// FileOwner reports the uid owning a file, where the platform has one.
func FileOwner(info os.FileInfo) (uint32, bool) { return ownerUID(info) }
