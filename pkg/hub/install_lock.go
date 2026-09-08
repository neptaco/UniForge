package hub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neptaco/uniforge/pkg/platform"
	"github.com/neptaco/uniforge/pkg/ui"
)

// ErrInstallInProgress reports that another process is already installing the
// same editor.
var ErrInstallInProgress = errors.New("another uniforge process is already installing this editor")

// lockHandle is an OS-level exclusive lock held on a file. Implementations
// live in the platform-specific files.
type lockHandle interface {
	release() error
}

// withEditorLock runs fn while holding an exclusive lock on the given editor.
//
// Unity Hub holds a cross-process lock for the install phase only, not for the
// download phase, so two concurrent runs download the same package twice and
// then install it twice — the second one landing in a `<version>-<architecture>`
// directory because the plain one is already taken. Locking here covers the
// whole download-and-install sequence, and covers installing modules too, since
// that writes into the same editor directory.
//
// The lock is keyed on version alone: Unity Hub's install-modules identifies
// its target by version only, so keying on architecture as well would let a
// module install and an editor install of the same version take different
// locks and run concurrently against the same editor.
//
// The OS lock is tied to the file handle, so it is dropped automatically if the
// process dies and never goes stale.
func (c *Client) withEditorLock(version string, fn func() error) error {
	release, err := c.acquireEditorLock(version)
	if err != nil {
		if errors.Is(err, ErrInstallInProgress) {
			// Callers decide whether to fail or wait, and how to word it.
			return fmt.Errorf("editor %s: %w", version, ErrInstallInProgress)
		}
		return err
	}
	defer func() {
		if err := release(); err != nil {
			ui.Debug("Failed to release install lock", "error", err)
		}
	}()

	return fn()
}

func (c *Client) acquireEditorLock(version string) (func() error, error) {
	dir := c.lockDirOverride
	if dir == "" {
		runtimeDir, err := platform.RuntimeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve runtime directory: %w", err)
		}
		dir = filepath.Join(runtimeDir, "locks")
	}
	return acquireInstallLockAt(dir, version)
}

func acquireInstallLockAt(dir, version string) (func() error, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	name := "install-" + sanitizeLockComponent(version) + ".lock"

	handle, err := acquireFileLock(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	return handle.release, nil
}

// sanitizeLockComponent keeps a version or architecture string usable as a
// single path element.
func sanitizeLockComponent(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, s)
}
