package unity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	projectPathArgumentPattern = regexp.MustCompile(`(?i)(?:^|\s)-projectPath(?:=|\s+)(?:"([^"]+)"|'([^']+)'|(\S+))`)
	namedPipeArgumentPattern   = regexp.MustCompile(`(?i)(?:^|\s)--namedPipe(?:=|\s+)(?:"([^"]+)"|'([^']+)'|(\S+))`)
	editorLicensePipePattern   = regexp.MustCompile(`(?i)^Unity-LicenseClient-.+-\d+\.\d+\.\d+$`)
)

const (
	RuntimeIssueActiveEditor          = "active_editor"
	RuntimeIssueStaleLockfile         = "stale_lockfile"
	RuntimeIssueUnverifiedLockfile    = "unverified_lockfile"
	RuntimeIssueActiveILPP            = "active_ilpp"
	RuntimeIssueStaleILPPPid          = "stale_ilpp_pid"
	RuntimeIssueOrphanLicensingClient = "orphan_licensing_client"
	RuntimeIssueProcessScanFailed     = "process_scan_failed"
)

const listProcessesPowerShell = `$ErrorActionPreference = 'Stop'
$OutputEncoding = [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$processes = @(Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId, Name, CommandLine, @{Name='CreationDate';Expression={$_.CreationDate.ToString('yyyyMMddHHmmss.ffffff+000')}})
ConvertTo-Json -InputObject $processes -Compress`

// listProcessesTimeout bounds each ps / PowerShell invocation so a wedged
// process scan cannot hang callers (notably exec.Cmd.Cancel, whose WaitDelay
// only starts after Cancel returns).
const listProcessesTimeout = 5 * time.Second

type RuntimeDoctor struct {
	listProcesses processLister
	stopProcess   processStopper
	probeLockfile unityLockfileProbe
}

type processLister func() ([]processInfo, error)
type processStopper func(pid int) error

type processInfo struct {
	PID     int
	PPID    int
	Name    string
	Command string
	// Created is the CIM creation timestamp on Windows (lexicographically
	// comparable); empty on Unix. Used to reject parent-PID-reuse artifacts.
	Created string
}

type RuntimeDoctorResult struct {
	ProjectPath string
	EditorPID   int
	Issues      []RuntimeIssue
	Fixes       []RuntimeFix
}

type RuntimeIssue struct {
	Kind     string
	Message  string
	Path     string
	PID      int
	Blocking bool
	Fixed    bool
}

type RuntimeFix struct {
	Kind    string
	Message string
	Path    string
	PID     int
}

func NewRuntimeDoctor() *RuntimeDoctor {
	return &RuntimeDoctor{
		listProcesses: listRuntimeProcesses,
		stopProcess:   stopRuntimeProcess,
		probeLockfile: probeUnityLockfile,
	}
}

func (d *RuntimeDoctor) Check(projectPath string, fix bool) (*RuntimeDoctorResult, error) {
	absPath, err := normalizeProjectPath(projectPath)
	if err != nil {
		return nil, fmt.Errorf("get absolute project path: %w", err)
	}
	result := &RuntimeDoctorResult{ProjectPath: absPath}
	processes, processErr := d.listProcesses()
	processesKnown := processErr == nil
	if processErr != nil {
		result.addIssue(RuntimeIssue{Kind: RuntimeIssueProcessScanFailed, Message: fmt.Sprintf("failed to list processes: %v", processErr)})
	} else {
		result.EditorPID = findUnityProcessInProcesses(processes, absPath)
	}

	processByPID := make(map[int]processInfo, len(processes))
	hasAnyUnityEditor := false
	hasUnityHub := false
	hasOpaqueUnityEditor := false
	var licensingClients []processInfo
	for _, process := range processes {
		processByPID[process.PID] = process
		hasAnyUnityEditor = hasAnyUnityEditor || isUnityEditorCommandLine(process.Command)
		hasUnityHub = hasUnityHub || isUnityHub(process)
		hasOpaqueUnityEditor = hasOpaqueUnityEditor || (isUnityEditorProcessName(process.Name) && strings.TrimSpace(process.Command) == "")
		if isUnityLicensingClient(process) {
			licensingClients = append(licensingClients, process)
		}
	}

	if err := d.checkLockfile(result, absPath, fix); err != nil {
		return result, err
	}
	if err := d.checkILPPPid(result, absPath, processByPID, processesKnown, fix); err != nil {
		return result, err
	}
	if err := d.checkLicensingClients(result, hasAnyUnityEditor, hasUnityHub, licensingClients, processesKnown && !hasOpaqueUnityEditor, fix); err != nil {
		return result, err
	}
	return result, nil
}

func (d *RuntimeDoctor) checkLockfile(result *RuntimeDoctorResult, projectPath string, fix bool) error {
	lockfile := filepath.Join(projectPath, "Temp", "UnityLockfile")
	if _, err := os.Stat(lockfile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat Unity lockfile: %w", err)
	}
	probeLockfile := d.probeLockfile
	if probeLockfile == nil {
		probeLockfile = probeUnityLockfile
	}
	held, err := probeLockfile(lockfile)
	if err != nil {
		result.addIssue(RuntimeIssue{Kind: RuntimeIssueUnverifiedLockfile, Message: fmt.Sprintf("Unity lockfile exists, but its OS lock state could not be inspected: %v", err), Path: lockfile, Blocking: true})
		return nil
	}
	if held {
		result.addIssue(RuntimeIssue{Kind: RuntimeIssueActiveEditor, Message: "Unity Editor is running for this project", Path: lockfile, PID: result.EditorPID, Blocking: true})
		return nil
	}
	index := result.addIssue(RuntimeIssue{Kind: RuntimeIssueStaleLockfile, Message: "stale Unity lockfile exists without a matching Unity Editor process", Path: lockfile, Blocking: true})
	if fix {
		if err := os.Remove(lockfile); err != nil {
			return fmt.Errorf("remove stale Unity lockfile %s: %w", lockfile, err)
		}
		result.Issues[index].Fixed = true
		result.addFix(RuntimeFix{Kind: RuntimeIssueStaleLockfile, Message: "removed stale Unity lockfile", Path: lockfile})
	}
	return nil
}

func (d *RuntimeDoctor) checkILPPPid(result *RuntimeDoctorResult, projectPath string, processes map[int]processInfo, processesKnown, fix bool) error {
	pidFile := filepath.Join(projectPath, "Library", "ilpp.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read ILPP pid file: %w", err)
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if parseErr != nil {
		result.addIssue(RuntimeIssue{Kind: RuntimeIssueProcessScanFailed, Message: "ILPP pid file is invalid, so process state could not be verified", Path: pidFile, Blocking: true})
		return nil
	}
	if !processesKnown {
		result.addIssue(RuntimeIssue{Kind: RuntimeIssueProcessScanFailed, Message: "ILPP pid file exists, but process state could not be verified", Path: pidFile, PID: pid, Blocking: true})
		return nil
	}
	if process, ok := processes[pid]; ok {
		if isUnityILPPProcess(process) {
			result.addIssue(RuntimeIssue{Kind: RuntimeIssueActiveILPP, Message: "Unity IL post processor is still running", Path: pidFile, PID: pid})
			return nil
		}
		if strings.TrimSpace(process.Command) == "" {
			result.addIssue(RuntimeIssue{Kind: RuntimeIssueProcessScanFailed, Message: "ILPP pid is active, but its command line could not be inspected", Path: pidFile, PID: pid, Blocking: true})
			return nil
		}
	}
	index := result.addIssue(RuntimeIssue{Kind: RuntimeIssueStaleILPPPid, Message: "stale Unity IL post processor pid file exists", Path: pidFile, PID: pid, Blocking: true})
	if fix {
		if err := os.Remove(pidFile); err != nil {
			return fmt.Errorf("remove stale ILPP pid file %s: %w", pidFile, err)
		}
		result.Issues[index].Fixed = true
		result.addFix(RuntimeFix{Kind: RuntimeIssueStaleILPPPid, Message: "removed stale Unity IL post processor pid file", Path: pidFile})
	}
	return nil
}

func (d *RuntimeDoctor) checkLicensingClients(result *RuntimeDoctorResult, hasAnyUnityEditor, hasUnityHub bool, clients []processInfo, processesKnown, fix bool) error {
	if !processesKnown || hasAnyUnityEditor {
		return nil
	}
	for _, process := range clients {
		// Hub owns a generic licensing client that must stay alive while Hub is
		// running. Editor-bundled/version-scoped clients are safe to clean once
		// every Editor has exited, even if Hub itself is still open.
		if hasUnityHub && !isEditorOwnedUnityLicensingClient(process) {
			continue
		}
		index := result.addIssue(RuntimeIssue{Kind: RuntimeIssueOrphanLicensingClient, Message: "orphan Unity licensing client is running without any Unity Editor process", PID: process.PID, Blocking: true})
		if !fix {
			continue
		}
		if err := d.stopProcess(process.PID); err != nil {
			return fmt.Errorf("stop orphan Unity licensing client pid %d: %w", process.PID, err)
		}
		result.Issues[index].Fixed = true
		result.addFix(RuntimeFix{Kind: RuntimeIssueOrphanLicensingClient, Message: "stopped orphan Unity licensing client", PID: process.PID})
	}
	return nil
}

func (r *RuntimeDoctorResult) HasUnfixedBlockingIssues() bool {
	for _, issue := range r.Issues {
		if issue.Blocking && !issue.Fixed {
			return true
		}
	}
	return false
}

func (r *RuntimeDoctorResult) HasIssues() bool { return len(r.Issues) > 0 }

func (r *RuntimeDoctorResult) HasFixableIssues() bool {
	for _, issue := range r.Issues {
		if issue.Fixed {
			continue
		}
		switch issue.Kind {
		case RuntimeIssueStaleLockfile, RuntimeIssueStaleILPPPid, RuntimeIssueOrphanLicensingClient:
			return true
		}
	}
	return false
}

func (r *RuntimeDoctorResult) addIssue(issue RuntimeIssue) int {
	r.Issues = append(r.Issues, issue)
	return len(r.Issues) - 1
}

func (r *RuntimeDoctorResult) addFix(fix RuntimeFix) { r.Fixes = append(r.Fixes, fix) }

func listRuntimeProcesses() ([]processInfo, error) {
	if runtime.GOOS == "windows" {
		return listRuntimeProcessesWindows()
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	ctx, cancel := context.WithTimeout(context.Background(), listProcessesTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ps", "-ax", "-o", "pid=,ppid=,command=").Output()
	if err != nil {
		return nil, err
	}
	return parsePSRuntimeProcesses(string(output)), nil
}

func listRuntimeProcessesWindows() ([]processInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), listProcessesTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", listProcessesPowerShell).Output()
	if err != nil {
		return nil, fmt.Errorf("list processes with PowerShell/CIM: %w", err)
	}
	processes, err := parseWindowsRuntimeProcesses(output)
	if err != nil {
		return nil, fmt.Errorf("parse PowerShell/CIM process output: %w", err)
	}
	return processes, nil
}

func parseWindowsRuntimeProcesses(output []byte) ([]processInfo, error) {
	var rows []struct {
		ProcessID       int     `json:"ProcessId"`
		ParentProcessID int     `json:"ParentProcessId"`
		Name            string  `json:"Name"`
		CommandLine     *string `json:"CommandLine"`
		CreationDate    *string `json:"CreationDate"`
	}
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, err
	}
	processes := make([]processInfo, 0, len(rows))
	for _, row := range rows {
		command := ""
		if row.CommandLine != nil {
			command = *row.CommandLine
		}
		created := ""
		if row.CreationDate != nil {
			created = *row.CreationDate
		}
		processes = append(processes, processInfo{PID: row.ProcessID, PPID: row.ParentProcessID, Name: row.Name, Command: command, Created: created})
	}
	return processes, nil
}

func parsePSRuntimeProcesses(output string) []processInfo {
	var processes []processInfo
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		processes = append(processes, processInfo{PID: pid, PPID: ppid, Name: filepath.Base(fields[2]), Command: strings.Join(fields[2:], " ")})
	}
	return processes
}

func findUnityProcessInProcesses(processes []processInfo, projectPath string) int {
	for _, process := range processes {
		if isUnityEditorCommandLine(process.Command) && commandTargetsProject(process.Command, projectPath) {
			return process.PID
		}
	}
	return 0
}

func isUnityLicensingClient(process processInfo) bool {
	name := normalizedExecutableName(process)
	return name == "unity.licensing.client" || name == "unity.licensing.client.exe" ||
		name == "unitylicensingclient" || name == "unitylicensingclient.exe"
}

func isEditorOwnedUnityLicensingClient(process processInfo) bool {
	if !isUnityLicensingClient(process) {
		return false
	}

	if editorLicensePipePattern.MatchString(namedPipeArgument(process.Command)) {
		return true
	}

	command := strings.ToLower(strings.ReplaceAll(process.Command, `\`, "/"))
	return strings.Contains(command, "/unity.app/contents/helpers/unitylicensingclient") ||
		(strings.Contains(command, "/editor/") && strings.Contains(command, "unity.licensing.client"))
}

func namedPipeArgument(command string) string {
	match := namedPipeArgumentPattern.FindStringSubmatch(command)
	if match == nil {
		return ""
	}
	for _, value := range match[1:] {
		if value != "" {
			return value
		}
	}
	return ""
}

func isUnityHub(process processInfo) bool {
	name := normalizedExecutableName(process)
	return name == "unity hub" || name == "unity hub.exe" || name == "unityhub" || name == "unityhub.exe"
}

func normalizedExecutableName(process processInfo) string {
	name := strings.Trim(strings.TrimSpace(process.Name), `"`)
	if name == "" {
		command := strings.TrimSpace(process.Command)
		if strings.HasPrefix(command, `"`) {
			if end := strings.Index(command[1:], `"`); end >= 0 {
				command = command[1 : end+1]
			}
		} else if fields := strings.Fields(command); len(fields) > 0 {
			command = fields[0]
		}
		name = filepath.Base(strings.ReplaceAll(command, `\`, "/"))
	}
	return strings.ToLower(name)
}

func commandTargetsProject(command, projectPath string) bool {
	return commandTargetsProjectForOS(command, projectPath, runtime.GOOS)
}

func commandTargetsProjectForOS(command, projectPath, goos string) bool {
	match := projectPathArgumentPattern.FindStringSubmatch(command)
	if match == nil {
		return false
	}
	candidate := ""
	for _, value := range match[1:] {
		if value != "" {
			candidate = value
			break
		}
	}
	if goos == "windows" {
		if runtime.GOOS == "windows" {
			normalizedCandidate, candidateErr := normalizeProjectPath(candidate)
			normalizedProject, projectErr := normalizeProjectPath(projectPath)
			if candidateErr == nil && projectErr == nil {
				candidate = normalizedCandidate
				projectPath = normalizedProject
			}
		}
		return normalizeWindowsPath(strings.TrimSpace(candidate)) == normalizeWindowsPath(strings.TrimSpace(projectPath))
	}
	normalizedCandidate, err := normalizeProjectPath(candidate)
	if err != nil {
		return false
	}
	normalizedProject, err := normalizeProjectPath(projectPath)
	return err == nil && normalizedCandidate == normalizedProject
}

func normalizeProjectPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if os.IsNotExist(err) {
		return filepath.Clean(absPath), nil
	}
	return "", err
}

func isUnityEditorProcessName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "unity" || name == "unity.exe"
}

func isUnityILPPProcess(process processInfo) bool {
	value := strings.ToLower(process.Name + " " + process.Command)
	return strings.Contains(value, "unity.ilpp") || strings.Contains(value, "unity.assemblypostprocessor") || strings.Contains(value, "unity ilpp")
}

func stopRuntimeProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return process.Kill()
}
