//go:build windows

package hub

import (
	"errors"
	"fmt"
	"syscall"
)

const (
	errorSharingViolation syscall.Errno = 32
	errorLockViolation    syscall.Errno = 33
)

type windowsLockHandle struct {
	handle syscall.Handle
}

func (h *windowsLockHandle) release() error {
	if err := syscall.CloseHandle(h.handle); err != nil {
		return fmt.Errorf("close install lock: %w", err)
	}
	return nil
}

func acquireFileLock(path string) (lockHandle, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("resolve install lock path: %w", err)
	}

	// dwShareMode 0 makes the handle exclusive, so a second process opening
	// the same path fails with a sharing violation.
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err == nil {
		return &windowsLockHandle{handle: handle}, nil
	}
	if errors.Is(err, errorSharingViolation) || errors.Is(err, errorLockViolation) {
		return nil, ErrInstallInProgress
	}
	return nil, fmt.Errorf("acquire install lock: %w", err)
}
