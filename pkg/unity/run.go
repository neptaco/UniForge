package unity

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/neptaco/unity-cli/pkg/logger"
	"github.com/sirupsen/logrus"
)

type RunConfig struct {
	ProjectPath    string
	ExecuteMethods []string
	BatchMode      bool
	NoGraphics     bool
	Quit           bool
	LogFile        string
	TestPlatform   string
	TestResults    string
}

type Runner struct {
	project *Project
	editor  *Editor
}

func NewRunner(project *Project) *Runner {
	return &Runner{
		project: project,
		editor:  NewEditor(project.UnityVersion),
	}
}

func (r *Runner) Run(config RunConfig) error {
	editorPath, err := r.editor.GetPath()
	if err != nil {
		return fmt.Errorf("failed to get Unity Editor path: %w", err)
	}

	// Convert to absolute path to avoid issues with relative paths
	absProjectPath, err := filepath.Abs(config.ProjectPath)
	if err != nil {
		absProjectPath = config.ProjectPath // fallback to original if abs fails
	}

	args := r.buildArgs(absProjectPath, config)
	
	cmd := exec.Command(editorPath, args...)
	
	log := logger.New(config.LogFile, false)
	defer log.Close()

	cmd.Stdout = log
	cmd.Stderr = log
	
	// Set working directory to parent of project directory
	projectDir := filepath.Dir(absProjectPath)
	cmd.Dir = projectDir

	logrus.Infof("Running Unity with command: %s %s", editorPath, strings.Join(args, " "))
	
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Unity: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("Unity execution failed: %w", err)
	}

	return nil
}

func (r *Runner) buildArgs(absProjectPath string, config RunConfig) []string {
	// Use only the project directory name for -projectPath
	projectName := filepath.Base(absProjectPath)
	
	args := []string{
		"-projectPath", projectName,
	}

	if config.BatchMode {
		args = append(args, "-batchmode")
	}

	if config.NoGraphics {
		args = append(args, "-nographics")
	}

	if config.Quit {
		args = append(args, "-quit")
	}

	if config.LogFile != "" {
		args = append(args, "-logFile", config.LogFile)
	} else {
		args = append(args, "-logFile", "-")
	}

	for _, method := range config.ExecuteMethods {
		args = append(args, "-executeMethod", method)
	}

	if config.TestPlatform != "" {
		args = append(args, "-runTests")
		args = append(args, "-testPlatform", config.TestPlatform)
		
		if config.TestResults != "" {
			args = append(args, "-testResults", config.TestResults)
		}
	}

	return args
}