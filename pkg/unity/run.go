package unity

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/neptaco/uniforge/pkg/logger"
	"github.com/neptaco/uniforge/pkg/ui"
)

// RunConfig holds configuration for running Unity in batch mode
type RunConfig struct {
	ProjectPath    string
	ExtraArgs      []string // Arguments passed after --
	LogFile        string
	TimeoutSeconds int
	CIMode         bool
	ShowTimestamp  bool
}

// Runner handles Unity batch execution
type Runner struct {
	project *Project
	editor  *Editor
}

// NewRunner creates a new Runner
func NewRunner(project *Project) *Runner {
	return &Runner{
		project: project,
		editor:  NewEditor(project.UnityVersion),
	}
}

// Run executes Unity in batch mode with the specified configuration
func (r *Runner) Run(config RunConfig) error {
	editorPath, err := r.editor.GetPath()
	if err != nil {
		return fmt.Errorf("failed to get Unity Editor path: %w", err)
	}

	absProjectPath, err := filepath.Abs(config.ProjectPath)
	if err != nil {
		absProjectPath = config.ProjectPath
	}

	args := r.buildArgs(absProjectPath, config)

	timeout := config.TimeoutSeconds
	if timeout == 0 {
		timeout = 3600 // Default 1 hour
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, editorPath, args...)

	log := logger.NewWithOptions(config.LogFile,
		logger.WithCIMode(config.CIMode),
		logger.WithShowTime(config.ShowTimestamp),
	)
	defer func() { _ = log.Close() }()

	cmd.Stdout = log
	cmd.Stderr = log

	projectDir := filepath.Dir(absProjectPath)
	cmd.Dir = projectDir

	ui.Debug("Running Unity", "path", editorPath, "args", strings.Join(args, " "))

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Unity: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("execution timeout after %d seconds", timeout)
		}
		return fmt.Errorf("unity execution failed: %w", err)
	}

	return nil
}

func (r *Runner) buildArgs(absProjectPath string, config RunConfig) []string {
	projectName := filepath.Base(absProjectPath)

	args := []string{
		"-projectPath", projectName,
		"-batchmode",
		"-nographics",
		"-quit",
	}

	if config.LogFile != "" {
		args = append(args, "-logFile", config.LogFile)
	} else {
		args = append(args, "-logFile", "-")
	}

	// Append extra arguments (passed after --)
	if len(config.ExtraArgs) > 0 {
		args = append(args, config.ExtraArgs...)
	}

	return args
}
