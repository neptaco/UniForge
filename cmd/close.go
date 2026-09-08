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
By default, requests a normal application quit and respects unsaved-change prompts.
It never escalates to a force kill unless --force is explicitly specified.

Examples:
  # Close Unity Editor for current project
  uniforge close

  # Close with specific project path
  uniforge close /path/to/project

  # Force close immediately, bypassing Unity's normal shutdown
  uniforge close --force`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClose,
}

func init() {
	rootCmd.AddCommand(closeCmd)

	closeCmd.Flags().BoolVar(&closeForce, "force", false, "Force kill the process (SIGKILL)")
}

func runClose(cmd *cobra.Command, args []string) error {
	project, err := resolveLoadedProjectArg(args)
	if err != nil {
		return err
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
