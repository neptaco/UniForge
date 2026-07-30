//go:build darwin

package unity

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const darwinQuitScript = `ObjC.import("AppKit");
const app = $.NSRunningApplication.runningApplicationWithProcessIdentifier(%d);
if (app.js === undefined) {
  throw new Error("application process not found");
}
if (!ObjC.unwrap(app.terminate)) {
  throw new Error("application rejected the normal quit request");
}`

// requestEditorQuit asks AppKit to terminate the exact Unity application
// instance. This follows the same application termination path as choosing
// Quit: Unity can save state, show unsaved-change prompts, or cancel shutdown.
func requestEditorQuit(process *os.Process) error {
	script := fmt.Sprintf(darwinQuitScript, process.Pid)
	output, err := exec.Command("/usr/bin/osascript", "-l", "JavaScript", "-e", script).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, message)
	}
	return nil
}
