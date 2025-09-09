package unity

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/neptaco/unity-cli/pkg/logger"
	"github.com/neptaco/unity-cli/pkg/platform"
	"github.com/sirupsen/logrus"
)

type BuildConfig struct {
	ProjectPath    string
	Target         string
	OutputPath     string
	Method         string
	Args           map[string]string
	LogFile        string
	CIMode         bool
	FailOnWarning  bool
	TimeoutSeconds int
}

type Builder struct {
	project *Project
	editor  *Editor
}

func NewBuilder(project *Project) *Builder {
	return &Builder{
		project: project,
		editor:  NewEditor(project.UnityVersion),
	}
}

func (b *Builder) Build(config BuildConfig) error {
	editorPath, err := b.editor.GetPath()
	if err != nil {
		return fmt.Errorf("failed to get Unity Editor path: %w", err)
	}

	if err := os.MkdirAll(config.OutputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	args := b.buildArgs(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, editorPath, args...)
	
	log := logger.New(config.LogFile, config.CIMode)
	defer log.Close()

	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Dir = config.ProjectPath

	logrus.Infof("Starting Unity build with command: %s %s", editorPath, strings.Join(args, " "))
	
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Unity: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("build timeout after %d seconds", config.TimeoutSeconds)
		}
		return fmt.Errorf("build failed: %w", err)
	}

	if config.FailOnWarning && log.HasWarnings() {
		return fmt.Errorf("build completed with warnings")
	}

	return nil
}

func (b *Builder) buildArgs(config BuildConfig) []string {
	args := []string{
		"-projectPath", config.ProjectPath,
		"-batchmode",
		"-nographics",
		"-quit",
	}

	if config.LogFile != "" {
		args = append(args, "-logFile", config.LogFile)
	} else {
		args = append(args, "-logFile", "-")
	}

	target := b.mapBuildTarget(config.Target)
	if target != "" {
		args = append(args, "-buildTarget", target)
	}

	if config.Method != "" {
		args = append(args, "-executeMethod", config.Method)
		
		if len(config.Args) > 0 {
			for k, v := range config.Args {
				args = append(args, fmt.Sprintf("-%s", k), v)
			}
		}
	} else {
		outputPath := filepath.Join(config.OutputPath, b.getDefaultOutputName(config.Target))
		
		switch config.Target {
		case "windows":
			args = append(args, "-buildWindows64Player", outputPath)
		case "macos":
			args = append(args, "-buildOSXUniversalPlayer", outputPath)
		case "linux":
			args = append(args, "-buildLinux64Player", outputPath)
		case "android":
			args = append(args, "-executeMethod", "UnityEditor.BuildPlayerWindow.DefaultBuildMethods.BuildPlayer")
		case "ios":
			args = append(args, "-executeMethod", "UnityEditor.BuildPlayerWindow.DefaultBuildMethods.BuildPlayer")
		}
	}

	return args
}

func (b *Builder) mapBuildTarget(target string) string {
	targets := map[string]string{
		"windows": "Win64",
		"macos":   "OSXUniversal",
		"linux":   "Linux64",
		"android": "Android",
		"ios":     "iOS",
		"webgl":   "WebGL",
	}
	
	return targets[strings.ToLower(target)]
}

func (b *Builder) getDefaultOutputName(target string) string {
	ext := platform.GetExecutableExtension(target)
	baseName := b.project.Name
	
	if ext != "" {
		return baseName + ext
	}
	
	return baseName
}