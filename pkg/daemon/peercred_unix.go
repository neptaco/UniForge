//go:build !windows

package daemon

import (
	"errors"
	"net"
	"syscall"
)

// peerCheckedListener wraps a Unix domain socket listener and drops connections
// whose peer process runs under a different uid.
//
// Directory and socket permissions already keep other local users out; this is a
// second line of defense that does not depend on the filesystem state.
type peerCheckedListener struct {
	inner      net.Listener
	allowedUID uint32
}

// newPeerCheckedListener returns a listener that only yields connections whose
// peer uid equals allowedUID. Connections from any other uid, and connections
// whose credentials cannot be read, are closed and skipped: Accept keeps
// waiting for the next connection instead of failing the whole serve loop.
func newPeerCheckedListener(l net.Listener, allowedUID uint32) net.Listener {
	return &peerCheckedListener{inner: l, allowedUID: allowedUID}
}

// Accept returns the next connection from an allowed peer.
func (l *peerCheckedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.inner.Accept()
		if err != nil {
			return nil, err
		}
		uid, err := connPeerUID(conn)
		if err != nil || uid != l.allowedUID {
			// Fail closed, and stay quiet: a rejected peer must not be able to
			// spam the daemon log.
			_ = conn.Close()
			continue
		}
		return conn, nil
	}
}

// Close closes the wrapped listener.
func (l *peerCheckedListener) Close() error { return l.inner.Close() }

// Addr returns the wrapped listener's address.
func (l *peerCheckedListener) Addr() net.Addr { return l.inner.Addr() }

// connPeerUID reads the uid of the process on the other end of conn.
func connPeerUID(conn net.Conn) (uint32, error) {
	sysConn, ok := conn.(syscall.Conn)
	if !ok {
		return 0, errors.New("connection does not expose peer credentials")
	}
	raw, err := sysConn.SyscallConn()
	if err != nil {
		return 0, err
	}

	var (
		uid    uint32
		uidErr error
	)
	if err := raw.Control(func(fd uintptr) {
		uid, uidErr = peerUID(int(fd))
	}); err != nil {
		return 0, err
	}
	if uidErr != nil {
		return 0, uidErr
	}
	return uid, nil
}
