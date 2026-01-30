package cmd

import (
	"errors"
	"fmt"

	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open [project]",
	Short: "Open Unity Editor with a project",
	Long: `Open Unity Editor with the specified project in GUI mode.
The Editor version is automatically detected from the project's ProjectVersion.txt.

If the argument is not a valid project path, it will search Unity Hub's
registered projects by name.

Examples:
  # Open current directory as Unity project
  uniforge open

  # Open a specific project by path
  uniforge open /path/to/project

  # Open a project by name (searches Unity Hub projects)
  uniforge open my-project`,
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

	// First, try to load as a path
	project, err := unity.LoadProject(projectPath)
	if err != nil {
		// If path loading fails and an argument was provided, try Unity Hub projects
		if len(args) > 0 {
			hubProject, hubErr := findHubProject(args[0])
			if hubErr == nil && hubProject != nil {
				ui.Info("Found project in Unity Hub: %s", hubProject.Title)
				return openProject(hubProject.Path, hubProject.Version, hubProject.Title)
			}
			if hubErr != nil {
				// Return Hub error if it's more specific than "not found"
				var multiErr *hub.MultipleMatchError
				if errors.As(hubErr, &multiErr) {
					return hubErr
				}
			}
		}
		return fmt.Errorf("failed to load project: %w", err)
	}

	return openProject(project.Path, project.UnityVersion, project.Name)
}

// findHubProject searches Unity Hub projects and handles multiple matches with selection UI
func findHubProject(query string) (*hub.ProjectInfo, error) {
	hubClient := hub.NewClient()
	hubProject, err := hubClient.GetProject(query)
	if err == nil {
		return hubProject, nil
	}

	// Check for multiple matches
	var multiErr *hub.MultipleMatchError
	if errors.As(err, &multiErr) {
		return selectProject(multiErr.Matches, query)
	}

	return nil, err
}

// selectProject displays a selection UI for multiple matching projects
func selectProject(matches []hub.ProjectInfo, query string) (*hub.ProjectInfo, error) {
	if !ui.IsTTY() {
		// Non-interactive: list matches and return error
		ui.Error("Multiple projects match '%s':", query)
		for _, p := range matches {
			ui.Print("  - %s (%s)", p.Title, p.Version)
		}
		return nil, fmt.Errorf("multiple projects match '%s', please be more specific", query)
	}

	// Build options for selection UI
	options := make([]ui.SelectOption, len(matches))
	for i, p := range matches {
		options[i] = ui.SelectOption{
			Label:       p.Title,
			Description: p.Version,
			Value:       i,
		}
	}

	selected := ui.Select(fmt.Sprintf("Multiple projects match '%s':", query), options)
	if selected < 0 {
		return nil, fmt.Errorf("cancelled")
	}

	return &matches[selected], nil
}

func openProject(path, version, name string) error {
	err := ui.WithSpinnerNoResult("Starting Unity Editor...", func() error {
		editor := unity.NewEditor(version)
		return editor.Open(path)
	})
	if err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	ui.Success("Unity Editor %s started for project: %s", version, name)
	return nil
}
