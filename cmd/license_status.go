package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/license"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var licenseStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Unity license status",
	Long: `Check the current Unity license status.

Examples:
  uniforge license status`,
	RunE: runLicenseStatus,
}

func init() {
	licenseCmd.AddCommand(licenseStatusCmd)
}

func runLicenseStatus(cmd *cobra.Command, args []string) error {
	status, err := license.GetStatus()
	if err != nil {
		return fmt.Errorf("failed to check license status: %w", err)
	}

	if status.HasLicense {
		ui.Success("License is active")
		ui.Muted("License file: %s", status.LicensePath)
	} else {
		ui.Warn("No license found")
		ui.Muted("Expected location: %s", status.LicensePath)
	}

	return nil
}
