//go:build !windows

package daemon

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"
)

// lockFile acquires a non-blocking exclusive flock.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// unlockFile releases a previously acquired flock.
func unlockFile(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// createListener creates a Unix domain socket listener restricted to the
// current user: the socket is chmod'ed to 0600 and every accepted connection is
// checked against the current uid.
func createListener(info Info) (net.Listener, error) {
	if info.Transport == TransportUnix {
		_ = os.Remove(info.Endpoint) // clean up stale socket
	}
	listener, err := net.Listen("unix", info.Endpoint)
	if err != nil {
		return nil, err
	}
	// The runtime directory is already 0700, so nothing can reach the socket
	// between Listen and Chmod. Narrow the socket itself anyway, as a second
	// line of defense if the directory mode is ever loosened.
	if err := os.Chmod(info.Endpoint, 0o600); err != nil {
		_ = listener.Close()
		return nil, wrapErr("restrict socket permissions", err)
	}
	return newPeerCheckedListener(listener, uint32(os.Getuid())), nil
}

// dialTransport connects to the daemon using the advertised transport.
func dialTransport(info Info, timeout time.Duration) (net.Conn, error) {
	switch info.Transport {
	case TransportUnix:
		return net.DialTimeout("unix", info.Endpoint, timeout)
	case TransportTCP:
		address := net.JoinHostPort(info.Host, fmt.Sprintf("%d", info.Port))
		return net.DialTimeout("tcp", address, timeout)
	default:
		return nil, fmt.Errorf("unsupported transport on this platform: %s", info.Transport)
	}
}

// processAttrs returns SysProcAttr for launching a daemon subprocess.
func processAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// isProcessAlive checks if a process with the given PID is alive.
func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// gracefulStop sends SIGTERM to the process for a graceful shutdown.
func gracefulStop(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}

// forceStop sends SIGKILL to the process.
func forceStop(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func uidString() string { return strconv.Itoa(os.Getuid()) }
