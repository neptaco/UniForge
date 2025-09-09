package unity

import (
	"fmt"
	"os/exec"
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

	args := r.buildArgs(config)
	
	cmd := exec.Command(editorPath, args...)
	
	log := logger.New(config.LogFile, false)
	defer log.Close()

	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Dir = config.ProjectPath

	logrus.Infof("Running Unity with command: %s %s", editorPath, strings.Join(args, " "))
	
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Unity: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("Unity execution failed: %w", err)
	}

	return nil
}

func (r *Runner) buildArgs(config RunConfig) []string {
	args := []string{
		"-projectPath", config.ProjectPath,
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