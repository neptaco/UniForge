package cmd

import (
	"fmt"
	"time"

	"github.com/neptaco/unity-cli/pkg/unity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	restartProject string
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart Unity Editor",
	Long: `Restart the Unity Editor for the specified project.
This closes the running Editor and opens it again.

Examples:
  # Restart Unity Editor for current project
  unity-cli restart

  # Restart with specific project path
  unity-cli restart --project /path/to/project`,
	RunE: runRestart,
}

func init() {
	rootCmd.AddCommand(restartCmd)

	restartCmd.Flags().StringVarP(&restartProject, "project", "p", ".", "Path to Unity project")
}

func runRestart(cmd *cobra.Command, args []string) error {
	logrus.Infof("Restarting Unity Editor for project: %s", restartProject)

	project, err := unity.LoadProject(restartProject)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	editor := unity.NewEditor(project.UnityVersion)

	// Try to close existing instance (ignore error if not running)
	if err := editor.Close(project.Path, false); err != nil {
		logrus.Debugf("No running editor found or close failed: %v", err)
	} else {
		// Wait a moment for the editor to fully close
		time.Sleep(2 * time.Second)
	}

	// Open editor
	if err := editor.Open(project.Path); err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	fmt.Printf("Unity Editor %s restarted for project: %s\n", project.UnityVersion, project.Name)
	return nil
}
