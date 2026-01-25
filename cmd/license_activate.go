package cmd

import (
	"fmt"
	"os"

	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/license"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var (
	licenseUsername string
	licensePassword string
	licenseSerial   string
	licenseVersion  string
	licenseTimeout  int
)

var licenseActivateCmd = &cobra.Command{
	Use:   "activate",
	Short: "Activate Unity license",
	Long: `Activate Unity license.

For Personal license, only username and password are required.
For Plus/Pro license, serial key is also required.

Credentials can be provided via flags or environment variables:
  UNITY_USERNAME  - Unity ID email
  UNITY_PASSWORD  - Password
  UNITY_SERIAL    - Serial key (required for Plus/Pro only)

Examples:
  # Activate Personal license (no serial needed)
  export UNITY_USERNAME=user@example.com
  export UNITY_PASSWORD=password
  uniforge license activate

  # Activate Plus/Pro license (serial required)
  export UNITY_USERNAME=user@example.com
  export UNITY_PASSWORD=password
  export UNITY_SERIAL=XXXX-XXXX-XXXX-XXXX
  uniforge license activate

  # Specify Unity version
  uniforge license activate --version 2022.3.10f1`,
	RunE: runLicenseActivate,
}

func init() {
	licenseCmd.AddCommand(licenseActivateCmd)

	licenseActivateCmd.Flags().StringVarP(&licenseUsername, "username", "u", "", "Unity ID email (or UNITY_USERNAME env)")
	licenseActivateCmd.Flags().StringVarP(&licensePassword, "password", "p", "", "Password (or UNITY_PASSWORD env)")
	licenseActivateCmd.Flags().StringVarP(&licenseSerial, "serial", "s", "", "Serial key for Plus/Pro license (or UNITY_SERIAL env)")
	licenseActivateCmd.Flags().StringVar(&licenseVersion, "version", "", "Unity version to use for activation")
	licenseActivateCmd.Flags().IntVar(&licenseTimeout, "timeout", 300, "Timeout in seconds")
}

func runLicenseActivate(cmd *cobra.Command, args []string) error {
	// Get credentials from flags or environment
	username := getCredential(licenseUsername, "UNITY_USERNAME")
	password := getCredential(licensePassword, "UNITY_PASSWORD")
	serial := getCredential(licenseSerial, "UNITY_SERIAL")

	// Warn if password is provided via flag
	if licensePassword != "" {
		ui.Warn("Password provided via flag is visible in shell history. Consider using UNITY_PASSWORD environment variable instead.")
	}

	// Validate credentials
	if username == "" {
		return fmt.Errorf("username is required (use --username or UNITY_USERNAME env)")
	}
	if password == "" {
		return fmt.Errorf("password is required (use --password or UNITY_PASSWORD env)")
	}
	// Note: serial is optional for Personal license, required for Plus/Pro

	// Get Unity Editor path
	editorPath, err := getEditorPath(licenseVersion)
	if err != nil {
		return err
	}

	if serial != "" {
		ui.Info("Activating Unity license (Plus/Pro)...")
	} else {
		ui.Info("Activating Unity license (Personal)...")
	}
	ui.Muted("Using editor: %s", editorPath)

	manager := license.NewManager(editorPath, licenseTimeout)
	if err := manager.Activate(license.ActivateOptions{
		Username: username,
		Password: password,
		Serial:   serial,
	}); err != nil {
		return err
	}

	ui.Success("License activated successfully")
	return nil
}

func getCredential(flagValue, envName string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv(envName)
}

func getEditorPath(version string) (string, error) {
	hubClient := hub.NewClient()

	if version != "" {
		// Use specific version
		installed, path, err := hubClient.IsEditorInstalled(version)
		if err != nil {
			return "", fmt.Errorf("failed to check editor installation: %w", err)
		}
		if !installed {
			return "", fmt.Errorf("unity %s is not installed", version)
		}
		return path, nil
	}

	// Get any installed editor
	editors, err := hubClient.ListInstalledEditors()
	if err != nil {
		return "", fmt.Errorf("failed to list installed editors: %w", err)
	}
	if len(editors) == 0 {
		return "", fmt.Errorf("no Unity editors installed. Install one with: uniforge editor install <version>")
	}

	// Use the first available editor
	return editors[0].Path, nil
}
