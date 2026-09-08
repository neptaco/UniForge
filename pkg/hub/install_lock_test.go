package hub

import (
	"errors"
	"testing"
)

func TestAcquireInstallLockRejectsConcurrentSameEditor(t *testing.T) {
	dir := t.TempDir()

	release, err := acquireInstallLockAt(dir, "6000.4.11f1")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	t.Cleanup(func() {
		_ = release()
	})

	if _, err := acquireInstallLockAt(dir, "6000.4.11f1"); !errors.Is(err, ErrInstallInProgress) {
		t.Fatalf("second acquire err = %v, want ErrInstallInProgress", err)
	}
}

func TestAcquireInstallLockAllowsReacquireAfterRelease(t *testing.T) {
	dir := t.TempDir()

	release, err := acquireInstallLockAt(dir, "6000.4.11f1")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}

	release, err = acquireInstallLockAt(dir, "6000.4.11f1")
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestAcquireInstallLockIsPerVersion(t *testing.T) {
	dir := t.TempDir()

	releaseA, err := acquireInstallLockAt(dir, "6000.4.11f1")
	if err != nil {
		t.Fatalf("acquire 6000.4.11f1: %v", err)
	}
	t.Cleanup(func() {
		_ = releaseA()
	})

	// A different version must not be blocked.
	releaseB, err := acquireInstallLockAt(dir, "6000.5.5f1")
	if err != nil {
		t.Fatalf("acquire 6000.5.5f1: %v", err)
	}
	t.Cleanup(func() {
		_ = releaseB()
	})
}

// Unity Hub's install-modules identifies its target by version alone, so the
// lock must not be keyed on architecture: a module install and an editor
// install of the same version would otherwise take different locks and run
// against the same editor directory concurrently.
func TestAcquireInstallLockIgnoresArchitecture(t *testing.T) {
	dir := t.TempDir()

	release, err := acquireInstallLockAt(dir, "6000.4.11f1")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	t.Cleanup(func() {
		_ = release()
	})

	// Whatever architecture the caller resolved, the same version must block.
	if _, err := acquireInstallLockAt(dir, "6000.4.11f1"); !errors.Is(err, ErrInstallInProgress) {
		t.Fatalf("second acquire err = %v, want ErrInstallInProgress", err)
	}
}

func TestAcquireInstallLockCreatesMissingDirectory(t *testing.T) {
	dir := t.TempDir() + "/nested/locks"

	release, err := acquireInstallLockAt(dir, "6000.4.11f1")
	if err != nil {
		t.Fatalf("acquire in missing directory: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}
}
