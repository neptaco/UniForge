package unity

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neptaco/uniforge/pkg/platform"
)

func TestCheckNotRunningSkipsLockProbeWithoutLockfile(t *testing.T) {
	projectPath := t.TempDir()

	err := checkNotRunning(projectPath, func(string) (bool, error) {
		t.Fatal("lock probe should not run without a lockfile")
		return false, nil
	})
	if err != nil {
		t.Fatalf("checkNotRunning failed: %v", err)
	}
}

func TestCheckNotRunningAllowsStaleLockfile(t *testing.T) {
	projectPath, lockfile := createEditorLockfile(t)

	err := checkNotRunning(projectPath, func(gotLockfile string) (bool, error) {
		if gotLockfile != lockfile {
			t.Fatalf("lockfile path = %q, want %q", gotLockfile, lockfile)
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("checkNotRunning failed: %v", err)
	}
	if _, err := os.Stat(lockfile); err != nil {
		t.Fatalf("stale lockfile should be left for Unity to handle: %v", err)
	}
}

func TestCheckNotRunningRejectsActiveEditor(t *testing.T) {
	projectPath, _ := createEditorLockfile(t)

	err := checkNotRunning(projectPath, func(string) (bool, error) {
		return true, nil
	})
	if err == nil {
		t.Fatal("expected active Unity Editor error")
	}
}

func TestCheckNotRunningRejectsUnverifiedLockfile(t *testing.T) {
	projectPath, _ := createEditorLockfile(t)

	err := checkNotRunning(projectPath, func(string) (bool, error) {
		return false, errors.New("lock probe failed")
	})
	if err == nil {
		t.Fatal("expected lock inspection error")
	}
}

func TestPrepareRuntimeForOpenRepairsOrphanEditorLicensingClient(t *testing.T) {
	projectPath := t.TempDir()
	processes := []processInfo{
		{PID: 77, Name: "Unity Hub", Command: `/Applications/Unity Hub.app/Contents/MacOS/Unity Hub`},
		{
			PID:     99,
			Name:    "Unity.Licensing.Client",
			Command: `/Applications/Unity/Hub/Editor/6000.4.11f1/Unity.app/Contents/Helpers/UnityLicensingClient.app/Contents/MacOS/Unity.Licensing.Client --namedPipe Unity-LicenseClient-developer-6000.4.11`,
		},
	}
	var stopped int
	editor := NewEditor("6000.4.11f1")
	editor.runtimeDoctor = testRuntimeDoctor(processes, func(pid int) error {
		stopped = pid
		return nil
	})

	if err := editor.prepareRuntimeForOpen(projectPath); err != nil {
		t.Fatalf("prepareRuntimeForOpen failed: %v", err)
	}
	if stopped != 99 {
		t.Fatalf("stopped pid = %d, want 99", stopped)
	}
}

func TestPrepareRuntimeForOpenRejectsUnfixedBlockingIssue(t *testing.T) {
	projectPath := t.TempDir()
	pidFile := filepath.Join(projectPath, "Library", "ilpp.pid")
	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte("not-a-pid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	editor := NewEditor("6000.4.11f1")
	editor.runtimeDoctor = testRuntimeDoctor(nil, nil)

	err := editor.prepareRuntimeForOpen(projectPath)
	if err == nil || !strings.Contains(err.Error(), "unity runtime has blocking issue") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepairRuntimeAfterCloseStopsOrphanEditorLicensingClient(t *testing.T) {
	projectPath := t.TempDir()
	processes := []processInfo{
		{PID: 77, Name: "Unity Hub", Command: `/Applications/Unity Hub.app/Contents/MacOS/Unity Hub`},
		{
			PID:     99,
			Name:    "Unity.Licensing.Client",
			Command: `/Applications/Unity/Hub/Editor/6000.4.11f1/Unity.app/Contents/Helpers/UnityLicensingClient.app/Contents/MacOS/Unity.Licensing.Client --namedPipe Unity-LicenseClient-developer-6000.4.11`,
		},
	}
	var stopped int
	editor := NewEditor("6000.4.11f1")
	editor.runtimeDoctor = testRuntimeDoctor(processes, func(pid int) error {
		stopped = pid
		return nil
	})

	if err := editor.repairRuntimeAfterClose(projectPath); err != nil {
		t.Fatalf("repairRuntimeAfterClose failed: %v", err)
	}
	if stopped != 99 {
		t.Fatalf("stopped pid = %d, want 99", stopped)
	}
}

func TestRepairRuntimeAfterClosePreservesLicensingClientWhileAnotherEditorRuns(t *testing.T) {
	projectPath := t.TempDir()
	processes := []processInfo{
		{
			PID:     88,
			Name:    "Unity",
			Command: `/Applications/Unity/Hub/Editor/6000.4.11f1/Unity.app/Contents/MacOS/Unity -projectPath /Projects/Other`,
		},
		{
			PID:     99,
			Name:    "Unity.Licensing.Client",
			Command: `/Applications/Unity/Hub/Editor/6000.4.11f1/Unity.app/Contents/Helpers/UnityLicensingClient.app/Contents/MacOS/Unity.Licensing.Client --namedPipe Unity-LicenseClient-developer-6000.4.11`,
		},
	}
	var stopped int
	editor := NewEditor("6000.4.11f1")
	editor.runtimeDoctor = testRuntimeDoctor(processes, func(pid int) error {
		stopped = pid
		return nil
	})

	if err := editor.repairRuntimeAfterClose(projectPath); err != nil {
		t.Fatalf("repairRuntimeAfterClose failed: %v", err)
	}
	if stopped != 0 {
		t.Fatalf("stopped pid = %d, want no stopped process", stopped)
	}
}

func createEditorLockfile(t *testing.T) (string, string) {
	t.Helper()
	projectPath := t.TempDir()
	tempDir := filepath.Join(projectPath, "Temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatalf("create Temp directory: %v", err)
	}
	lockfile := filepath.Join(tempDir, "UnityLockfile")
	if err := os.WriteFile(lockfile, []byte("lock"), 0o644); err != nil {
		t.Fatalf("create lockfile: %v", err)
	}
	return projectPath, lockfile
}

func TestFindUnityProcessFromPSOutputMatchesProjectPathSafely(t *testing.T) {
	projectPath := "/Projects/O'Brien/Test Project"
	output := `
123 /Applications/Unity/Hub/Editor/2022.3.60f1/Unity.app/Contents/MacOS/Unity -projectPath /Projects/Other
456 /Applications/Unity/Hub/Editor/2022.3.60f1/Unity.app/Contents/MacOS/Unity -projectPath /Projects/O'Brien/Test Project
`

	pid := findUnityProcessFromPSOutput(output, projectPath)
	if pid != 456 {
		t.Fatalf("pid = %d, want 456", pid)
	}
}

func TestFindUnityProcessFromPSOutputIgnoresNonUnityProcesses(t *testing.T) {
	projectPath := "/Projects/MyGame"
	output := `
111 /usr/bin/python worker.py /Projects/MyGame
222 /Applications/Unity/Hub/Unity Hub.app/Contents/MacOS/Unity Hub
333 /Applications/Unity/Hub/Editor/2022.3.60f1/Unity.app/Contents/MacOS/Unity -projectPath /Projects/OtherGame
`

	pid := findUnityProcessFromPSOutput(output, projectPath)
	if pid != 0 {
		t.Fatalf("pid = %d, want 0", pid)
	}
}

func TestFindUnityProcessFromPSOutputIgnoresUniforgeOpenCommand(t *testing.T) {
	projectPath := "/Users/developer/work/uniforge/uniforge-client"
	output := `
11772 /Users/developer/work/uniforge/dist/uniforge open /Users/developer/work/uniforge/uniforge-client
`

	pid := findUnityProcessFromPSOutput(output, projectPath)
	if pid != 0 {
		t.Fatalf("pid = %d, want 0", pid)
	}
}

func TestWaitForUnityProcessExitReturnsWhenProcessDisappears(t *testing.T) {
	callCount := 0
	findProcess := func(projectPath string) (int, error) {
		callCount++
		if callCount < 3 {
			return 1234, nil
		}
		return 0, nil
	}

	err := waitForUnityProcessExit("/Projects/MyGame", 50*time.Millisecond, time.Millisecond, findProcess)
	if err != nil {
		t.Fatalf("waitForUnityProcessExit failed: %v", err)
	}
}

func TestWaitForUnityProcessExitTimesOut(t *testing.T) {
	findProcess := func(projectPath string) (int, error) {
		return 1234, nil
	}

	err := waitForUnityProcessExit("/Projects/MyGame", 5*time.Millisecond, time.Millisecond, findProcess)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestWaitForUnityProcessExitPropagatesFinderErrors(t *testing.T) {
	findProcess := func(projectPath string) (int, error) {
		return 0, errors.New("ps failed")
	}

	err := waitForUnityProcessExit("/Projects/MyGame", time.Second, time.Millisecond, findProcess)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveEditorExecutablePathHandlesLinuxExecutablePath(t *testing.T) {
	path := "/opt/Unity/Hub/Editor/2022.3.60f1/Editor/Unity"

	resolved := resolveEditorExecutablePath("linux", path)
	if resolved != path {
		t.Fatalf("resolved = %q, want %q", resolved, path)
	}
}

func TestResolveEditorExecutablePathBuildsLinuxExecutableFromVersionDir(t *testing.T) {
	baseDir := "/opt/Unity/Hub/Editor/2022.3.60f1"

	resolved := resolveEditorExecutablePath("linux", baseDir)
	expected := "/opt/Unity/Hub/Editor/2022.3.60f1/Editor/Unity"
	if resolved != expected {
		t.Fatalf("resolved = %q, want %q", resolved, expected)
	}
}

func TestBuildEditorLaunchArgsIncludesLogFile(t *testing.T) {
	got := buildEditorLaunchArgs("/projects/My Game", "/state/editor-logs/My_Game.log")
	want := []string{
		"-projectPath", "/projects/My Game",
		"-logFile", "/state/editor-logs/My_Game.log",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestBuildEditorLaunchArgsOmitsLogFile(t *testing.T) {
	got := buildEditorLaunchArgs("/projects/My Game", "")
	want := []string{"-projectPath", "/projects/My Game"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestBuildEditorLogPathSanitizesProjectName(t *testing.T) {
	stateDir := t.TempDir()
	projectPath := filepath.Join(t.TempDir(), "My Game: Demo")
	got := buildEditorLogPath(stateDir, projectPath)
	want := filepath.Join(stateDir, "editor-logs", "My_Game_Demo-"+expectedProjectPathHash(projectPath)+".log")
	if got != want {
		t.Fatalf("log path = %q, want %q", got, want)
	}
}

func TestBuildEditorLogPathDistinguishesProjectsWithSameName(t *testing.T) {
	stateDir := t.TempDir()
	first := buildEditorLogPath(stateDir, filepath.Join(t.TempDir(), "Game"))
	second := buildEditorLogPath(stateDir, filepath.Join(t.TempDir(), "Game"))

	if first == second {
		t.Fatalf("same-name projects resolved to the same log path: %s", first)
	}
}

func expectedProjectPathHash(projectPath string) string {
	hashInput := filepath.Clean(projectPath)
	if runtime.GOOS == "windows" {
		hashInput = strings.ToLower(hashInput)
	}
	sum := sha256.Sum256([]byte(hashInput))
	return fmt.Sprintf("%x", sum[:4])
}

func TestGetManagedEditorLogPathUsesAbsoluteProjectPath(t *testing.T) {
	stateDir, err := platform.StateDir()
	if err != nil {
		t.Fatalf("get state directory: %v", err)
	}
	projectPath, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("get absolute project path: %v", err)
	}

	got, err := GetManagedEditorLogPath(".")
	if err != nil {
		t.Fatalf("GetManagedEditorLogPath failed: %v", err)
	}
	want := buildEditorLogPath(stateDir, projectPath)
	if got != want {
		t.Fatalf("managed log path = %q, want %q", got, want)
	}
}
