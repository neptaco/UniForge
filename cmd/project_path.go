package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var projectPathCmd = &cobra.Command{
	Use:   "path <project>",
	Short: "Print project path",
	Long: `Print the filesystem path of a project.

The project can be specified by name (partial match) or index (1-based).

Examples:
  # Get path by project name
  uniforge project path my-project

  # Get path by index
  uniforge project path 1

  # Use in shell commands
  cd $(uniforge project path my-project)
  code $(uniforge project path my-project)`,
	Args: cobra.ExactArgs(1),
	RunE: runProjectPath,
}

func init() {
	projectCmd.AddCommand(projectPathCmd)
}

func runProjectPath(cmd *cobra.Command, args []string) error {
	project, err := findHubProject(args[0])
	if err != nil {
		return fmt.Errorf("failed to find project: %w", err)
	}

	fmt.Println(project.Path)
	return nil
}
