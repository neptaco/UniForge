package unity

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"
)

func descendantPIDs(processes []processInfo, rootPID int) []int {
	descendants := collectDescendants(processes, rootPID)
	pids := make([]int, 0, len(descendants))
	for _, process := range descendants {
		pids = append(pids, process.PID)
	}
	sort.Ints(pids)
	return pids
}

func TestCollectDescendantsFindsChildrenAndGrandchildren(t *testing.T) {
	processes := []processInfo{
		{PID: 1, PPID: 0, Name: "launchd"},
		{PID: 100, PPID: 1, Name: "Unity"},
		{PID: 200, PPID: 100, Name: "Unity.Licensing.Client"},
		{PID: 300, PPID: 200, Name: "worker"},
		{PID: 400, PPID: 1, Name: "unrelated"},
	}
	got := descendantPIDs(processes, 100)
	want := []int{200, 300}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("descendants = %v, want %v", got, want)
	}
}

func TestCollectDescendantsExcludesRootItself(t *testing.T) {
	processes := []processInfo{
		{PID: 100, PPID: 1, Name: "Unity"},
	}
	if got := descendantPIDs(processes, 100); len(got) != 0 {
		t.Fatalf("descendants = %v, want empty", got)
	}
}

func TestCollectDescendantsFindsChildrenOfMissingRoot(t *testing.T) {
	// The snapshot may miss the root itself (e.g. it exited between kill and
	// listing), while children still reference its PID.
	processes := []processInfo{
		{PID: 200, PPID: 100, Name: "Unity.Licensing.Client"},
	}
	got := descendantPIDs(processes, 100)
	if len(got) != 1 || got[0] != 200 {
		t.Fatalf("descendants = %v, want [200]", got)
	}
}

func TestCollectDescendantsSurvivesPPIDCycles(t *testing.T) {
	processes := []processInfo{
		{PID: 2, PPID: 3},
		{PID: 3, PPID: 2},
	}
	got := descendantPIDs(processes, 2)
	if len(got) != 1 || got[0] != 3 {
		t.Fatalf("descendants = %v, want [3]", got)
	}
}

func TestCollectDescendantsIgnoresSelfParented(t *testing.T) {
	processes := []processInfo{
		{PID: 100, PPID: 100, Name: "weird"},
		{PID: 200, PPID: 100, Name: "child"},
	}
	got := descendantPIDs(processes, 100)
	if len(got) != 1 || got[0] != 200 {
		t.Fatalf("descendants = %v, want [200]", got)
	}
}

func TestCollectDescendantsEmptySnapshot(t *testing.T) {
	if got := descendantPIDs(nil, 100); len(got) != 0 {
		t.Fatalf("descendants = %v, want empty", got)
	}
}

func TestCollectDescendantsRejectsChildOlderThanParent(t *testing.T) {
	// Windows Win32_Process.ParentProcessId records the creator PID, which may
	// have been reused: a "child" created before its "parent" is a reuse
	// artifact, not a real descendant.
	processes := []processInfo{
		{PID: 100, PPID: 1, Name: "Unity", Created: "20260717120000.000000+540"},
		{PID: 200, PPID: 100, Name: "old-daemon", Created: "20260717110000.000000+540"},
		{PID: 300, PPID: 100, Name: "Unity.Licensing.Client", Created: "20260717120100.000000+540"},
	}
	got := descendantPIDs(processes, 100)
	if len(got) != 1 || got[0] != 300 {
		t.Fatalf("descendants = %v, want [300]", got)
	}
}

func TestCollectDescendantsKeepsChildrenWithoutCreationInfo(t *testing.T) {
	processes := []processInfo{
		{PID: 100, PPID: 1, Name: "Unity"},
		{PID: 200, PPID: 100, Name: "Unity.Licensing.Client"},
	}
	got := descendantPIDs(processes, 100)
	if len(got) != 1 || got[0] != 200 {
		t.Fatalf("descendants = %v, want [200]", got)
	}
}

// fakeProcessTable simulates a shrinking process table for reaper tests.
type fakeProcessTable struct {
	alive      map[int]bool
	names      map[int]string
	ignoreTerm map[int]bool
	termCalls  []int
	killCalls  []int
}

func newFakeProcessTable(pids ...int) *fakeProcessTable {
	table := &fakeProcessTable{alive: map[int]bool{}, names: map[int]string{}, ignoreTerm: map[int]bool{}}
	for _, pid := range pids {
		table.alive[pid] = true
	}
	return table
}

func (f *fakeProcessTable) reaper() *processTreeReaper {
	return &processTreeReaper{
		listProcesses: func() ([]processInfo, error) {
			var processes []processInfo
			for pid, alive := range f.alive {
				if alive {
					processes = append(processes, processInfo{PID: pid, PPID: 1, Name: f.names[pid]})
				}
			}
			return processes, nil
		},
		isAlive: func(pid int) bool { return f.alive[pid] },
		terminate: func(pid int) error {
			f.termCalls = append(f.termCalls, pid)
			if !f.ignoreTerm[pid] {
				f.alive[pid] = false
			}
			return nil
		},
		kill: func(pid int) error {
			f.killCalls = append(f.killCalls, pid)
			f.alive[pid] = false
			return nil
		},
		pollInterval: time.Millisecond,
	}
}

func reapTargets(pids ...int) []processInfo {
	targets := make([]processInfo, 0, len(pids))
	for _, pid := range pids {
		targets = append(targets, processInfo{PID: pid})
	}
	return targets
}

func TestReapDoesNothingWhenDescendantsAlreadyExited(t *testing.T) {
	table := newFakeProcessTable()
	table.reaper().reap(reapTargets(200, 300), 20*time.Millisecond, 20*time.Millisecond)
	if len(table.termCalls) != 0 || len(table.killCalls) != 0 {
		t.Fatalf("no signals expected, got term=%v kill=%v", table.termCalls, table.killCalls)
	}
}

func TestWaitForNaturalExitNeverSignalsSurvivors(t *testing.T) {
	table := newFakeProcessTable(200)
	table.reaper().waitForNaturalExit(reapTargets(200), 5*time.Millisecond)
	if len(table.termCalls) != 0 || len(table.killCalls) != 0 {
		t.Fatalf("normal quit grace must not signal survivors, got term=%v kill=%v", table.termCalls, table.killCalls)
	}
	if !table.alive[200] {
		t.Fatal("normal quit grace unexpectedly stopped the surviving process")
	}
}

func TestReapTerminatesSurvivorsAfterNaturalGrace(t *testing.T) {
	table := newFakeProcessTable(200)
	table.reaper().reap(reapTargets(200), 5*time.Millisecond, 50*time.Millisecond)
	if len(table.termCalls) != 1 || table.termCalls[0] != 200 {
		t.Fatalf("expected TERM for 200, got %v", table.termCalls)
	}
	if len(table.killCalls) != 0 {
		t.Fatalf("no KILL expected, got %v", table.killCalls)
	}
}

func TestReapKillsSurvivorsIgnoringTerm(t *testing.T) {
	table := newFakeProcessTable(200)
	table.ignoreTerm[200] = true
	table.reaper().reap(reapTargets(200), 5*time.Millisecond, 10*time.Millisecond)
	if len(table.termCalls) != 1 || table.termCalls[0] != 200 {
		t.Fatalf("expected TERM for 200, got %v", table.termCalls)
	}
	if len(table.killCalls) != 1 || table.killCalls[0] != 200 {
		t.Fatalf("expected KILL for 200, got %v", table.killCalls)
	}
}

func TestReapDoesNotSignalWhenIdentityCannotBeConfirmed(t *testing.T) {
	// If the full process scan fails right before signalling, the PIDs cannot
	// be re-verified against reuse; leaving orphans (doctor cleans them up
	// later) is safer than signalling unverified PIDs.
	table := newFakeProcessTable(200)
	reaper := table.reaper()
	reaper.listProcesses = func() ([]processInfo, error) { return nil, errors.New("scan failed") }
	reaper.reap(reapTargets(200), 5*time.Millisecond, 10*time.Millisecond)
	if len(table.termCalls) != 0 || len(table.killCalls) != 0 {
		t.Fatalf("unverified PIDs must not be signalled, got term=%v kill=%v", table.termCalls, table.killCalls)
	}
}

func TestReapSkipsReusedPIDWithDifferentName(t *testing.T) {
	table := newFakeProcessTable()
	// PID 200 was snapshotted as Unity.Licensing.Client, but by reap time the
	// PID belongs to an unrelated process: it must not be signalled.
	table.alive[200] = true
	table.names = map[int]string{200: "unrelated-daemon"}
	reaper := table.reaper()
	reaper.reap([]processInfo{{PID: 200, Name: "Unity.Licensing.Client"}}, 5*time.Millisecond, 10*time.Millisecond)
	if len(table.termCalls) != 0 || len(table.killCalls) != 0 {
		t.Fatalf("reused PID must not be signalled, got term=%v kill=%v", table.termCalls, table.killCalls)
	}
}

func TestReapOnlyTouchesGivenDescendants(t *testing.T) {
	table := newFakeProcessTable(200, 999)
	table.ignoreTerm[200] = true
	table.reaper().reap(reapTargets(200), 5*time.Millisecond, 10*time.Millisecond)
	for _, pid := range append(table.termCalls, table.killCalls...) {
		if pid == 999 {
			t.Fatal("unrelated process 999 must not be signalled")
		}
	}
}

func TestReapEmptyDescendantsReturnsImmediately(t *testing.T) {
	reaper := &processTreeReaper{
		listProcesses: func() ([]processInfo, error) {
			t.Fatal("listProcesses must not be called for empty descendants")
			return nil, nil
		},
		pollInterval: time.Millisecond,
	}
	reaper.reap(nil, time.Second, time.Second)
}

func TestSnapshotDescendantsReturnsNilOnScanFailure(t *testing.T) {
	reaper := &processTreeReaper{
		listProcesses: func() ([]processInfo, error) { return nil, errors.New("scan failed") },
		pollInterval:  time.Millisecond,
	}
	if got := reaper.snapshotDescendants(100); got != nil {
		t.Fatalf("expected nil on scan failure, got %v", got)
	}
}

func TestNewUnityBatchCommandConfiguresTreeCleanup(t *testing.T) {
	cmd := newUnityBatchCommand(context.Background(), "unity-editor", "-batchmode")
	if cmd.Cancel == nil {
		t.Fatal("Cancel must be configured to reap the Unity process tree")
	}
	if cmd.WaitDelay <= 0 {
		t.Fatal("WaitDelay must bound the post-exit I/O drain")
	}
}
