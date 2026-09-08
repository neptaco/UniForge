//go:build windows

package daemon

import "os"

// ensureRuntimeDir creates the runtime directory.
//
// Windows keeps the runtime files under LOCALAPPDATA, which is already isolated
// per user by ACLs, so there is no owner or permission-bit check to perform.
func ensureRuntimeDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return wrapErr("prepare runtime directory", err)
	}
	return nil
}
