//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"syscall"
)

// ensureRuntimeDir creates the runtime directory with owner-only permissions and
// verifies that it is a real directory owned by the current user.
//
// The runtime directory holds the daemon socket, lock, and info file, so another
// local user must not be able to read it, create entries in it, or redirect it
// through a symlink.
func ensureRuntimeDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return wrapErr("prepare runtime directory", err)
	}

	// Lstat, not Stat: a symlink planted at the runtime directory path must be
	// reported as a symlink instead of being followed to its target.
	fi, err := os.Lstat(dir)
	if err != nil {
		return wrapErr("prepare runtime directory", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return wrapErr("prepare runtime directory", fmt.Errorf("runtime directory %s is a symlink", dir))
	}
	if !fi.IsDir() {
		return wrapErr("prepare runtime directory", fmt.Errorf("runtime directory %s is not a directory", dir))
	}

	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return wrapErr("prepare runtime directory", fmt.Errorf("runtime directory %s: cannot determine owner", dir))
	}
	uid := os.Getuid()
	if stat.Uid != uint32(uid) {
		return wrapErr("prepare runtime directory", fmt.Errorf("runtime directory %s is owned by uid %d, not the current user (uid %d)", dir, stat.Uid, uid))
	}

	// The directory is ours, so tighten it instead of failing: installations
	// created before this check exist with mode 0755.
	if fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return wrapErr("restrict runtime directory permissions", err)
		}
	}
	return nil
}
