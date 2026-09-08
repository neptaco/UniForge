package daemon

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultStartTimeout is how long to wait for a daemon to advertise its info after launch.
	DefaultStartTimeout = 10 * time.Second
	// DefaultShutdownTimeout is how long to wait for SIGTERM before escalating to SIGKILL.
	DefaultShutdownTimeout = 10 * time.Second

	pollInterval = 200 * time.Millisecond
)

// StartOptions configures how a daemon is launched as a background process.
type StartOptions struct {
	// Executable overrides the path to the daemon binary.
	// If empty, defaults to os.Executable().
	Executable string
	// Args are the command-line arguments (e.g., ["daemon", "run"]).
	Args []string
	// Env specifies additional environment variables for the subprocess ("KEY=VALUE").
	Env []string
	// WaitTimeout overrides DefaultStartTimeout for waiting on the daemon to become ready.
	WaitTimeout time.Duration
}

// IsRunning checks whether a daemon is running for the given config
// by attempting to acquire the lock file. If the lock cannot be acquired,
// another daemon holds it and is running.
func IsRunning(config Config) bool {
	_, running, _ := runningLockPID(config)
	return running
}

// runningLockPID reports the PID written by the process currently holding the
// daemon lock. A stale lock file is treated as not running.
func runningLockPID(config Config) (int, bool, error) {
	lockPath, err := config.lockPath()
	if err != nil {
		return 0, false, err
	}

	f, err := os.OpenFile(lockPath, os.O_RDONLY, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, err
	}
	defer func() { _ = f.Close() }()

	if err := lockFile(f); err == nil {
		unlockFile(f)
		return 0, false, nil
	}

	if _, err := f.Seek(0, 0); err != nil {
		return 0, true, fmt.Errorf("seek daemon lock: %w", err)
	}
	line, err := bufio.NewReader(f).ReadString('\n')
	if err != nil && len(line) == 0 {
		return 0, true, fmt.Errorf("read daemon lock PID: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || pid <= 0 {
		return 0, true, fmt.Errorf("invalid daemon lock PID %q", strings.TrimSpace(line))
	}
	return pid, true, nil
}

// Stop sends SIGTERM to the running daemon, waits for graceful shutdown,
// then escalates to SIGKILL if needed. Cleans up the info file.
// Returns nil if no daemon is running.
func Stop(ctx context.Context, config Config) error {
	lock, err := acquireLifecycleLock(ctx, config)
	if err != nil {
		return err
	}
	defer releaseLifecycleLock(lock)
	return stopUnlocked(ctx, config)
}

func stopUnlocked(ctx context.Context, config Config) error {
	lockPID, running, err := stopTargetPID(config)
	if err != nil {
		return err
	}
	if !running {
		_ = removeInfo(config)
		return nil
	}
	if err := stopProcess(ctx, lockPID); err != nil {
		return err
	}

	return removeInfoIfPID(config, lockPID)
}

func stopTargetPID(config Config) (int, bool, error) {
	lockPID, running, err := runningLockPID(config)
	if err != nil || !running {
		return lockPID, running, err
	}
	info, infoErr := ReadInfo(config)
	if infoErr != nil && !errors.Is(infoErr, os.ErrNotExist) {
		return 0, true, infoErr
	}
	if info != nil && lockPID != info.PID {
		return 0, true, fmt.Errorf("daemon state changed: info PID %d does not match lock owner PID %d", info.PID, lockPID)
	}
	return lockPID, true, nil
}

// Start launches the daemon as a background process.
// It waits for the daemon to advertise its info file before returning.
// Returns nil if a daemon is already running.
func Start(ctx context.Context, config Config, opts StartOptions) error {
	lock, err := acquireLifecycleLock(ctx, config)
	if err != nil {
		return err
	}
	defer releaseLifecycleLock(lock)
	return startUnlocked(ctx, config, opts)
}

// Restart stops the current daemon and starts a replacement while holding one
// lifecycle lock for the complete operation.
func Restart(ctx context.Context, config Config, opts StartOptions) error {
	lock, err := acquireLifecycleLock(ctx, config)
	if err != nil {
		return err
	}
	defer releaseLifecycleLock(lock)

	if err := stopUnlocked(ctx, config); err != nil {
		return err
	}
	return startUnlocked(ctx, config, opts)
}

func startUnlocked(ctx context.Context, config Config, opts StartOptions) error {
	if IsRunning(config) {
		return nil
	}
	if err := removeInfo(config); err != nil {
		return fmt.Errorf("remove stale daemon info: %w", err)
	}

	executablePath := opts.Executable
	if executablePath == "" {
		var err error
		executablePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable: %w", err)
		}
	}

	if err := prepareRuntimeDir(config); err != nil {
		return err
	}
	if err := ensureDir(config.stateDir); err != nil {
		return err
	}

	logPath, err := config.logPath()
	if err != nil {
		return err
	}

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	// The daemon outlives the command that starts it. Use ctx only while waiting
	// for readiness; binding the child to ctx would kill it as soon as a CLI
	// command such as `update` returns and cancels its request context.
	cmd := exec.Command(executablePath, opts.Args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = processAttrs()
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- cmd.Wait()
	}()

	timeout := opts.WaitTimeout
	if timeout <= 0 {
		timeout = DefaultStartTimeout
	}

	// Poll for info file to confirm the daemon is ready
	for waited := time.Duration(0); waited < timeout; waited += pollInterval {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-waitErr:
			exitState := "unknown"
			if cmd.ProcessState != nil {
				exitState = cmd.ProcessState.String()
			} else if err != nil {
				exitState = err.Error()
			}
			return fmt.Errorf("daemon process exited before advertising its endpoint (exit: %s); see log: %s", exitState, logPath)
		case <-time.After(pollInterval):
		}
		if info, err := ReadInfo(config); err == nil && info != nil && info.PID == cmd.Process.Pid {
			// The Wait goroutine intentionally remains for the daemon lifetime after
			// readiness. The starter is a short-lived CLI process, and Wait also
			// ensures an early child exit is reaped while that process is alive.
			return nil
		}
	}

	return errors.New("daemon did not advertise its endpoint in time")
}

func acquireLifecycleLock(ctx context.Context, config Config) (*os.File, error) {
	if err := prepareRuntimeDir(config); err != nil {
		return nil, err
	}
	path, err := config.lifecycleLockPath()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		if err := lockFile(f); err == nil {
			return f, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func releaseLifecycleLock(f *os.File) {
	if f == nil {
		return
	}
	unlockFile(f)
	_ = f.Close()
}

// Dial connects to the running daemon using the transport advertised in its info file.
func Dial(config Config, timeout time.Duration) (net.Conn, error) {
	info, err := ReadInfo(config)
	if err != nil {
		return nil, fmt.Errorf("read daemon info: %w", err)
	}

	return dialTransport(*info, timeout)
}

// stopProcess attempts graceful shutdown, then escalates to force kill.
// On Unix: SIGTERM → wait → SIGKILL.
// On Windows: immediate Kill (no graceful shutdown for detached processes).
func stopProcess(ctx context.Context, pid int) error {
	if !isProcessAlive(pid) {
		return nil
	}

	// Try graceful shutdown (SIGTERM on Unix, no-op on Windows)
	if err := gracefulStop(pid); err == nil {
		// Wait for graceful exit
		for waited := time.Duration(0); waited < DefaultShutdownTimeout; waited += pollInterval {
			select {
			case <-ctx.Done():
				_ = forceStop(pid)
				return ctx.Err()
			case <-time.After(pollInterval):
			}
			if !isProcessAlive(pid) {
				return nil
			}
		}
	}

	// Escalate to force kill
	if err := forceStop(pid); err != nil {
		return err
	}

	// Wait briefly for the process to fully terminate
	for waited := time.Duration(0); waited < 2*time.Second; waited += pollInterval {
		time.Sleep(pollInterval)
		if !isProcessAlive(pid) {
			return nil
		}
	}

	return fmt.Errorf("daemon (pid %d) did not stop after force kill", pid)
}
