package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open [project]",
	Short: "Open Unity Editor with a project",
	Long: `Open Unity Editor with the specified project in GUI mode.
The Editor version is automatically detected from the project's ProjectVersion.txt.

Examples:
  # Open current directory as Unity project
  uniforge open

  # Open a specific project
  uniforge open /path/to/project`,
	Args: cobra.MaximumNArgs(1),
	RunE: runOpen,
}

func init() {
	rootCmd.AddCommand(openCmd)
}

func runOpen(cmd *cobra.Command, args []string) error {
	projectPath := "."
	if len(args) > 0 {
		projectPath = args[0]
	}

	project, err := unity.LoadProject(projectPath)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	err = ui.WithSpinnerNoResult("Starting Unity Editor...", func() error {
		editor := unity.NewEditor(project.UnityVersion)
		return editor.Open(project.Path)
	})
	if err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	ui.Success("Unity Editor %s started for project: %s", project.UnityVersion, project.Name)
	return nil
}
