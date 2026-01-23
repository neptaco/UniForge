package cmd

import (
	"fmt"

	"github.com/neptaco/unity-cli/pkg/unity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	openProject string
)

var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open Unity Editor with a project",
	Long: `Open Unity Editor with the specified project in GUI mode.
The Editor version is automatically detected from the project's ProjectVersion.txt.

Examples:
  # Open current directory as Unity project
  unity-cli open

  # Open a specific project
  unity-cli open --project /path/to/project`,
	RunE: runOpen,
}

func init() {
	rootCmd.AddCommand(openCmd)

	openCmd.Flags().StringVarP(&openProject, "project", "p", ".", "Path to Unity project")
}

func runOpen(cmd *cobra.Command, args []string) error {
	logrus.Infof("Opening Unity project: %s", openProject)

	project, err := unity.LoadProject(openProject)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	editor := unity.NewEditor(project.UnityVersion)
	if err := editor.Open(project.Path); err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	fmt.Printf("Unity Editor %s started for project: %s\n", project.UnityVersion, project.Name)
	return nil
}
