package cmd

import (
	"fmt"

	"github.com/neptaco/unity-cli/pkg/hub"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var editorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed Unity Editor versions",
	Long:  `List all installed Unity Editor versions managed by Unity Hub.`,
	RunE:  runList,
}

func init() {
	editorCmd.AddCommand(editorListCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	logrus.Debug("Listing installed Unity Editor versions")

	hubClient := hub.NewClient()
	editors, err := hubClient.ListInstalledEditors()
	if err != nil {
		return fmt.Errorf("failed to list editors: %w", err)
	}

	if len(editors) == 0 {
		fmt.Println("No Unity Editor installations found")
		return nil
	}

	fmt.Println("Installed Unity Editor versions:")
	for _, editor := range editors {
		fmt.Printf("  - %s (%s)\n", editor.Version, editor.Path)
	}

	return nil
}