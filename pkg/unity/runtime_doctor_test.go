package unity

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeDoctorFixesStaleFiles(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	lockfile := filepath.Join(project, "Temp", "UnityLockfile")
	pidFile := filepath.Join(project, "Library", "ilpp.pid")
	writeRuntimeFile(t, lockfile, "")
	writeRuntimeFile(t, pidFile, "4242\n")
	doctor := testRuntimeDoctor(nil, nil)
	result, err := doctor.Check(project, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.HasUnfixedBlockingIssues() || len(result.Fixes) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertRuntimeFileMissing(t, lockfile)
	assertRuntimeFileMissing(t, pidFile)
}

func TestRuntimeDoctorNeverFixesActiveEditorLockfile(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	lockfile := filepath.Join(project, "Temp", "UnityLockfile")
	writeRuntimeFile(t, lockfile, "")
	doctor := testRuntimeDoctor([]processInfo{{PID: 123, Command: runtimeDoctorEditorCommand(project)}}, nil)
	result, err := doctor.Check(project, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasUnfixedBlockingIssues() || len(result.Fixes) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertRuntimeFileExists(t, lockfile)
}

func TestRuntimeDoctorMatchesQuotedProjectPathExactly(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	lockfile := filepath.Join(project, "Temp", "UnityLockfile")
	writeRuntimeFile(t, lockfile, "")
	doctor := testRuntimeDoctor([]processInfo{{PID: 123, Command: `/Applications/Unity/Unity.app/Contents/MacOS/Unity -projectPath "` + project + `-other"`}}, nil)
	result, err := doctor.Check(project, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.EditorPID != 0 || !result.HasFixableIssues() {
		t.Fatalf("prefix path must not match: %+v", result)
	}
}

func TestRuntimeDoctorMatchesSymlinkedProjectPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink path normalization test")
	}
	project := makeRuntimeDoctorProject(t)
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(project, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	lockfile := filepath.Join(project, "Temp", "UnityLockfile")
	writeRuntimeFile(t, lockfile, "")
	doctor := testRuntimeDoctor([]processInfo{{PID: 123, Command: `/Applications/Unity/Unity.app/Contents/MacOS/Unity -projectPath "` + link + `"`}}, nil)
	result, err := doctor.Check(project, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.EditorPID != 123 || len(result.Fixes) != 0 {
		t.Fatalf("symlinked project must match active editor: %+v", result)
	}
	assertRuntimeFileExists(t, lockfile)
}

func runtimeDoctorEditorCommand(project string) string {
	if runtime.GOOS == "windows" {
		return `C:\Program Files\Unity\Hub\Editor\6000.0.0f1\Editor\unity.exe -projectPath "` + project + `"`
	}
	return `/Applications/Unity/Unity.app/Contents/MacOS/Unity -projectPath "` + project + `"`
}

func TestCommandTargetsWindowsProject(t *testing.T) {
	project := `C:\Users\runneradmin\AppData\Local\Temp\Project\001`
	command := `C:\Program Files\Unity\Hub\Editor\6000.0.0f1\Editor\unity.exe -projectPath "` + project + `"`
	if !isUnityEditorCommandLine(command) || !commandTargetsProjectForOS(command, project, "windows") {
		t.Fatalf("Windows Unity command must target project: %q", command)
	}
}

func TestRuntimeDoctorKeepsActiveILPP(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	pidFile := filepath.Join(project, "Library", "ilpp.pid")
	writeRuntimeFile(t, pidFile, "88\n")
	doctor := testRuntimeDoctor([]processInfo{{PID: 88, Name: "Unity.ILPP.Runner", Command: "Unity.ILPP.Runner"}}, nil)
	result, err := doctor.Check(project, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.HasUnfixedBlockingIssues() || len(result.Fixes) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertRuntimeFileExists(t, pidFile)
}

func TestRuntimeDoctorStopsOrphanLicensingClient(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	var stopped int
	doctor := testRuntimeDoctor([]processInfo{{PID: 99, Name: "Unity.Licensing.Client", Command: "Unity.Licensing.Client"}}, func(pid int) error { stopped = pid; return nil })
	result, err := doctor.Check(project, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.HasUnfixedBlockingIssues() || stopped != 99 {
		t.Fatalf("unexpected result: %+v, stopped=%d", result, stopped)
	}
}

func TestRuntimeDoctorIgnoresLicensingTextInUnrelatedCommand(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	doctor := testRuntimeDoctor([]processInfo{{PID: 99, Name: "tail", Command: "tail -f /tmp/Unity.Licensing.Client.log"}}, func(pid int) error {
		t.Fatalf("unexpected stop %d", pid)
		return nil
	})
	result, err := doctor.Check(project, true)
	if err != nil || result.HasIssues() {
		t.Fatalf("unexpected result: %+v, err=%v", result, err)
	}
}

func TestRuntimeDoctorDoesNotStopLicensingClientWhileHubRuns(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	processes := []processInfo{
		{PID: 77, Name: "Unity Hub", Command: `/Applications/Unity Hub.app/Contents/MacOS/Unity Hub`},
		{
			PID:     99,
			Name:    "Unity.Licensing.Client",
			Command: `/Applications/Unity Hub.app/Contents/Frameworks/UnityLicensingClient_V1.app/Contents/MacOS/Unity.Licensing.Client --namedPipe Unity-LicenseClient-developer`,
		},
	}
	doctor := testRuntimeDoctor(processes, func(pid int) error { t.Fatalf("unexpected stop %d", pid); return nil })
	result, err := doctor.Check(project, true)
	if err != nil || result.HasIssues() {
		t.Fatalf("unexpected result: %+v, err=%v", result, err)
	}
}

func TestRuntimeDoctorStopsEditorLicensingClientWhileHubRuns(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	processes := []processInfo{
		{PID: 77, Name: "Unity Hub", Command: `/Applications/Unity Hub.app/Contents/MacOS/Unity Hub`},
		{
			PID:     98,
			Name:    "Unity.Licensing.Client",
			Command: `/Applications/Unity Hub.app/Contents/Frameworks/UnityLicensingClient_V1.app/Contents/MacOS/Unity.Licensing.Client --namedPipe Unity-LicenseClient-developer`,
		},
		{
			PID:     99,
			Name:    "Unity.Licensing.Client",
			Command: `/Applications/Unity/Hub/Editor/6000.4.11f1/Unity.app/Contents/Helpers/UnityLicensingClient.app/Contents/MacOS/Unity.Licensing.Client --namedPipe Unity-LicenseClient-developer-6000.4.11`,
		},
	}
	var stopped []int
	doctor := testRuntimeDoctor(processes, func(pid int) error {
		stopped = append(stopped, pid)
		return nil
	})
	result, err := doctor.Check(project, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.HasUnfixedBlockingIssues() || len(result.Fixes) != 1 || len(stopped) != 1 || stopped[0] != 99 {
		t.Fatalf("unexpected result: %+v, stopped=%v", result, stopped)
	}
}

func TestRuntimeDoctorStopsGenericEditorBundledLicensingClientWhileHubRuns(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	processes := []processInfo{
		{PID: 77, Name: "Unity Hub", Command: `/Applications/Unity Hub.app/Contents/MacOS/Unity Hub`},
		{
			PID:     99,
			Name:    "Unity.Licensing.Client",
			Command: `/Applications/Unity/Hub/Editor/6000.4.11f1/Unity.app/Contents/Helpers/UnityLicensingClient.app/Contents/MacOS/Unity.Licensing.Client --namedPipe Unity-LicenseClient-developer`,
		},
	}
	var stopped int
	doctor := testRuntimeDoctor(processes, func(pid int) error {
		stopped = pid
		return nil
	})
	result, err := doctor.Check(project, true)
	if err != nil || result.HasUnfixedBlockingIssues() || stopped != 99 {
		t.Fatalf("unexpected result: %+v, stopped=%d, err=%v", result, stopped, err)
	}
}

func TestRuntimeDoctorPreservesOpaqueLicensingClientWhileHubRuns(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	processes := []processInfo{
		{PID: 77, Name: "Unity Hub", Command: `/Applications/Unity Hub.app/Contents/MacOS/Unity Hub`},
		{PID: 99, Name: "Unity.Licensing.Client"},
	}
	doctor := testRuntimeDoctor(processes, func(pid int) error { t.Fatalf("unexpected stop %d", pid); return nil })
	result, err := doctor.Check(project, true)
	if err != nil || result.HasIssues() {
		t.Fatalf("unexpected result: %+v, err=%v", result, err)
	}
}

func TestEditorOwnedUnityLicensingClientDetection(t *testing.T) {
	tests := []struct {
		name    string
		process processInfo
		want    bool
	}{
		{
			name:    "version scoped pipe",
			process: processInfo{Name: "Unity.Licensing.Client", Command: `Unity.Licensing.Client --namedPipe "Unity-LicenseClient-developer-6000.4.11"`},
			want:    true,
		},
		{
			name:    "editor bundled generic pipe",
			process: processInfo{Name: "Unity.Licensing.Client", Command: `/Applications/Unity/Hub/Editor/6000.4.11f1/Unity.app/Contents/Helpers/UnityLicensingClient.app/Contents/MacOS/Unity.Licensing.Client --namedPipe Unity-LicenseClient-developer`},
			want:    true,
		},
		{
			name:    "hub generic pipe",
			process: processInfo{Name: "Unity.Licensing.Client", Command: `/Applications/Unity Hub.app/Contents/Frameworks/UnityLicensingClient_V1.app/Contents/MacOS/Unity.Licensing.Client --namedPipe=Unity-LicenseClient-developer`},
			want:    false,
		},
		{
			name:    "opaque client",
			process: processInfo{Name: "Unity.Licensing.Client"},
			want:    false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isEditorOwnedUnityLicensingClient(test.process); got != test.want {
				t.Fatalf("isEditorOwnedUnityLicensingClient() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestRuntimeDoctorDoesNotFixInvalidILPPPid(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	pidFile := filepath.Join(project, "Library", "ilpp.pid")
	writeRuntimeFile(t, pidFile, "not-a-pid\n")
	doctor := testRuntimeDoctor(nil, nil)
	result, err := doctor.Check(project, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasUnfixedBlockingIssues() || result.HasFixableIssues() || len(result.Fixes) != 0 || result.Issues[0].PID != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertRuntimeFileExists(t, pidFile)
}

func TestRuntimeDoctorDoesNotStopLicensingClientWhileAnyEditorRuns(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	processes := []processInfo{
		{PID: 77, Command: "/Applications/Unity/Unity.app/Contents/MacOS/Unity -projectPath /other"},
		{PID: 99, Name: "Unity.Licensing.Client", Command: "Unity.Licensing.Client"},
	}
	doctor := testRuntimeDoctor(processes, func(pid int) error { t.Fatalf("unexpected stop %d", pid); return nil })
	result, err := doctor.Check(project, true)
	if err != nil || result.HasIssues() {
		t.Fatalf("unexpected result: %+v, err=%v", result, err)
	}
}

func TestRuntimeDoctorFixesStaleLockfileWhenProcessScanFails(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	lockfile := filepath.Join(project, "Temp", "UnityLockfile")
	pidFile := filepath.Join(project, "Library", "ilpp.pid")
	writeRuntimeFile(t, lockfile, "")
	writeRuntimeFile(t, pidFile, "88\n")
	doctor := &RuntimeDoctor{
		listProcesses: func() ([]processInfo, error) { return nil, errors.New("scan failed") },
		stopProcess:   func(int) error { return nil },
		probeLockfile: func(string) (bool, error) { return false, nil },
	}
	result, err := doctor.Check(project, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasUnfixedBlockingIssues() || len(result.Fixes) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertRuntimeFileMissing(t, lockfile)
	assertRuntimeFileExists(t, pidFile)
}

func TestRuntimeDoctorDoesNotFixLockfileWhenUnityCommandLineIsUnavailable(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	lockfile := filepath.Join(project, "Temp", "UnityLockfile")
	writeRuntimeFile(t, lockfile, "")
	doctor := testRuntimeDoctor([]processInfo{{PID: 123, Name: "Unity.exe"}}, nil)
	result, err := doctor.Check(project, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasUnfixedBlockingIssues() || len(result.Fixes) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertRuntimeFileExists(t, lockfile)
}

func TestRuntimeDoctorDoesNotFixILPPPidWhenCommandLineIsUnavailable(t *testing.T) {
	project := makeRuntimeDoctorProject(t)
	pidFile := filepath.Join(project, "Library", "ilpp.pid")
	writeRuntimeFile(t, pidFile, "88\n")
	doctor := testRuntimeDoctor([]processInfo{{PID: 88, Name: "dotnet"}}, nil)
	result, err := doctor.Check(project, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasUnfixedBlockingIssues() || len(result.Fixes) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertRuntimeFileExists(t, pidFile)
}

func TestParsePSRuntimeProcesses(t *testing.T) {
	got := parsePSRuntimeProcesses("  123     1 /Applications/Unity/Unity -projectPath /tmp/project\ninvalid\n  77 notnum /bin/x\n  88\n")
	if len(got) != 1 || got[0].PID != 123 || got[0].PPID != 1 || got[0].Name != "Unity" {
		t.Fatalf("unexpected processes: %+v", got)
	}
	if got[0].Command != "/Applications/Unity/Unity -projectPath /tmp/project" {
		t.Fatalf("unexpected command: %q", got[0].Command)
	}
}

func TestParseWindowsRuntimeProcesses(t *testing.T) {
	output := []byte(`[
  {"ProcessId":123,"ParentProcessId":100,"Name":"Unity.exe","CommandLine":"Unity.exe -projectPath C:\\Projects\\X","CreationDate":"20260717120000.000000+540"},
  {"ProcessId":200,"Name":"Unity.Licensing.Client.exe","CommandLine":null}
]`)
	got, err := parseWindowsRuntimeProcesses(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected processes: %+v", got)
	}
	if got[0].PID != 123 || got[0].PPID != 100 || got[0].Name != "Unity.exe" || got[0].Command == "" {
		t.Fatalf("unexpected first process: %+v", got[0])
	}
	if got[0].Created != "20260717120000.000000+540" {
		t.Fatalf("unexpected creation date: %+v", got[0])
	}
	if got[1].PID != 200 || got[1].PPID != 0 || got[1].Command != "" || got[1].Created != "" {
		t.Fatalf("unexpected second process: %+v", got[1])
	}
}

func TestListProcessesPowerShellSelectsParentProcessId(t *testing.T) {
	for _, field := range []string{"ParentProcessId", "CreationDate"} {
		if !strings.Contains(listProcessesPowerShell, field) {
			t.Fatalf("PowerShell query must select %s for descendant collection", field)
		}
	}
}

func testRuntimeDoctor(processes []processInfo, stopper processStopper) *RuntimeDoctor {
	if stopper == nil {
		stopper = func(int) error { return nil }
	}
	return &RuntimeDoctor{
		listProcesses: func() ([]processInfo, error) { return processes, nil },
		stopProcess:   stopper,
		probeLockfile: func(path string) (bool, error) {
			projectPath := filepath.Dir(filepath.Dir(path))
			for _, process := range processes {
				if isUnityEditorProcessName(process.Name) && process.Command == "" {
					return false, errors.New("lock state unavailable")
				}
			}
			return findUnityProcessInProcesses(processes, projectPath) != 0, nil
		},
	}
}

func makeRuntimeDoctorProject(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	for _, dir := range []string{"Temp", "Library"} {
		if err := os.MkdirAll(filepath.Join(project, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return project
}

func writeRuntimeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertRuntimeFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertRuntimeFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, got %v", path, err)
	}
}
