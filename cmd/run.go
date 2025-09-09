package cmd

import (
	"fmt"
	"strings"

	"github.com/neptaco/unity-cli/pkg/unity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	runProject       string
	runExecuteMethod string
	runBatchMode     bool
	runNoGraphics    bool
	runQuit          bool
	runLogFile       string
	runTestPlatform  string
	runTestResults   string
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Unity with custom methods",
	Long: `Run Unity Editor with custom methods or scripts.
This is useful for running tests, custom build scripts, or any automation tasks.`,
	RunE: runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVar(&runProject, "project", ".", "Path to Unity project")
	runCmd.Flags().StringVar(&runExecuteMethod, "execute-method", "", "Method(s) to execute (semicolon-separated for multiple)")
	runCmd.Flags().BoolVar(&runBatchMode, "batch-mode", true, "Run Unity in batch mode")
	runCmd.Flags().BoolVar(&runNoGraphics, "no-graphics", true, "Run Unity without graphics")
	runCmd.Flags().BoolVar(&runQuit, "quit", true, "Quit Unity after execution")
	runCmd.Flags().StringVar(&runLogFile, "log-file", "", "Path to save log file")
	runCmd.Flags().StringVar(&runTestPlatform, "test-platform", "", "Test platform (editmode, playmode)")
	runCmd.Flags().StringVar(&runTestResults, "test-results", "", "Path to save test results")
}

func runRun(cmd *cobra.Command, args []string) error {
	logrus.Infof("Running Unity project: %s", runProject)

	project, err := unity.LoadProject(runProject)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	runner := unity.NewRunner(project)

	runConfig := unity.RunConfig{
		ProjectPath:  runProject,
		BatchMode:    runBatchMode,
		NoGraphics:   runNoGraphics,
		Quit:         runQuit,
		LogFile:      runLogFile,
	}

	if runExecuteMethod != "" {
		methods := strings.Split(runExecuteMethod, ";")
		for _, method := range methods {
			runConfig.ExecuteMethods = append(runConfig.ExecuteMethods, strings.TrimSpace(method))
		}
	}

	if runTestPlatform != "" {
		runConfig.TestPlatform = runTestPlatform
		runConfig.TestResults = runTestResults
	}

	if err := runner.Run(runConfig); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	fmt.Println("Unity execution completed successfully")

	return nil
}