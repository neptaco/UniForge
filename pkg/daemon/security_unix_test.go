//go:build !windows

package daemon

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// tempSocketDir returns a short-lived directory suitable for a Unix socket path.
// t.TempDir() is avoided here because its per-test name can push the socket path
// past the sun_path length limit on macOS.
func tempSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(os.TempDir(), "dsec-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// Note: a runtime directory owned by a different uid cannot be created without
// root, so that rejection path is not covered by an automated test.

func TestEnsureRuntimeDirCreatesPrivateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rt")

	if err := ensureRuntimeDir(dir); err != nil {
		t.Fatalf("ensureRuntimeDir: %v", err)
	}

	fi, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("lstat runtime directory: %v", err)
	}
	if !fi.IsDir() {
		t.Fatalf("runtime directory mode = %v, want a directory", fi.Mode())
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("runtime directory perm = %#o, want %#o", perm, 0o700)
	}
}

func TestEnsureRuntimeDirTightensExistingPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Defend against the process umask having cleared bits during MkdirAll.
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ensureRuntimeDir(dir); err != nil {
		t.Fatalf("ensureRuntimeDir: %v", err)
	}

	fi, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("lstat runtime directory: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("runtime directory perm = %#o, want %#o", perm, 0o700)
	}
}

func TestEnsureRuntimeDirRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "rt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := ensureRuntimeDir(link); err == nil {
		t.Fatal("ensureRuntimeDir should reject a symlinked runtime directory")
	}
}

func TestEnsureRuntimeDirRejectsRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rt")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ensureRuntimeDir(path); err == nil {
		t.Fatal("ensureRuntimeDir should reject a regular file at the runtime directory path")
	}
}

func TestListenCreatesOwnerOnlySocket(t *testing.T) {
	cfg := testConfig(t)
	d := New(cfg)
	if err := d.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer func() { _ = d.Shutdown() }()

	if _, err := d.Listen(nil); err != nil {
		t.Fatalf("listen: %v", err)
	}

	fi, err := os.Lstat(d.Info().Endpoint)
	if err != nil {
		t.Fatalf("lstat socket: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("socket perm = %#o, want %#o", perm, 0o600)
	}
}

func TestLockCreatesOwnerOnlyLockFile(t *testing.T) {
	cfg := testConfig(t)
	// A lock file left behind by an older release is world-readable; Lock must
	// tighten it rather than only setting the mode on creation.
	preexisting, err := cfg.lockPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareRuntimeDir(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preexisting, []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(preexisting, 0o644); err != nil {
		t.Fatal(err)
	}
	d := New(cfg)
	if err := d.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer func() { _ = d.Shutdown() }()

	lockPath, err := cfg.lockPath()
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(lockPath)
	if err != nil {
		t.Fatalf("lstat lock file: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("lock file perm = %#o, want %#o", perm, 0o600)
	}
}

// newTestUnixListener returns a raw Unix socket listener and its endpoint path.
func newTestUnixListener(t *testing.T) (net.Listener, string) {
	t.Helper()
	endpoint := filepath.Join(tempSocketDir(t), "s.sock")
	raw, err := net.Listen("unix", endpoint)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	return raw, endpoint
}

func TestPeerCheckedListenerAcceptsSameUID(t *testing.T) {
	raw, endpoint := newTestUnixListener(t)
	ln := newPeerCheckedListener(raw, uint32(os.Getuid()))
	defer func() { _ = ln.Close() }()

	type acceptResult struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := ln.Accept()
		accepted <- acceptResult{conn: conn, err: err}
	}()

	client := testDial(t, endpoint)
	defer func() { _ = client.Close() }()

	var result acceptResult
	select {
	case result = <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("Accept did not return a connection from the current uid")
	}
	if result.err != nil {
		t.Fatalf("accept: %v", result.err)
	}
	defer func() { _ = result.conn.Close() }()

	if _, err := result.conn.Write([]byte{7}); err != nil {
		t.Fatalf("server write: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if buf[0] != 7 {
		t.Errorf("client read %d, want 7", buf[0])
	}
}

func TestPeerCheckedListenerRejectsOtherUID(t *testing.T) {
	raw, endpoint := newTestUnixListener(t)
	// No other uid is available without root, so invert the test instead: allow
	// a uid the connecting process cannot have.
	ln := newPeerCheckedListener(raw, uint32(os.Getuid())+1)
	defer func() { _ = ln.Close() }()

	acceptErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
			acceptErr <- errors.New("Accept returned a connection from a rejected uid")
			return
		}
		acceptErr <- err
	}()

	client := testDial(t, endpoint)
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	if _, err := client.Read(buf); !errors.Is(err, io.EOF) {
		t.Fatalf("client read after rejection = %v, want EOF", err)
	}
	_ = client.Close()

	// The rejection must not have stopped the accept loop: it only ends once the
	// listener itself is closed.
	select {
	case err := <-acceptErr:
		t.Fatalf("Accept returned before Close: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	select {
	case err := <-acceptErr:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Accept error after Close = %v, want net.ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Accept did not return after the listener was closed")
	}
}
