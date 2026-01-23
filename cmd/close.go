package cmd

import (
	"fmt"

	"github.com/neptaco/unity-cli/pkg/unity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	closeProject string
	closeForce   bool
)

var closeCmd = &cobra.Command{
	Use:   "close",
	Short: "Close running Unity Editor",
	Long: `Close the Unity Editor that has the specified project open.
By default, sends SIGTERM for graceful shutdown. Use --force for immediate termination.

Examples:
  # Close Unity Editor for current project
  unity-cli close

  # Close with specific project path
  unity-cli close --project /path/to/project

  # Force close (SIGKILL)
  unity-cli close --force`,
	RunE: runClose,
}

func init() {
	rootCmd.AddCommand(closeCmd)

	closeCmd.Flags().StringVarP(&closeProject, "project", "p", ".", "Path to Unity project")
	closeCmd.Flags().BoolVar(&closeForce, "force", false, "Force kill the process (SIGKILL)")
}

func runClose(cmd *cobra.Command, args []string) error {
	logrus.Infof("Closing Unity Editor for project: %s", closeProject)

	project, err := unity.LoadProject(closeProject)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	editor := unity.NewEditor(project.UnityVersion)
	if err := editor.Close(project.Path, closeForce); err != nil {
		return fmt.Errorf("failed to close editor: %w", err)
	}

	fmt.Printf("Unity Editor closed for project: %s\n", project.Name)
	return nil
}
