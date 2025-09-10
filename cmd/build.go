package cmd

import (
	"fmt"
	"strings"

	"github.com/neptaco/unity-cli/pkg/unity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	buildProject    string
	buildTarget     string
	buildOutput     string
	buildMethod     string
	buildArgs       string
	buildLogFile    string
	buildCIMode     bool
	buildFailOnWarn bool
	buildTimeout    int
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Unity project",
	Long: `Build a Unity project for specified target platform.
You can specify custom build methods and pass arguments to them.`,
	RunE: runBuild,
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVar(&buildProject, "project", ".", "Path to Unity project")
	buildCmd.Flags().StringVar(&buildTarget, "target", "", "Build target platform (ios, android, windows, macos, linux)")
	buildCmd.Flags().StringVar(&buildOutput, "output", "./Build", "Output directory for build")
	buildCmd.Flags().StringVar(&buildMethod, "method", "", "Custom build method to execute")
	buildCmd.Flags().StringVar(&buildArgs, "args", "", "Arguments to pass to build method")
	buildCmd.Flags().StringVar(&buildLogFile, "log-file", "", "Path to save build log")
	buildCmd.Flags().BoolVar(&buildCIMode, "ci-mode", false, "Enable CI-friendly output format")
	buildCmd.Flags().BoolVar(&buildFailOnWarn, "fail-on-warning", false, "Fail build on warnings")
	buildCmd.Flags().IntVar(&buildTimeout, "timeout", 3600, "Build timeout in seconds")

	if err := buildCmd.MarkFlagRequired("target"); err != nil {
		logrus.Fatalf("Failed to mark target flag as required: %v", err)
	}
}

func runBuild(cmd *cobra.Command, args []string) error {
	logrus.Infof("Building Unity project: %s", buildProject)

	project, err := unity.LoadProject(buildProject)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	buildConfig := unity.BuildConfig{
		ProjectPath:    buildProject,
		Target:         buildTarget,
		OutputPath:     buildOutput,
		Method:         buildMethod,
		Args:           parseArgs(buildArgs),
		LogFile:        buildLogFile,
		CIMode:         buildCIMode,
		FailOnWarning:  buildFailOnWarn,
		TimeoutSeconds: buildTimeout,
	}

	builder := unity.NewBuilder(project)
	if err := builder.Build(buildConfig); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Printf("Build completed successfully\n")
	fmt.Printf("Output: %s\n", buildOutput)

	return nil
}

func parseArgs(argsStr string) map[string]string {
	args := make(map[string]string)
	if argsStr == "" {
		return args
	}

	pairs := strings.Split(argsStr, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			args[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return args
}
