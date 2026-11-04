//go:build darwin

package cli

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

// The kernel fills this struct; its layout is the SDK's sys/ucred.h.
func TestXucredLayout(t *testing.T) {
	var cred xucred
	if size := unsafe.Sizeof(cred); size != 76 {
		t.Fatalf("xucred size = %d, want 76", size)
	}
	offsets := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"cr_version", unsafe.Offsetof(cred.Version), 0},
		{"cr_uid", unsafe.Offsetof(cred.UID), 4},
		{"cr_ngroups", unsafe.Offsetof(cred.NGroups), 8},
		{"cr_groups", unsafe.Offsetof(cred.Groups), 12},
	}
	for _, offset := range offsets {
		if offset.got != offset.want {
			t.Fatalf("%s offset = %d, want %d", offset.name, offset.got, offset.want)
		}
	}
}

func unixPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	accepted := make(chan *net.UnixConn, 1)
	go func() {
		conn, err := listener.AcceptUnix()
		if err == nil {
			accepted <- conn
		}
	}()
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	server := <-accepted
	t.Cleanup(func() {
		client.Close()
		if server != nil {
			server.Close()
		}
	})
	return client, server
}

func TestPeerUIDOnOwnSocket(t *testing.T) {
	client, server := unixPair(t)
	for name, conn := range map[string]*net.UnixConn{"client": client, "server": server} {
		uid, err := PeerUID(conn)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if uid != uint32(os.Getuid()) {
			t.Fatalf("%s uid = %d, want %d", name, uid, os.Getuid())
		}
	}
}

func TestPeerUIDOnClosedSocket(t *testing.T) {
	client, _ := unixPair(t)
	client.Close()
	if _, err := PeerUID(client); err == nil {
		t.Fatal("a closed socket reported a peer")
	}
}

func TestFileOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if uid, ok := FileOwner(info); !ok || uid != uint32(os.Getuid()) {
		t.Fatalf("owner = %d %v", uid, ok)
	}
}
