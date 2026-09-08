package hub

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// newLockedTestClient returns a client whose install lock lives in a temp
// directory, plus a release func for a lock already held on the same editor.
func newLockedTestClient(t *testing.T, version string) *Client {
	t.Helper()

	lockDir := t.TempDir()
	release, err := acquireInstallLockAt(lockDir, version)
	if err != nil {
		t.Fatalf("hold lock for %s: %v", version, err)
	}
	t.Cleanup(func() {
		_ = release()
	})

	return &Client{
		// A path that would fail loudly if it were ever executed: the guard
		// must reject before Unity Hub is spawned.
		hubPath:         filepath.Join(t.TempDir(), "hub-must-not-run"),
		lockDirOverride: lockDir,
	}
}

func TestInstallEditorRejectsConcurrentInstall(t *testing.T) {
	client := newLockedTestClient(t, "6000.4.11f1")

	_, err := client.InstallEditorWithOptions(InstallOptions{
		Version:      "6000.4.11f1",
		Architecture: runtimeArchitecture(),
	})
	if !errors.Is(err, ErrInstallInProgress) {
		t.Fatalf("err = %v, want it to wrap ErrInstallInProgress", err)
	}
}

// Installing modules downloads into the same editor directory, so it needs the
// same guard as installing the editor itself.
func TestInstallModulesRejectsConcurrentInstall(t *testing.T) {
	client := newLockedTestClient(t, "6000.4.11f1")

	_, err := client.InstallModules("6000.4.11f1", []string{"ios"})
	if !errors.Is(err, ErrInstallInProgress) {
		t.Fatalf("err = %v, want it to wrap ErrInstallInProgress", err)
	}
}

// A run that waited on the lock must report that it installed nothing, so the
// caller does not claim a successful install it never performed.
func TestInstallEditorReportsAlreadyInstalledWithoutRunningHub(t *testing.T) {
	installPath := t.TempDir()
	editorPath := writeFakeEditor(t, installPath, "6000.4.11f1")

	client := &Client{
		hubPath:         filepath.Join(t.TempDir(), "hub-must-not-run"),
		lockDirOverride: t.TempDir(),
		installPath:     installPath,
		installPathInit: true,
	}

	outcome, err := client.InstallEditorWithOptions(InstallOptions{
		Version:      "6000.4.11f1",
		Architecture: runtimeArchitecture(),
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !outcome.AlreadyInstalled {
		t.Error("outcome.AlreadyInstalled = false, want true")
	}
	if outcome.Path != editorPath {
		t.Errorf("outcome.Path = %q, want %q", outcome.Path, editorPath)
	}
}

// An explicit architecture request must not be satisfied by an editor of a
// different architecture: Unity Hub installs those into separate directories.
func TestInstallEditorDoesNotSkipForADifferentArchitecture(t *testing.T) {
	installPath := t.TempDir()
	writeFakeEditor(t, installPath, "6000.4.11f1")
	writeEditorMetadata(t, installPath, "6000.4.11f1", "arm64")

	client := &Client{
		hubPath:         filepath.Join(t.TempDir(), "hub-must-not-run"),
		lockDirOverride: t.TempDir(),
		installPath:     installPath,
		installPathInit: true,
	}

	// The installed editor is arm64, so an x86_64 request must reach Unity Hub
	// rather than report the arm64 install as satisfying it. The stub hub path
	// does not exist, so reaching Hub surfaces as an error.
	outcome, err := client.InstallEditorWithOptions(InstallOptions{
		Version:      "6000.4.11f1",
		Architecture: "x86_64",
	})
	if err == nil && outcome.AlreadyInstalled {
		t.Fatal("reported the arm64 editor as satisfying an x86_64 request")
	}
}

func TestInstallEditorSkipsForTheMatchingArchitecture(t *testing.T) {
	installPath := t.TempDir()
	editorPath := writeFakeEditor(t, installPath, "6000.4.11f1")
	writeEditorMetadata(t, installPath, "6000.4.11f1", "arm64")

	client := &Client{
		hubPath:         filepath.Join(t.TempDir(), "hub-must-not-run"),
		lockDirOverride: t.TempDir(),
		installPath:     installPath,
		installPathInit: true,
	}

	outcome, err := client.InstallEditorWithOptions(InstallOptions{
		Version:      "6000.4.11f1",
		Architecture: "arm64",
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !outcome.AlreadyInstalled {
		t.Error("outcome.AlreadyInstalled = false, want true")
	}
	if outcome.Path != editorPath {
		t.Errorf("outcome.Path = %q, want %q", outcome.Path, editorPath)
	}
}

// writeEditorMetadata writes the metadata.hub.json Unity Hub leaves next to an
// installed editor, which records the architecture it installed.
func writeEditorMetadata(t *testing.T, installPath, version, architecture string) {
	t.Helper()

	path := filepath.Join(installPath, version, "metadata.hub.json")
	content := `{"architecture":"` + architecture + `","revisionHash":null,"releaseStream":"","isLTS":null}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write metadata.hub.json: %v", err)
	}
}

// writeFakeEditor creates the on-disk layout IsEditorInstalled looks for and
// returns the executable path it should report.
func writeFakeEditor(t *testing.T, installPath, version string) string {
	t.Helper()

	var execPath string
	switch runtime.GOOS {
	case "darwin":
		execPath = filepath.Join(installPath, version, "Unity.app")
	case "windows":
		execPath = filepath.Join(installPath, version, "Editor", "Unity.exe")
	default:
		execPath = filepath.Join(installPath, version, "Editor", "Unity")
	}

	if err := os.MkdirAll(filepath.Dir(execPath), 0o755); err != nil {
		t.Fatalf("create editor directory: %v", err)
	}
	if runtime.GOOS == "darwin" {
		// Unity.app is a bundle directory.
		if err := os.MkdirAll(execPath, 0o755); err != nil {
			t.Fatalf("create Unity.app: %v", err)
		}
	} else if err := os.WriteFile(execPath, []byte("fake"), 0o755); err != nil {
		t.Fatalf("create Unity executable: %v", err)
	}

	return execPath
}

// Unity Hub is never given modules it does not recognise, so reporting them as
// installed would be a lie.
func TestInstallOutcomeReportsOnlyRecognisedModules(t *testing.T) {
	client := &Client{}

	resolved := client.mapModules([]string{"ios", "definitely-not-a-module", "android"})

	for _, module := range resolved {
		if module == "definitely-not-a-module" {
			t.Fatalf("mapModules kept an unknown module: %v", resolved)
		}
	}
	if len(resolved) != 2 {
		t.Fatalf("resolved = %v, want the two known modules", resolved)
	}
}

// The install TUI supplies canonical Unity Hub module ids rather than the
// friendly aliases, so those must resolve instead of being rejected.
func TestMapModulesAcceptsCanonicalHubIDs(t *testing.T) {
	client := &Client{}

	resolved := client.mapModules([]string{"windows-il2cpp", "linux-il2cpp", "mac-il2cpp"})

	want := []string{"windows-il2cpp", "linux-il2cpp", "mac-il2cpp"}
	if len(resolved) != len(want) {
		t.Fatalf("resolved = %v, want %v", resolved, want)
	}
	for i, module := range want {
		if resolved[i] != module {
			t.Errorf("resolved[%d] = %q, want %q", i, resolved[i], module)
		}
	}
}

// editors-v2.json can record an editor by its install directory rather than by
// its executable, so architecture detection must handle both shapes.
func TestRecordedArchitectureHandlesDirectoryAndExecutablePaths(t *testing.T) {
	installPath := t.TempDir()
	execPath := writeFakeEditor(t, installPath, "6000.4.11f1")
	writeEditorMetadata(t, installPath, "6000.4.11f1", "x86_64")

	client := &Client{}
	editorRoot := filepath.Join(installPath, "6000.4.11f1")

	for _, path := range []string{editorRoot, execPath} {
		if got := client.recordedArchitecture(path); got != "x86_64" {
			t.Errorf("recordedArchitecture(%q) = %q, want x86_64", path, got)
		}
	}

	if got := client.recordedArchitecture(""); got != "" {
		t.Errorf("recordedArchitecture(\"\") = %q, want empty", got)
	}
}

// A stale metadata.hub.json in the shared install root must not be mistaken
// for this editor's, or an unrelated architecture decides the comparison.
func TestRecordedArchitectureIgnoresUnrelatedAncestorMetadata(t *testing.T) {
	installPath := t.TempDir()
	execPath := writeFakeEditor(t, installPath, "6000.4.11f1")

	// No metadata beside the editor, but one directory above it.
	stale := `{"architecture":"x86_64"}`
	if err := os.WriteFile(filepath.Join(installPath, "metadata.hub.json"), []byte(stale), 0o644); err != nil {
		t.Fatalf("write stale metadata: %v", err)
	}

	client := &Client{}
	if got := client.recordedArchitecture(execPath); got != "" {
		t.Errorf("recordedArchitecture = %q, want empty (the ancestor's metadata is not this editor's)", got)
	}
}

// Module ids must resolve regardless of case, or an installed module is seen as
// missing and Unity Hub is invoked again for nothing.
func TestResolveModuleIDIsCaseInsensitive(t *testing.T) {
	for _, input := range []string{"Windows-IL2CPP", "windows-il2cpp", "WINDOWS", "Windows"} {
		if got := resolveModuleID(input); got != "windows-il2cpp" {
			t.Errorf("resolveModuleID(%q) = %q, want windows-il2cpp", input, got)
		}
	}
	if got := resolveModuleID("definitely-not-a-module"); got != "" {
		t.Errorf("resolveModuleID(unknown) = %q, want empty", got)
	}
}

// If another process installed the bare editor while we waited, the modules we
// were asked for still have to be installed rather than silently dropped.
func TestInstallEditorInstallsModulesMissingFromAnExistingEditor(t *testing.T) {
	installPath := t.TempDir()
	writeFakeEditor(t, installPath, "6000.4.11f1")
	writeEditorMetadata(t, installPath, "6000.4.11f1", runtimeArchitecture())

	client := &Client{
		hubPath:         filepath.Join(t.TempDir(), "hub-must-not-run"),
		lockDirOverride: t.TempDir(),
		installPath:     installPath,
		installPathInit: true,
	}

	// ios is not installed in the fake editor, so the module install must run.
	// It reaches the missing hub binary, which is how we observe that it ran.
	outcome, err := client.InstallEditorWithOptions(InstallOptions{
		Version: "6000.4.11f1",
		Modules: []string{"ios"},
	})
	if err == nil && outcome.AlreadyInstalled && len(outcome.Modules) == 0 {
		t.Fatal("reported already-installed without installing the requested module")
	}
}

// An unknown module name must not turn into a hard failure just because
// another process happened to install the editor first: on the ordinary path
// it is merely warned about and dropped.
func TestInstallEditorToleratesUnknownModulesOnAnExistingEditor(t *testing.T) {
	installPath := t.TempDir()
	editorPath := writeFakeEditor(t, installPath, "6000.4.11f1")
	writeEditorMetadata(t, installPath, "6000.4.11f1", runtimeArchitecture())

	client := &Client{
		hubPath:         filepath.Join(t.TempDir(), "hub-must-not-run"),
		lockDirOverride: t.TempDir(),
		installPath:     installPath,
		installPathInit: true,
	}

	outcome, err := client.InstallEditorWithOptions(InstallOptions{
		Version: "6000.4.11f1",
		Modules: []string{"definitely-not-a-module"},
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !outcome.AlreadyInstalled {
		t.Error("outcome.AlreadyInstalled = false, want true")
	}
	if outcome.Path != editorPath {
		t.Errorf("outcome.Path = %q, want %q", outcome.Path, editorPath)
	}
	if len(outcome.Modules) != 0 {
		t.Errorf("outcome.Modules = %v, want none", outcome.Modules)
	}
}

func runtimeArchitecture() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "x86_64"
}
