package cmd

import (
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

func addOutputFlag(cmd *cobra.Command, target *string, description string) {
	cmd.Flags().StringVarP(target, "output", "o", "", description)
}

func resolveOutputOrDefault(output, defaultValue string) string {
	if output == "" {
		return defaultValue
	}
	return output
}

// statusPrinter emits human-readable progress commentary.
//
// ui.Info/Muted/Warn all write to stdout, which is also where machine-readable
// output goes. Rather than guarding every call site — where one forgotten
// guard corrupts the stream — commands hold a printer that knows the mode.
type statusPrinter struct {
	enabled bool
}

func newStatusPrinter(output string) statusPrinter {
	return statusPrinter{enabled: output != "json"}
}

func (p statusPrinter) Info(format string, args ...any) {
	if p.enabled {
		ui.Info(format, args...)
	}
}

func (p statusPrinter) Muted(format string, args ...any) {
	if p.enabled {
		ui.Muted(format, args...)
	}
}

// Warn always reaches the user, in either mode: ui.Warn writes to stderr, so
// it cannot corrupt machine-readable output on stdout.
func (p statusPrinter) Warn(format string, args ...any) {
	ui.Warn(format, args...)
}
