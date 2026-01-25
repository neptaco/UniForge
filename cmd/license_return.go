package cmd

import (
	"github.com/neptaco/uniforge/pkg/license"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var (
	returnVersion string
	returnTimeout int
)

var licenseReturnCmd = &cobra.Command{
	Use:   "return",
	Short: "Return Unity license",
	Long: `Return the active Unity license.

This is typically used in CI/CD pipelines to release the license seat after builds.

Examples:
  # Return license
  uniforge license return

  # Return using specific Unity version
  uniforge license return --version 2022.3.10f1`,
	RunE: runLicenseReturn,
}

func init() {
	licenseCmd.AddCommand(licenseReturnCmd)

	licenseReturnCmd.Flags().StringVar(&returnVersion, "version", "", "Unity version to use for return")
	licenseReturnCmd.Flags().IntVar(&returnTimeout, "timeout", 300, "Timeout in seconds")
}

func runLicenseReturn(cmd *cobra.Command, args []string) error {
	// Get Unity Editor path
	editorPath, err := getEditorPath(returnVersion)
	if err != nil {
		return err
	}

	ui.Info("Returning Unity license...")
	ui.Muted("Using editor: %s", editorPath)

	manager := license.NewManager(editorPath, returnTimeout)
	if err := manager.Return(); err != nil {
		return err
	}

	ui.Success("License returned successfully")
	return nil
}
