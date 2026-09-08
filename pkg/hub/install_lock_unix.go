//go:build darwin || linux

package hub

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type unixLockHandle struct {
	file *os.File
}

func (h *unixLockHandle) release() error {
	if err := syscall.Flock(int(h.file.Fd()), syscall.LOCK_UN); err != nil {
		_ = h.file.Close()
		return fmt.Errorf("release install lock: %w", err)
	}
	if err := h.file.Close(); err != nil {
		return fmt.Errorf("close install lock: %w", err)
	}
	return nil
}

func acquireFileLock(path string) (lockHandle, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open install lock: %w", err)
	}

	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return &unixLockHandle{file: file}, nil
	}

	if closeErr := file.Close(); closeErr != nil {
		return nil, fmt.Errorf("close install lock after failed acquire: %w", closeErr)
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return nil, ErrInstallInProgress
	}
	return nil, fmt.Errorf("acquire install lock: %w", err)
}
