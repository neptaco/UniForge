package unity

import (
	"context"
	"os/exec"
	"time"

	"github.com/neptaco/uniforge/pkg/ui"
)

// Grace periods for leftover Unity child processes: how long they get to exit
// on their own after their Editor died, then how long a SIGTERM gets before
// escalating to SIGKILL. Variables so tests can shorten them.
var (
	treeReapNaturalGrace = 5 * time.Second
	treeReapTermGrace    = 5 * time.Second
)

// collectDescendants returns every process in the snapshot that is a direct or
// transitive child of rootPID. The root itself is never included. The snapshot
// must be taken before the root is killed: once the root dies, its children are
// reparented (PPID 1 on Unix) and can no longer be attributed to it.
func collectDescendants(processes []processInfo, rootPID int) []processInfo {
	byPID := make(map[int]processInfo, len(processes))
	childrenByParent := make(map[int][]processInfo, len(processes))
	for _, process := range processes {
		byPID[process.PID] = process
		if process.PID == process.PPID {
			continue
		}
		childrenByParent[process.PPID] = append(childrenByParent[process.PPID], process)
	}

	var descendants []processInfo
	visited := map[int]bool{rootPID: true}
	queue := []int{rootPID}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range childrenByParent[parent] {
			if visited[child.PID] {
				continue
			}
			// Windows records the creator PID, which may have been reused: a
			// "child" created before its "parent" is a reuse artifact.
			if parentInfo, ok := byPID[parent]; ok &&
				child.Created != "" && parentInfo.Created != "" && child.Created < parentInfo.Created {
				continue
			}
			visited[child.PID] = true
			descendants = append(descendants, child)
			queue = append(queue, child.PID)
		}
	}
	return descendants
}

// processTreeReaper cleans up child processes (licensing client, asset import
// workers, ILPP) left behind after their Unity Editor root has been killed.
type processTreeReaper struct {
	listProcesses processLister
	isAlive       func(pid int) bool
	terminate     func(pid int) error
	kill          func(pid int) error
	pollInterval  time.Duration
}

func newProcessTreeReaper() *processTreeReaper {
	return &processTreeReaper{
		listProcesses: listRuntimeProcesses,
		isAlive:       isProcessAlive,
		terminate:     terminateProcessByPID,
		kill:          killProcessByPID,
		pollInterval:  200 * time.Millisecond,
	}
}

// snapshotDescendants lists the current descendants of rootPID. Best-effort:
// a failed process scan yields nil rather than blocking the caller's shutdown.
func (r *processTreeReaper) snapshotDescendants(rootPID int) []processInfo {
	processes, err := r.listProcesses()
	if err != nil {
		ui.Debug("Failed to snapshot process tree", "pid", rootPID, "error", err)
		return nil
	}
	return collectDescendants(processes, rootPID)
}

// reap waits naturalGrace for the given processes to exit on their own (they
// normally do once the Editor disconnects), then terminates survivors and,
// after termGrace, kills the stragglers. Signal errors are ignored because the
// target may exit between the liveness check and the signal.
func (r *processTreeReaper) reap(descendants []processInfo, naturalGrace, termGrace time.Duration) {
	if len(descendants) == 0 {
		return
	}
	pending := make(map[int]processInfo, len(descendants))
	for _, process := range descendants {
		pending[process.PID] = process
	}

	if pending = r.confirmTracked(r.waitUntilGone(pending, naturalGrace)); len(pending) == 0 {
		return
	}
	for pid := range pending {
		ui.Debug("Terminating leftover Unity child process", "pid", pid)
		_ = r.terminate(pid)
	}
	if pending = r.confirmTracked(r.waitUntilGone(pending, termGrace)); len(pending) == 0 {
		return
	}
	for pid := range pending {
		ui.Warn("Force killing leftover Unity child process (pid %d)", pid)
		_ = r.kill(pid)
	}
}

// waitForNaturalExit gives helpers owned by a normally-quitting Editor time to
// finish their own shutdown. It never sends a signal; any verified stale state
// is handled separately after the normal application lifecycle has completed.
func (r *processTreeReaper) waitForNaturalExit(descendants []processInfo, grace time.Duration) {
	if len(descendants) == 0 {
		return
	}
	pending := make(map[int]processInfo, len(descendants))
	for _, process := range descendants {
		pending[process.PID] = process
	}
	if survivors := r.waitUntilGone(pending, grace); len(survivors) > 0 {
		ui.Debug("Unity child processes remain after normal quit grace period", "count", len(survivors))
	}
}

// waitUntilGone polls per-PID liveness (cheap: no full process listing) until
// the pids disappear or the grace period ends, returning the pids still alive.
func (r *processTreeReaper) waitUntilGone(pids map[int]processInfo, grace time.Duration) map[int]processInfo {
	deadline := time.Now().Add(grace)
	for {
		alive := make(map[int]processInfo)
		for pid, process := range pids {
			if r.isAlive(pid) {
				alive[pid] = process
			}
		}
		pids = alive
		if len(pids) == 0 || time.Now().After(deadline) {
			return pids
		}
		time.Sleep(r.pollInterval)
	}
}

// confirmTracked re-verifies identity with one full process listing right
// before signalling: a PID may have been reused by an unrelated process since
// the snapshot, and a changed executable name means it is no longer ours.
func (r *processTreeReaper) confirmTracked(pids map[int]processInfo) map[int]processInfo {
	if len(pids) == 0 {
		return pids
	}
	processes, err := r.listProcesses()
	if err != nil {
		// Identity cannot be re-verified against PID reuse: leaving orphans
		// (the doctor cleans them up later) is safer than signalling
		// unverified PIDs.
		ui.Debug("Skipping leftover Unity child cleanup: process scan failed", "error", err)
		return nil
	}
	confirmed := make(map[int]processInfo)
	for _, process := range processes {
		original, tracked := pids[process.PID]
		if !tracked {
			continue
		}
		if original.Name != "" && process.Name != "" && original.Name != process.Name {
			continue
		}
		confirmed[process.PID] = original
	}
	return confirmed
}

// newUnityBatchCommand builds the exec.Cmd used to run Unity in batch mode,
// making context cancellation (batch timeout) take down the whole Unity
// process tree instead of only the Editor process. Without this, exec's
// default Cancel SIGKILLs the Editor alone and leaves the licensing client and
// asset import workers behind — a leftover licensing client can wedge holding
// the license mutex and break every later Editor startup. WaitDelay bounds the
// post-exit I/O drain so a leftover grandchild holding stdout cannot block
// Wait forever.
func newUnityBatchCommand(ctx context.Context, editorPath string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, editorPath, args...)
	cmd.Cancel = func() error {
		process := cmd.Process
		if process == nil {
			return nil
		}
		reaper := newProcessTreeReaper()
		descendants := reaper.snapshotDescendants(process.Pid)
		err := process.Kill()
		reaper.reap(descendants, treeReapNaturalGrace, treeReapTermGrace)
		return err
	}
	cmd.WaitDelay = 15 * time.Second
	return cmd
}
