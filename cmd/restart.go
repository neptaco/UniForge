package cmd

import (
	"errors"
	"fmt"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var (
	restartForce   bool
	restartVersion string
)

var restartCmd = &cobra.Command{
	Use:   "restart [project]",
	Short: "Restart Unity Editor",
	Long: `Restart the Unity Editor for the specified project.
This closes the running Editor and opens it again.

Examples:
  # Restart Unity Editor for current project
  uniforge restart

  # Restart with specific project path
  uniforge restart /path/to/project

  # Force restart (SIGKILL then reopen)
  uniforge restart --force

  # Override editor version
  uniforge restart /path/to/project --version 6000.0.54f1`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRestart,
}

func init() {
	rootCmd.AddCommand(restartCmd)

	restartCmd.Flags().BoolVar(&restartForce, "force", false, "Force kill the process before restart (SIGKILL)")
	restartCmd.Flags().StringVar(&restartVersion, "version", "", "Override Unity Editor version")
}

func runRestart(cmd *cobra.Command, args []string) error {
	project, err := resolveLoadedProjectArg(args)
	if err != nil {
		return err
	}

	version := project.UnityVersion
	if restartVersion != "" {
		version = restartVersion
	}

	editor := unity.NewEditor(version)

	// A restart must not open another Editor when normal quit was cancelled,
	// timed out, or otherwise failed. Only an already-closed Editor is ignored.
	err = ui.WithSpinnerNoResult("Closing Unity Editor...", func() error {
		return editor.Close(project.Path, restartForce)
	})
	if err != nil && !errors.Is(err, unity.ErrEditorNotRunning) {
		return fmt.Errorf("failed to close editor before restart: %w", err)
	}

	// Open editor
	err = ui.WithSpinnerNoResult("Starting Unity Editor...", func() error {
		return editor.Open(project.Path)
	})
	if err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	ui.Success("Unity Editor %s restarted for project: %s", version, project.Name)
	return nil
}
