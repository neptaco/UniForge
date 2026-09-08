//go:build darwin

package daemon

import "golang.org/x/sys/unix"

// peerUID returns the uid of the peer connected to the given socket fd.
func peerUID(fd int) (uint32, error) {
	xucred, err := unix.GetsockoptXucred(fd, unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	if err != nil {
		return 0, err
	}
	return xucred.Uid, nil
}
