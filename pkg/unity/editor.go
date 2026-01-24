package unity

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/ui"
)

type Editor struct {
	Version string
	Path    string
}

func NewEditor(version string) *Editor {
	return &Editor{
		Version: version,
	}
}

func (e *Editor) GetPath() (string, error) {
	if e.Path != "" {
		return e.Path, nil
	}

	hubClient := hub.NewClient()
	editors, err := hubClient.ListInstalledEditors()
	if err != nil {
		return "", fmt.Errorf("failed to list editors: %w", err)
	}

	for _, editor := range editors {
		if editor.Version == e.Version {
			e.Path = e.getExecutablePath(editor.Path)
			return e.Path, nil
		}
	}

	return "", fmt.Errorf("Unity Editor %s not found. Please install it using: uniforge install --version %s", e.Version, e.Version)
}

func (e *Editor) getExecutablePath(installPath string) string {
	switch runtime.GOOS {
	case "darwin":
		// Unity Hub may return path ending with .app (e.g., /path/to/Unity.app)
		if strings.HasSuffix(installPath, ".app") {
			return filepath.Join(installPath, "Contents", "MacOS", "Unity")
		}
		return filepath.Join(installPath, "Unity.app", "Contents", "MacOS", "Unity")
	case "windows":
		// Unity Hub already returns the full path to Unity.exe, so just return it as-is
		if filepath.Ext(installPath) == ".exe" {
			return installPath
		}
		return filepath.Join(installPath, "Editor", "Unity.exe")
	case "linux":
		return filepath.Join(installPath, "Editor", "Unity")
	default:
		return filepath.Join(installPath, "Unity")
	}
}

func (e *Editor) Exists() bool {
	path, err := e.GetPath()
	if err != nil {
		return false
	}

	_, err = os.Stat(path)
	return err == nil
}

// Open starts the Unity Editor with the specified project in GUI mode
func (e *Editor) Open(projectPath string) error {
	editorPath, err := e.GetPath()
	if err != nil {
		return fmt.Errorf("failed to get Unity Editor path: %w", err)
	}

	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	args := []string{"-projectPath", absProjectPath}

	ui.Debug("Opening Unity Editor", "path", editorPath, "args", strings.Join(args, " "))

	cmd := exec.Command(editorPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Unity Editor: %w", err)
	}

	ui.Debug("Unity Editor started", "pid", cmd.Process.Pid)
	return nil
}

// Close terminates the Unity Editor process for the specified project
func (e *Editor) Close(projectPath string, force bool) error {
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	pid, err := e.findUnityProcess(absProjectPath)
	if err != nil {
		return err
	}

	if pid == 0 {
		return fmt.Errorf("no Unity Editor process found for project: %s", absProjectPath)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if force {
		ui.Debug("Force killing Unity Editor process", "pid", pid)
		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
	} else {
		ui.Debug("Terminating Unity Editor process", "pid", pid)
		if err := process.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to terminate process: %w", err)
		}

		// Wait for process to exit gracefully
		done := make(chan error, 1)
		go func() {
			_, err := process.Wait()
			done <- err
		}()

		select {
		case <-done:
			ui.Debug("Unity Editor terminated gracefully")
		case <-time.After(10 * time.Second):
			ui.Warn("Grace period expired, force killing...")
			if err := process.Kill(); err != nil {
				return fmt.Errorf("failed to kill process: %w", err)
			}
		}
	}

	return nil
}

// findUnityProcess finds the Unity Editor process for the specified project
func (e *Editor) findUnityProcess(projectPath string) (int, error) {
	switch runtime.GOOS {
	case "darwin":
		return e.findUnityProcessDarwin(projectPath)
	case "windows":
		return e.findUnityProcessWindows(projectPath)
	case "linux":
		return e.findUnityProcessLinux(projectPath)
	default:
		return 0, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (e *Editor) findUnityProcessDarwin(projectPath string) (int, error) {
	// Use ps and grep to find Unity process with the project path
	cmd := exec.Command("bash", "-c", fmt.Sprintf("ps aux | grep -i Unity | grep -v grep | grep '%s'", projectPath))
	output, err := cmd.Output()
	if err != nil {
		// No process found
		return 0, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return 0, nil
	}

	// Parse PID from ps output (second column)
	fields := strings.Fields(lines[0])
	if len(fields) < 2 {
		return 0, nil
	}

	var pid int
	if _, err := fmt.Sscanf(fields[1], "%d", &pid); err != nil {
		return 0, nil
	}

	return pid, nil
}

func (e *Editor) findUnityProcessWindows(projectPath string) (int, error) {
	// Use wmic to find Unity process
	cmd := exec.Command("wmic", "process", "where", "name='Unity.exe'", "get", "ProcessId,CommandLine", "/format:csv")
	output, err := cmd.Output()
	if err != nil {
		return 0, nil
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, projectPath) {
			fields := strings.Split(strings.TrimSpace(line), ",")
			if len(fields) >= 3 {
				var pid int
				if _, err := fmt.Sscanf(fields[len(fields)-1], "%d", &pid); err == nil {
					return pid, nil
				}
			}
		}
	}

	return 0, nil
}

func (e *Editor) findUnityProcessLinux(projectPath string) (int, error) {
	// Same approach as Darwin
	return e.findUnityProcessDarwin(projectPath)
}