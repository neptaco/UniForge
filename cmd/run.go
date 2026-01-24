package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var (
	runProject   string
	runLogFile   string
	runTimeout   int
	runCIMode    bool
	runTimestamp bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Unity in batch mode with custom arguments",
	Long: `Run Unity Editor in batch mode with custom arguments.
All arguments after -- are passed directly to Unity.

This is a generic command for executing any Unity batch operation:
builds, custom methods, asset processing, etc.

Examples:
  # Run a custom method
  uniforge run -- -executeMethod MyScript.DoSomething

  # Build for Windows
  uniforge run -- -buildTarget Win64 -buildWindows64Player ./Build/Game.exe

  # Run multiple methods
  uniforge run -- -executeMethod BuildScript.PreBuild -executeMethod BuildScript.Build

  # Custom asset processing
  uniforge run -- -executeMethod AssetProcessor.ProcessAll

  # With project path and timeout
  uniforge run -p /path/to/project --timeout 3600 -- -executeMethod LongProcess.Run`,
	RunE: runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVarP(&runProject, "project", "p", ".", "Path to Unity project")
	runCmd.Flags().StringVar(&runLogFile, "log-file", "", "Path to save log file")
	runCmd.Flags().IntVar(&runTimeout, "timeout", 3600, "Timeout in seconds")
	runCmd.Flags().BoolVar(&runCIMode, "ci", false, "CI mode (optimized output format)")
	runCmd.Flags().BoolVarP(&runTimestamp, "timestamp", "t", false, "Show timestamp for each line")
}

func runRun(cmd *cobra.Command, args []string) error {
	ui.Info("Running Unity for project: %s", runProject)

	project, err := unity.LoadProject(runProject)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	runConfig := unity.RunConfig{
		ProjectPath:    runProject,
		ExtraArgs:      args,
		LogFile:        runLogFile,
		TimeoutSeconds: runTimeout,
		CIMode:         runCIMode,
		ShowTimestamp:  runTimestamp,
	}

	runner := unity.NewRunner(project)
	if err := runner.Run(runConfig); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	ui.Success("Unity execution completed successfully")
	return nil
}
