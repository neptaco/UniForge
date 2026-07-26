package ui

import (
	"errors"
	"io"
	"os"
	"testing"
)

func withTTY(t *testing.T, tty bool) {
	t.Helper()

	original := isTTY
	isTTY = func() bool { return tty }
	t.Cleanup(func() { isTTY = original })
}

// Callers rely on WithSpinner to run the task and return its result whether or
// not a terminal is attached; on a non-TTY it must skip the bubbletea program
// rather than emit animation frames into a pipe.
func TestWithSpinnerRunsTaskWithoutTTY(t *testing.T) {
	withTTY(t, false)

	ran := false
	got, err := WithSpinner("working", func() (string, error) {
		ran = true
		return "done", nil
	})
	if err != nil {
		t.Fatalf("WithSpinner: %v", err)
	}
	if !ran {
		t.Error("task did not run")
	}
	if got != "done" {
		t.Errorf("got %q, want %q", got, "done")
	}
}

func TestWithSpinnerPropagatesErrorWithoutTTY(t *testing.T) {
	withTTY(t, false)

	wantErr := errors.New("boom")
	_, err := WithSpinner("working", func() (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// stdout carries the command's result, including machine-readable output, so
// warnings must not land there.
func TestWarnWritesToStderrNotStdout(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() {
		_ = read.Close()
	}()

	original := os.Stdout
	os.Stdout = write
	t.Cleanup(func() { os.Stdout = original })

	Warn("something looks off: %s", "detail")

	if err := write.Close(); err != nil {
		t.Fatalf("close write end: %v", err)
	}
	captured, err := io.ReadAll(read)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	if len(captured) != 0 {
		t.Errorf("Warn wrote %q to stdout, want it on stderr", captured)
	}
}

func TestIsTTYReportsTheDetectedMode(t *testing.T) {
	withTTY(t, false)
	if IsTTY() {
		t.Error("IsTTY() = true, want false")
	}

	withTTY(t, true)
	if !IsTTY() {
		t.Error("IsTTY() = false, want true")
	}
}
