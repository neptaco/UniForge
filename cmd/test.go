package cmd

import (
	"fmt"

	"github.com/neptaco/unity-cli/pkg/unity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	testProject   string
	testPlatform  string
	testFilter    string
	testResults   string
	testLogFile   string
	testTimeout   int
	testCIMode    bool
	testTimestamp bool
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run Unity Test Runner",
	Long: `Run Unity Test Runner with the specified configuration.

Supports both EditMode and PlayMode tests.

Examples:
  # Run all EditMode tests
  unity-cli test --platform editmode

  # Run all PlayMode tests
  unity-cli test --platform playmode

  # Run tests with filter
  unity-cli test --platform editmode --filter MyTestClass

  # Save test results to file
  unity-cli test --platform editmode --results ./test-results.xml

  # CI mode with custom timeout
  unity-cli test --platform editmode --ci --timeout 1800`,
	RunE: runTest,
}

func init() {
	rootCmd.AddCommand(testCmd)

	testCmd.Flags().StringVarP(&testProject, "project", "p", ".", "Path to Unity project")
	testCmd.Flags().StringVar(&testPlatform, "platform", "", "Test platform (editmode, playmode)")
	testCmd.Flags().StringVar(&testFilter, "filter", "", "Test filter expression")
	testCmd.Flags().StringVar(&testResults, "results", "", "Path to save test results (XML)")
	testCmd.Flags().StringVar(&testLogFile, "log-file", "", "Path to save log file")
	testCmd.Flags().IntVar(&testTimeout, "timeout", 600, "Test timeout in seconds")
	testCmd.Flags().BoolVar(&testCIMode, "ci", false, "CI mode (optimized output format)")
	testCmd.Flags().BoolVarP(&testTimestamp, "timestamp", "t", false, "Show timestamp for each line")

	if err := testCmd.MarkFlagRequired("platform"); err != nil {
		logrus.Warnf("Failed to mark platform flag as required: %v", err)
	}
}

func runTest(cmd *cobra.Command, args []string) error {
	logrus.Infof("Running tests for project: %s", testProject)

	project, err := unity.LoadProject(testProject)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	platform := unity.TestPlatform(testPlatform)
	if platform != unity.TestPlatformEditMode && platform != unity.TestPlatformPlayMode {
		return fmt.Errorf("invalid platform: %s (must be 'editmode' or 'playmode')", testPlatform)
	}

	testConfig := unity.TestConfig{
		ProjectPath:    testProject,
		Platform:       platform,
		Filter:         testFilter,
		ResultsFile:    testResults,
		LogFile:        testLogFile,
		TimeoutSeconds: testTimeout,
		CIMode:         testCIMode,
		ShowTimestamp:  testTimestamp,
	}

	runner := unity.NewTestRunner(project)
	if err := runner.RunTests(testConfig); err != nil {
		return fmt.Errorf("tests failed: %w", err)
	}

	fmt.Println("Tests completed successfully")
	return nil
}
