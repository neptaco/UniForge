//go:build linux

package daemon

import "golang.org/x/sys/unix"

// peerUID returns the uid of the peer connected to the given socket fd.
func peerUID(fd int) (uint32, error) {
	ucred, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return 0, err
	}
	return ucred.Uid, nil
}
