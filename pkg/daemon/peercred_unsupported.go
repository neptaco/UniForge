//go:build !windows && !linux && !darwin

package daemon

import "errors"

// peerUID is not implemented for this platform. Returning an error makes
// peerCheckedListener fail closed rather than accepting unverified peers.
func peerUID(fd int) (uint32, error) {
	return 0, errors.New("peer credentials are not supported on this platform")
}
