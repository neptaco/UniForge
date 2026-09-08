//go:build !windows

package ui

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

const ttyHelperEnv = "UNIFORGE_UI_TTY_HELPER"

// TestMain lets the test binary act as a helper process whose stdio is a
// pseudo-terminal. The helper only touches the ui package the way a normal
// command invocation would.
func TestMain(m *testing.M) {
	if os.Getenv(ttyHelperEnv) == "1" {
		SetDebugMode(true)
		Debug("helper debug line")
		Error("helper error line")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestNoTerminalQueriesOnStartup verifies that using the ui package with a
// TTY attached to stderr does not send terminal queries (OSC 10/11 foreground and
// background color requests, CSI 6n cursor position requests) to the
// terminal. Such queries leak their responses into the shell's input when the
// CLI runs during shell initialisation, e.g. `eval "$(uniforge completion zsh)"`.
func TestNoTerminalQueriesOnStartup(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), ttyHelperEnv+"=1")
	// Mirror `x=$(uniforge completion zsh)`: stdout is captured (not a
	// terminal) while stderr and stdin remain attached to the terminal.
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	master, err := pty.StartWithAttrs(cmd, nil, &syscall.SysProcAttr{Setsid: true, Setctty: true})
	if err != nil {
		t.Fatalf("start helper on pty: %v", err)
	}
	defer func() { _ = master.Close() }()

	var out bytes.Buffer
	copied := make(chan struct{})
	go func() {
		_, _ = io.Copy(&out, master) // returns with EIO once the child exits
		close(copied)
	}()

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	select {
	case err := <-waitErr:
		if err != nil {
			t.Fatalf("helper exited with error: %v", err)
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("helper did not exit in time (blocked waiting for a terminal response?)")
	}
	select {
	case <-copied:
	case <-time.After(5 * time.Second):
	}

	got := out.String()
	for _, query := range []string{"\x1b]10;?", "\x1b]11;?", "\x1b[6n"} {
		if strings.Contains(got, query) {
			t.Errorf("terminal query %q was sent to the tty; output: %q", query, got)
		}
	}
	if !strings.Contains(got, "helper debug line") {
		t.Errorf("debug output missing from tty; output: %q", got)
	}
	if !strings.Contains(got, "helper error line") {
		t.Errorf("error output missing from tty; output: %q", got)
	}
}
