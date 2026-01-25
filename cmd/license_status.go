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

Checks the following license types:
  - Serial license (Unity_lic.ulf file)
  - Unity Hub login
  - Unity Licensing Server (via UNITY_LICENSING_SERVER env or services-config.json)

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
		switch status.LicenseType {
		case license.LicenseTypeSerial:
			ui.Success("License is active (Serial)")
			ui.Muted("License file: %s", status.LicensePath)
		case license.LicenseTypeHub:
			ui.Success("License is active (Unity Hub)")
			ui.Muted("Logged in via Unity Hub")
		case license.LicenseTypeServer:
			ui.Success("License is active (Licensing Server)")
			ui.Muted("Server: %s", status.ServerURL)
		case license.LicenseTypeBuildServer:
			ui.Success("License is active (Build Server)")
			ui.Muted("Server: %s", status.ServerURL)
		}
	} else {
		ui.Warn("No license found")
		fmt.Println()
		fmt.Println("Checked the following license sources:")
		fmt.Printf("  Serial license: %s\n", status.LicensePath)
		fmt.Printf("  Unity Hub:      %s\n", status.HubConfigPath)
		fmt.Println("  License Server: (not configured)")
		fmt.Println()
		fmt.Println("To activate a license:")
		fmt.Println("  - Login via Unity Hub")
		fmt.Println("  - Use 'uniforge license activate' with serial key")
		fmt.Println("  - Configure UNITY_LICENSING_SERVER environment variable")
	}

	return nil
}
