package unity

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/neptaco/unity-cli/pkg/logger"
	"github.com/sirupsen/logrus"
)

// TestPlatform represents the test platform
type TestPlatform string

const (
	TestPlatformEditMode TestPlatform = "editmode"
	TestPlatformPlayMode TestPlatform = "playmode"
)

// TestConfig holds configuration for running Unity tests
type TestConfig struct {
	ProjectPath    string
	Platform       TestPlatform
	Filter         string
	ResultsFile    string
	LogFile        string
	TimeoutSeconds int
	CIMode         bool
	ShowTimestamp  bool
}

// TestRunner handles Unity test execution
type TestRunner struct {
	project *Project
	editor  *Editor
}

// NewTestRunner creates a new TestRunner
func NewTestRunner(project *Project) *TestRunner {
	return &TestRunner{
		project: project,
		editor:  NewEditor(project.UnityVersion),
	}
}

// RunTests executes Unity tests with the specified configuration
func (t *TestRunner) RunTests(config TestConfig) error {
	editorPath, err := t.editor.GetPath()
	if err != nil {
		return fmt.Errorf("failed to get Unity Editor path: %w", err)
	}

	absProjectPath, err := filepath.Abs(config.ProjectPath)
	if err != nil {
		absProjectPath = config.ProjectPath
	}

	args := t.buildArgs(absProjectPath, config)

	timeout := config.TimeoutSeconds
	if timeout == 0 {
		timeout = 600 // Default 10 minutes for tests
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, editorPath, args...)

	log := logger.NewWithOptions(config.LogFile,
		logger.WithCIMode(config.CIMode),
		logger.WithShowTime(config.ShowTimestamp),
	)
	defer log.Close()

	cmd.Stdout = log
	cmd.Stderr = log

	projectDir := filepath.Dir(absProjectPath)
	cmd.Dir = projectDir

	logrus.Infof("Running Unity tests: %s %s", editorPath, strings.Join(args, " "))

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Unity: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("test timeout after %d seconds", timeout)
		}
		return fmt.Errorf("tests failed: %w", err)
	}

	return nil
}

func (t *TestRunner) buildArgs(absProjectPath string, config TestConfig) []string {
	projectName := filepath.Base(absProjectPath)

	args := []string{
		"-projectPath", projectName,
		"-batchmode",
		"-nographics",
		"-runTests",
	}

	if config.Platform != "" {
		args = append(args, "-testPlatform", string(config.Platform))
	}

	if config.Filter != "" {
		args = append(args, "-testFilter", config.Filter)
	}

	if config.ResultsFile != "" {
		args = append(args, "-testResults", config.ResultsFile)
	}

	if config.LogFile != "" {
		args = append(args, "-logFile", config.LogFile)
	} else {
		args = append(args, "-logFile", "-")
	}

	return args
}
