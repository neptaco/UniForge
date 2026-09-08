//go:build darwin

package unity

import (
	"fmt"
	"strings"
	"testing"
)

func TestDarwinQuitScriptTargetsExactProcessAndUsesNormalTermination(t *testing.T) {
	script := fmt.Sprintf(darwinQuitScript, 12345)
	for _, expected := range []string{
		"runningApplicationWithProcessIdentifier(12345)",
		"app.terminate",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("quit script does not contain %q: %s", expected, script)
		}
	}
}
