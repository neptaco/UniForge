package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var (
	closeForce bool
)

var closeCmd = &cobra.Command{
	Use:   "close [project]",
	Short: "Close running Unity Editor",
	Long: `Close the Unity Editor that has the specified project open.
By default, sends SIGTERM for graceful shutdown. Use --force for immediate termination.

Examples:
  # Close Unity Editor for current project
  uniforge close

  # Close with specific project path
  uniforge close /path/to/project

  # Force close (SIGKILL)
  uniforge close --force`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClose,
}

func init() {
	rootCmd.AddCommand(closeCmd)

	closeCmd.Flags().BoolVar(&closeForce, "force", false, "Force kill the process (SIGKILL)")
}

func runClose(cmd *cobra.Command, args []string) error {
	projectPath := "."
	if len(args) > 0 {
		projectPath = args[0]
	}

	project, err := unity.LoadProject(projectPath)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	err = ui.WithSpinnerNoResult("Closing Unity Editor...", func() error {
		editor := unity.NewEditor(project.UnityVersion)
		return editor.Close(project.Path, closeForce)
	})
	if err != nil {
		return fmt.Errorf("failed to close editor: %w", err)
	}

	ui.Success("Unity Editor closed for project: %s", project.Name)
	return nil
}
