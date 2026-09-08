package unity

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/ui"
)

type Editor struct {
	Version       string
	Path          string
	runtimeDoctor *RuntimeDoctor
}

// EditorOpenOptions configures a GUI Unity Editor launch.
type EditorOpenOptions struct {
	NoLogFile bool
}

type unityProcessFinder func(projectPath string) (int, error)

// ErrEditorNotRunning indicates that the target project has no active Editor.
var ErrEditorNotRunning = errors.New("editor is not running")

const (
	unityProcessExitPollInterval = 200 * time.Millisecond
	editorNormalQuitTimeout      = 30 * time.Second
)

func NewEditor(version string) *Editor {
	return &Editor{
		Version:       version,
		runtimeDoctor: NewRuntimeDoctor(),
	}
}

func (e *Editor) GetPath() (string, error) {
	if e.Path != "" {
		return e.Path, nil
	}

	hubClient := hub.NewClient()

	// First, try to find editor via install path (faster, doesn't require Hub CLI)
	installPath, err := hubClient.GetInstallPath()
	if err == nil && installPath != "" {
		editorDir := filepath.Join(installPath, e.Version)
		execPath := e.getExecutablePath(editorDir)
		if fileExists(execPath) {
			ui.Debug("Found Unity Editor via install path", "version", e.Version, "path", execPath)
			e.Path = execPath
			return e.Path, nil
		}
	}

	// Fallback: try Hub CLI to list installed editors
	editors, err := hubClient.ListInstalledEditors()
	if err != nil {
		return "", fmt.Errorf("unity editor %s not found. install path: %s, hub error: %w", e.Version, installPath, err)
	}

	for _, editor := range editors {
		if editor.Version == e.Version {
			e.Path = e.getExecutablePath(editor.Path)
			return e.Path, nil
		}
	}

	return "", fmt.Errorf("unity editor %s not found, please install it using: uniforge editor install %s", e.Version, e.Version)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (e *Editor) getExecutablePath(installPath string) string {
	return resolveEditorExecutablePath(runtime.GOOS, installPath)
}

func resolveEditorExecutablePath(goos, installPath string) string {
	installPath = cleanPathForOS(goos, installPath)

	switch goos {
	case "darwin":
		executableSuffix := joinPathForOS(goos, "Unity.app", "Contents", "MacOS", "Unity")
		if strings.HasSuffix(installPath, executableSuffix) {
			return installPath
		}
		// Unity Hub may return path ending with .app (e.g., /path/to/Unity.app)
		if strings.HasSuffix(installPath, ".app") {
			return joinPathForOS(goos, installPath, "Contents", "MacOS", "Unity")
		}
		return joinPathForOS(goos, installPath, "Unity.app", "Contents", "MacOS", "Unity")
	case "windows":
		// Unity Hub already returns the full path to Unity.exe, so just return it as-is
		if strings.EqualFold(filepath.Ext(installPath), ".exe") {
			return installPath
		}
		return joinPathForOS(goos, installPath, "Editor", "Unity.exe")
	case "linux":
		if strings.HasSuffix(installPath, joinPathForOS(goos, "Editor", "Unity")) {
			return installPath
		}
		return joinPathForOS(goos, installPath, "Editor", "Unity")
	default:
		return joinPathForOS(goos, installPath, "Unity")
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
	return e.OpenWithOptions(projectPath, EditorOpenOptions{})
}

// OpenWithOptions starts the Unity Editor with the specified project in GUI mode.
func (e *Editor) OpenWithOptions(projectPath string, options EditorOpenOptions) error {
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	if err := e.prepareRuntimeForOpen(absProjectPath); err != nil {
		return err
	}

	// Check if Unity Editor is already running for this project
	if err := e.CheckNotRunning(absProjectPath); err != nil {
		return err
	}

	editorPath, err := e.GetPath()
	if err != nil {
		return fmt.Errorf("failed to get Unity Editor path: %w", err)
	}

	logFilePath := ""
	if !options.NoLogFile {
		logFilePath, err = GetManagedEditorLogPath(absProjectPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(logFilePath), 0o755); err != nil {
			return fmt.Errorf("failed to create editor log directory: %w", err)
		}
	}

	args := buildEditorLaunchArgs(absProjectPath, logFilePath)

	ui.Debug("Opening Unity Editor", "path", editorPath, "args", strings.Join(args, " "))

	cmd := exec.Command(editorPath, args...)
	configureOpenCommand(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Unity Editor: %w", err)
	}

	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		ui.Debug("Failed to release Unity Editor process handle", "pid", pid, "error", err)
	}

	ui.Debug("Unity Editor started", "pid", pid)
	return nil
}

func (e *Editor) prepareRuntimeForOpen(projectPath string) error {
	result, err := e.checkAndFixRuntime(projectPath)
	if err != nil {
		return fmt.Errorf("failed to prepare Unity runtime: %w", err)
	}
	for _, fix := range result.Fixes {
		ui.Debug("Repaired Unity runtime before launch", "kind", fix.Kind, "path", fix.Path, "pid", fix.PID)
	}
	if result.HasUnfixedBlockingIssues() {
		return fmt.Errorf("unity runtime has blocking issue(s); run `uniforge doctor %q --fix` for details", projectPath)
	}
	return nil
}

func (e *Editor) repairRuntimeAfterClose(projectPath string) error {
	result, err := e.checkAndFixRuntime(projectPath)
	if err != nil {
		return fmt.Errorf("failed to repair Unity runtime after close: %w", err)
	}
	for _, fix := range result.Fixes {
		ui.Debug("Repaired Unity runtime after close", "kind", fix.Kind, "path", fix.Path, "pid", fix.PID)
	}
	if result.HasUnfixedBlockingIssues() {
		return fmt.Errorf("unity runtime has blocking issue(s) after close; run `uniforge doctor %q --fix` for details", projectPath)
	}
	return nil
}

func (e *Editor) checkAndFixRuntime(projectPath string) (*RuntimeDoctorResult, error) {
	doctor := e.runtimeDoctor
	if doctor == nil {
		doctor = NewRuntimeDoctor()
	}
	return doctor.Check(projectPath, true)
}

func buildEditorLaunchArgs(projectPath, logFilePath string) []string {
	args := []string{"-projectPath", projectPath}
	if logFilePath != "" {
		args = append(args, "-logFile", logFilePath)
	}
	return args
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
		return fmt.Errorf("%w for project: %s", ErrEditorNotRunning, absProjectPath)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	// Snapshot the Editor's children (licensing client, asset import workers,
	// ILPP) before signalling: once the Editor dies they are reparented and can
	// no longer be attributed to it. A killed Editor cannot tear its children
	// down, so leftovers are reaped afterwards — even when the exit wait fails,
	// which is exactly when orphans are guaranteed. A graceful exit gets a
	// conservative runtime-doctor pass afterwards: it only cleans an Editor
	// licensing client when no other Editor is running.
	reaper := newProcessTreeReaper()
	descendants := reaper.snapshotDescendants(pid)
	reapAfterKill := func(waitErr error) error {
		reaper.reap(descendants, treeReapNaturalGrace, treeReapTermGrace)
		if waitErr != nil {
			return waitErr
		}
		return e.repairRuntimeAfterClose(absProjectPath)
	}

	if force {
		ui.Debug("Force killing Unity Editor process", "pid", pid)
		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		return reapAfterKill(waitForUnityProcessExit(absProjectPath, 10*time.Second, unityProcessExitPollInterval, e.findUnityProcess))
	}

	ui.Debug("Requesting normal Unity Editor quit", "pid", pid)
	if err := requestEditorQuit(process); err != nil {
		return fmt.Errorf("failed to request normal Unity Editor quit: %w", err)
	}

	if err := waitForUnityProcessExit(absProjectPath, editorNormalQuitTimeout, unityProcessExitPollInterval, e.findUnityProcess); err != nil {
		return fmt.Errorf(
			"editor did not complete normal quit; it may be waiting for an unsaved-changes confirmation: %w (use --force only to discard that shutdown flow)",
			err,
		)
	}

	ui.Debug("Unity Editor completed normal quit")
	reaper.waitForNaturalExit(descendants, treeReapNaturalGrace)
	return e.repairRuntimeAfterClose(absProjectPath)
}

type unityLockfileProbe func(path string) (held bool, err error)

// CheckNotRunning returns an error if Unity Editor holds the project's lockfile.
// The file itself can outlive an interrupted or failed Editor process, so its OS-level
// lock state is authoritative rather than its mere existence or a process-list heuristic.
func (e *Editor) CheckNotRunning(projectPath string) error {
	return checkNotRunning(projectPath, probeUnityLockfile)
}

func checkNotRunning(projectPath string, probeLockfile unityLockfileProbe) error {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	lockfile := filepath.Join(absPath, "Temp", "UnityLockfile")
	if _, err := os.Stat(lockfile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to inspect Unity lockfile %s: %w", lockfile, err)
	}

	held, err := probeLockfile(lockfile)
	if err != nil {
		return fmt.Errorf("failed to inspect Unity lock state for %s: %w", lockfile, err)
	}
	if held {
		return fmt.Errorf("unity Editor is already running for this project (lockfile is held: %s)", lockfile)
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
	cmd := exec.Command("ps", "-ax", "-o", "pid=,command=")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to list processes: %w", err)
	}

	return findUnityProcessFromPSOutput(string(output), projectPath), nil
}

func (e *Editor) findUnityProcessLinux(projectPath string) (int, error) {
	return e.findUnityProcessDarwin(projectPath)
}

func findUnityProcessFromPSOutput(output, projectPath string) int {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		commandLine := strings.Join(fields[1:], " ")
		if !isUnityEditorCommandLine(commandLine) {
			continue
		}
		if !strings.Contains(commandLine, projectPath) {
			continue
		}

		var pid int
		if _, err := fmt.Sscanf(fields[0], "%d", &pid); err == nil {
			return pid
		}
	}

	return 0
}

func isUnityEditorCommandLine(commandLine string) bool {
	commandLine = strings.ToLower(strings.TrimSpace(commandLine))
	if commandLine == "" {
		return false
	}

	// Match the actual Unity editor executable, not arbitrary commands that happen
	// to contain "unity" in their working directory or arguments.
	return strings.Contains(commandLine, "unity.app/contents/macos/unity") ||
		strings.Contains(commandLine, "/editor/unity") ||
		strings.Contains(commandLine, `\editor\unity`) ||
		strings.Contains(commandLine, "unity.exe")
}

func waitForUnityProcessExit(
	projectPath string,
	timeout time.Duration,
	pollInterval time.Duration,
	findProcess unityProcessFinder,
) error {
	if pollInterval <= 0 {
		pollInterval = unityProcessExitPollInterval
	}

	deadline := time.Now().Add(timeout)
	for {
		pid, err := findProcess(projectPath)
		if err != nil {
			return fmt.Errorf("failed to verify Unity process exit: %w", err)
		}
		if pid == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("unity Editor is still running for project: %s", projectPath)
		}

		time.Sleep(pollInterval)
	}
}

func cleanPathForOS(goos, value string) string {
	if goos == "windows" {
		return filepath.Clean(value)
	}

	return pathpkg.Clean(strings.ReplaceAll(value, "\\", "/"))
}

func joinPathForOS(goos string, elems ...string) string {
	if goos == "windows" {
		return filepath.Join(elems...)
	}

	normalized := make([]string, 0, len(elems))
	for _, elem := range elems {
		normalized = append(normalized, strings.ReplaceAll(elem, "\\", "/"))
	}

	return pathpkg.Join(normalized...)
}
