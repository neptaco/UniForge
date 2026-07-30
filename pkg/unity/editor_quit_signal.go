//go:build !darwin

package unity

import (
	"os"
	"syscall"
)

// Non-macOS platforms retain their existing graceful signal request. Unlike
// the old Close flow, a timeout never escalates to a force kill unless the
// caller explicitly requested --force.
func requestEditorQuit(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
