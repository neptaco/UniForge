package cmd

import (
	"os"

	"github.com/mattn/go-isatty"
	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage Unity Hub projects",
	Long: `Manage Unity projects registered in Unity Hub.

When run without arguments in an interactive terminal, launches a TUI
for browsing and opening projects.

Examples:
  # Launch interactive TUI
  uniforge project

  # List all projects
  uniforge project list

  # Open a project in Unity
  uniforge project open my-project

  # Get project path (for shell scripts)
  uniforge project path my-project`,
	RunE: runProjectTUI,
}

func init() {
	rootCmd.AddCommand(projectCmd)
}

func runProjectTUI(cmd *cobra.Command, args []string) error {
	// If not a TTY, show list instead
	if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		return runProjectList(cmd, args)
	}

	hubClient := hub.NewClient()

	// Open function that uses the unity package
	openFn := func(path, version string) error {
		editor := unity.NewEditor(version)
		return editor.Open(path)
	}

	return hub.RunProjectTUI(hubClient, openFn)
}
