package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/neptaco/uniforge/pkg/ui"
)

type Client struct {
	hubPath              string
	installPath          string // Cache for install path
	installPathInit      bool   // Whether install path has been initialized
	projectsFileOverride string // For testing: override projects file path
	lockDirOverride      string // For testing: override install lock directory
	NoCache              bool   // Skip reading from cache (still writes to cache)
	ProgressJSON         bool   // Emit Unity Hub progress as JSON Lines instead of text

	majorVersionsOnce sync.Once // Memoizes DiscoverMajorVersions per Client
	majorVersions     []string
}

type EditorInfo struct {
	Version      string
	Path         string
	Modules      []string
	Changeset    string // Changeset from Unity executable
	Architecture string // arm64, x86_64, etc.
	Manual       bool   // Whether it was manually added
}

type ReleaseInfo struct {
	Version      string
	Changeset    string
	Architecture string
}

type InstallOptions struct {
	Version      string
	Changeset    string
	Modules      []string
	Architecture string
	Force        bool // Reinstall even if the editor is already present
}

// InstallOutcome describes what an install actually did, so callers can report
// it without asking the filesystem again or guessing.
type InstallOutcome struct {
	AlreadyInstalled bool     // Another process had already installed it
	Path             string   // Path to the editor executable, when known
	Modules          []string // Modules handed to Unity Hub, with unknown ones dropped
}

// moduleFileEntry represents an entry in modules.json
type moduleFileEntry struct {
	ID          string `json:"id"`
	IsInstalled *bool  `json:"isInstalled"` // pointer to detect null vs false
}

func NewClient() *Client {
	return &Client{
		hubPath: findUnityHub(),
	}
}

func (c *Client) ListInstalledEditors() ([]EditorInfo, error) {
	// Collect editors from multiple sources
	editorMap := make(map[string]EditorInfo)

	// 1. Read from editors-v2.json (Unity Hub 3.16+)
	editors, err := c.listEditorsFromFile()
	if err == nil {
		for _, e := range editors {
			editorMap[e.Version] = e
		}
		ui.Debug("Loaded editors from editors-v2.json", "count", len(editors))
	}

	// 2. Scan default install paths
	for _, path := range c.getEditorInstallPaths() {
		scannedEditors, err := c.scanInstallPath(path)
		if err == nil {
			for _, e := range scannedEditors {
				if _, exists := editorMap[e.Version]; !exists {
					editorMap[e.Version] = e
				}
			}
			ui.Debug("Scanned install path", "path", path, "count", len(scannedEditors))
		}
	}

	// Convert map to slice
	var result []EditorInfo
	for _, e := range editorMap {
		result = append(result, e)
	}

	if len(result) > 0 {
		return result, nil
	}

	// Fallback to Unity Hub CLI
	if c.hubPath == "" {
		return nil, fmt.Errorf("unity hub not found")
	}

	ui.Debug("Falling back to Unity Hub CLI for editor list")
	cmd := exec.Command(c.hubPath, "--", "--headless", "editors", "-i")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list editors: %w", err)
	}

	return c.parseEditorsList(string(output))
}

// editorsFileData represents the structure of editors-v2.json
type editorsFileData struct {
	SchemaVersion string            `json:"schema_version"`
	Data          []editorFileEntry `json:"data"`
}

type editorFileEntry struct {
	Version      string   `json:"version"`
	Location     []string `json:"location"`
	Manual       bool     `json:"manual"`
	Architecture string   `json:"architecture"`
	ProductName  string   `json:"productName"`
}

// listEditorsFromFile reads installed editors from Unity Hub's editors-v2.json
func (c *Client) listEditorsFromFile() ([]EditorInfo, error) {
	editorsFilePath := c.getEditorsFilePath()
	if editorsFilePath == "" {
		return nil, fmt.Errorf("could not determine editors file path")
	}

	data, err := os.ReadFile(editorsFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []EditorInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read editors file: %w", err)
	}

	var editorsData editorsFileData
	if err := json.Unmarshal(data, &editorsData); err != nil {
		return nil, fmt.Errorf("failed to parse editors file: %w", err)
	}

	var result []EditorInfo
	for _, entry := range editorsData.Data {
		path := ""
		if len(entry.Location) > 0 {
			path = entry.Location[0]
		}

		result = append(result, EditorInfo{
			Version:      entry.Version,
			Path:         path,
			Architecture: entry.Architecture,
			Manual:       entry.Manual,
		})
	}

	return result, nil
}

// getEditorsFilePath returns the path to Unity Hub's editors-v2.json
func (c *Client) getEditorsFilePath() string {
	return filepath.Join(c.getUnityHubBasePath(), "editors-v2.json")
}

// getUnityHubBasePath returns the base path for Unity Hub configuration files
func (c *Client) getUnityHubBasePath() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "UnityHub")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "UnityHub")
	case "linux":
		return filepath.Join(os.Getenv("HOME"), ".config", "UnityHub")
	default:
		return ""
	}
}

// getSecondaryInstallPath reads the secondary install path from Unity Hub configuration
func (c *Client) getSecondaryInstallPath() string {
	basePath := c.getUnityHubBasePath()
	if basePath == "" {
		return ""
	}

	data, err := os.ReadFile(filepath.Join(basePath, "secondaryInstallPath.json"))
	if err != nil {
		return ""
	}

	// The file contains a quoted path, e.g., "/path/to/editors"
	var path string
	if err := json.Unmarshal(data, &path); err != nil {
		return ""
	}

	return path
}

// getEditorInstallPaths returns all paths where Unity editors might be installed
func (c *Client) getEditorInstallPaths() []string {
	var paths []string

	// Secondary install path from Unity Hub settings
	if secondaryPath := c.getSecondaryInstallPath(); secondaryPath != "" {
		paths = append(paths, secondaryPath)
	}

	// Default install paths per platform
	switch runtime.GOOS {
	case "darwin":
		paths = append(paths, "/Applications/Unity/Hub/Editor")
	case "windows":
		// Common Windows install locations
		paths = append(paths, filepath.Join(os.Getenv("ProgramFiles"), "Unity", "Hub", "Editor"))
		// Also check secondary common location
		if drive := os.Getenv("SystemDrive"); drive != "" {
			paths = append(paths, filepath.Join(drive, "Unity", "Hub", "Editor"))
		}
	case "linux":
		paths = append(paths, filepath.Join(os.Getenv("HOME"), "Unity", "Hub", "Editor"))
	}

	return paths
}

// scanInstallPath scans a directory for Unity editors
func (c *Client) scanInstallPath(installPath string) ([]EditorInfo, error) {
	if installPath == "" {
		return nil, fmt.Errorf("empty install path")
	}

	entries, err := os.ReadDir(installPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read install path: %w", err)
	}

	var result []EditorInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if this looks like a Unity version directory (e.g., 2022.3.60f1)
		version := entry.Name()
		if !isValidUnityVersion(version) {
			continue
		}

		// Check if Unity.app exists (macOS) or Unity.exe (Windows)
		var editorPath string
		switch runtime.GOOS {
		case "darwin":
			editorPath = filepath.Join(installPath, version, "Unity.app")
		case "windows":
			editorPath = filepath.Join(installPath, version, "Editor", "Unity.exe")
		case "linux":
			editorPath = filepath.Join(installPath, version, "Editor", "Unity")
		}

		if _, err := os.Stat(editorPath); err != nil {
			continue
		}

		result = append(result, EditorInfo{
			Version:      version,
			Path:         editorPath,
			Architecture: runtime.GOARCH,
		})
	}

	return result, nil
}

// isValidUnityVersion checks if a string looks like a Unity version
func isValidUnityVersion(s string) bool {
	// Unity versions look like: 2022.3.60f1, 6000.3.3f1, etc.
	// Format: YEAR.MINOR.PATCH[a|b|f|p|x]REVISION
	if len(s) < 8 {
		return false
	}

	// Check for at least 2 dots
	dotCount := 0
	for _, c := range s {
		if c == '.' {
			dotCount++
		}
	}
	if dotCount < 2 {
		return false
	}

	// Check if first character is a digit
	if s[0] < '0' || s[0] > '9' {
		return false
	}

	return true
}

func (c *Client) InstallEditor(version string, modules []string) (InstallOutcome, error) {
	return c.InstallEditorWithOptions(InstallOptions{
		Version: version,
		Modules: modules,
	})
}

func (c *Client) InstallEditorWithOptions(options InstallOptions) (InstallOutcome, error) {
	if c.hubPath == "" {
		return InstallOutcome{}, fmt.Errorf("unity hub not found")
	}

	// Resolve the architecture first: it is part of the lock identity because
	// each architecture installs into its own directory.
	architecture := options.Architecture
	if architecture == "" {
		architecture = c.detectArchitecture()
	}

	var outcome InstallOutcome
	err := c.withEditorLock(options.Version, func() error {
		var err error
		outcome, err = c.installEditorLocked(options, architecture)
		return err
	})
	return outcome, err
}

// installEditorLocked performs the install with the editor lock already held.
func (c *Client) installEditorLocked(options InstallOptions, architecture string) (InstallOutcome, error) {
	// Re-check under the lock. A run that started before another one finished
	// would otherwise reinstall the same editor, and Unity Hub would place the
	// duplicate in a `<version>-<architecture>` directory because the plain one
	// is already occupied.
	if !options.Force {
		installed, path, checkErr := c.IsEditorInstalled(options.Version)
		switch {
		case checkErr != nil:
			ui.Debug("Failed to re-check installed editors under lock", "error", checkErr)
		// Compare against what the caller actually asked for: an unspecified
		// architecture accepts the editor that is already there.
		case installed && c.ArchitectureMatches(path, options.Architecture):
			// The editor is here, but the caller may also have asked for
			// modules the other process did not install. Resolve them first so
			// that unrecognised names are merely warned about, exactly as they
			// would be on the ordinary install path.
			missing := c.mapModules(c.GetMissingModules(path, options.Modules))
			if len(missing) == 0 {
				return InstallOutcome{AlreadyInstalled: true, Path: path}, nil
			}
			addedModules, err := c.installModulesLocked(options.Version, missing)
			if err != nil {
				return InstallOutcome{}, err
			}
			return InstallOutcome{AlreadyInstalled: true, Path: path, Modules: addedModules}, nil
		}
	}

	args := []string{"--", "--headless", "install", "--version", options.Version}

	// Add changeset if provided (required for versions not in release list)
	if options.Changeset != "" {
		args = append(args, "--changeset", options.Changeset)
		ui.Debug("Using changeset", "changeset", options.Changeset)
	}

	if architecture != "" {
		args = append(args, "--architecture", architecture)
		ui.Debug("Using architecture", "arch", architecture)
	}

	// Add modules. Unknown ones are dropped by mapModules, so report back the
	// list Unity Hub was actually given.
	var installedModules []string
	if len(options.Modules) > 0 {
		installedModules = c.mapModules(options.Modules)
		if len(installedModules) > 0 {
			for _, mod := range installedModules {
				args = append(args, "--module", mod)
			}
			// Add --childModules flag to automatically install child modules (e.g., android-open-jdk)
			args = append(args, "--childModules")
		}
	}

	if err := c.executeHubCommand("Installing Unity Editor", "install Unity Editor", args); err != nil {
		return InstallOutcome{}, err
	}

	_, path, err := c.IsEditorInstalled(options.Version)
	if err != nil {
		ui.Debug("Failed to resolve installed editor path", "error", err)
	}

	// Editor discovery resolves by version alone, so with several
	// architectures of one version installed it can hand back a different
	// one's path. Report no path rather than a wrong one.
	if !c.ArchitectureMatches(path, options.Architecture) {
		ui.Debug("Installed editor path does not match the requested architecture",
			"path", path, "architecture", options.Architecture)
		path = ""
	}

	return InstallOutcome{Path: path, Modules: installedModules}, nil
}

// ArchitectureMatches reports whether the editor installed at editorPath was
// built for the requested architecture. Unity Hub installs each architecture
// into its own directory, so an arm64 install does not satisfy an explicit
// x86_64 request.
//
// An empty request matches anything: the caller did not ask for a specific
// architecture, so a working editor is what they wanted. An editor whose
// architecture Unity Hub did not record also matches, rather than triggering a
// multi-gigabyte reinstall on a guess.
func (c *Client) ArchitectureMatches(editorPath, architecture string) bool {
	if architecture == "" {
		return true
	}

	installed := c.recordedArchitecture(editorPath)
	return installed == "" || installed == architecture
}

// recordedArchitecture reads the architecture from the metadata.hub.json that
// Unity Hub writes in the editor's install directory. It works off the
// discovered editor path rather than the configured install root, because an
// editor can be found through editors-v2.json, a secondary install path, or a
// `<version>-<architecture>` directory.
func (c *Client) recordedArchitecture(editorPath string) string {
	root := editorInstallRoot(editorPath)
	if root == "" {
		return ""
	}

	data, err := os.ReadFile(filepath.Join(root, "metadata.hub.json"))
	if err != nil {
		return ""
	}

	var metadata struct {
		Architecture string `json:"architecture"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		ui.Debug("Failed to parse editor metadata", "path", root, "error", err)
		return ""
	}

	return metadata.Architecture
}

// editorInstallRoot returns the directory that holds metadata.hub.json for a
// discovered editor path, or "" when the shape is not recognised.
//
// The path can be the install directory itself (editors-v2.json records it that
// way), `<root>/Unity.app` on macOS, or `<root>/Editor/Unity[.exe]` elsewhere.
// The shape is matched explicitly rather than walking up blindly, because an
// unrelated metadata.hub.json in a shared parent directory would otherwise be
// read as this editor's.
func editorInstallRoot(editorPath string) string {
	if editorPath == "" {
		return ""
	}

	switch filepath.Base(editorPath) {
	case "Unity.app":
		return filepath.Dir(editorPath)
	case "Unity", "Unity.exe":
		parent := filepath.Dir(editorPath)
		if filepath.Base(parent) == "Editor" {
			return filepath.Dir(parent)
		}
		return parent
	default:
		// Recorded as the install directory itself.
		return editorPath
	}
}

func (c *Client) detectArchitecture() string {
	// Auto-detect architecture based on current system
	switch runtime.GOOS {
	case "darwin":
		// Check if running on Apple Silicon
		cmd := exec.Command("uname", "-m")
		output, err := cmd.Output()
		if err == nil {
			arch := strings.TrimSpace(string(output))
			if arch == "arm64" {
				return "arm64"
			}
			return "x86_64"
		}
	case "windows", "linux":
		// Check system architecture
		if runtime.GOARCH == "arm64" {
			return "arm64"
		}
		return "x86_64"
	}

	return ""
}

// IsEditorInstalled checks if a Unity Editor version is installed
// Returns: installed (bool), path (string), error
func (c *Client) IsEditorInstalled(version string) (bool, string, error) {
	// First, try quick directory check
	installPath, err := c.GetInstallPath()
	if err == nil && installPath != "" {
		editorPath := filepath.Join(installPath, version)
		if fileExists(editorPath) {
			ui.Debug("Found Unity Editor via directory check", "version", version, "path", editorPath)

			// Get full executable path
			var execPath string
			switch runtime.GOOS {
			case "darwin":
				execPath = filepath.Join(editorPath, "Unity.app")
			case "windows":
				execPath = filepath.Join(editorPath, "Editor", "Unity.exe")
			case "linux":
				execPath = filepath.Join(editorPath, "Editor", "Unity")
			}

			if fileExists(execPath) {
				return true, execPath, nil
			}
		}
	}

	// Fallback to Unity Hub query if directory check fails
	editors, err := c.ListInstalledEditors()
	if err != nil {
		return false, "", err
	}

	for _, editor := range editors {
		if editor.Version == version {
			return true, editor.Path, nil
		}
	}

	return false, "", nil
}

// GetEditorChangeset retrieves the changeset for an installed Unity Editor
// First tries to read from version.txt file, then falls back to running Unity -version
func (c *Client) GetEditorChangeset(editorPath string) string {
	versionFilePath := getVersionFilePath(runtime.GOOS, editorPath)

	// Try to read version.txt file
	if fileExists(versionFilePath) {
		changeset := c.readChangesetFromVersionFile(versionFilePath)
		if changeset != "" {
			ui.Debug("Found changeset from version.txt", "changeset", changeset)
			return changeset
		}
	}

	unityExec := getEditorExecutablePath(runtime.GOOS, editorPath)

	if !fileExists(unityExec) {
		ui.Debug("Unity executable not found", "path", unityExec)
		return ""
	}

	cmd := exec.Command(unityExec, "-version")
	output, err := cmd.Output()
	if err != nil {
		ui.Debug("Failed to get Unity version", "error", err)
		return ""
	}

	// Parse output like "2022.3.59f1 (630718f645a5)"
	versionStr := strings.TrimSpace(string(output))
	if idx := strings.Index(versionStr, "("); idx > 0 {
		if idx2 := strings.Index(versionStr, ")"); idx2 > idx {
			changeset := strings.TrimSpace(versionStr[idx+1 : idx2])
			ui.Debug("Found changeset from Unity executable", "changeset", changeset)
			return changeset
		}
	}

	return ""
}

// readChangesetFromVersionFile reads changeset from Unity's version.txt file
func (c *Client) readChangesetFromVersionFile(filepath string) string {
	data, err := os.ReadFile(filepath)
	if err != nil {
		ui.Debug("Failed to read version.txt", "error", err)
		return ""
	}

	// version.txt format example:
	// 2022.3.20f1 (f3a49e6e3c6e)
	// Windows/Mac/Linux x64 Unity Editor
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		// Extract changeset from parentheses
		if idx := strings.Index(firstLine, "("); idx > 0 {
			if idx2 := strings.Index(firstLine, ")"); idx2 > idx {
				return strings.TrimSpace(firstLine[idx+1 : idx2])
			}
		}
	}

	return ""
}

func (c *Client) GetInstallPath() (string, error) {
	// Return cached value if already initialized
	if c.installPathInit {
		if c.installPath == "" {
			return "", fmt.Errorf("unity hub install path not available")
		}
		return c.installPath, nil
	}

	// Initialize install path (only once)
	c.installPathInit = true

	// Try to load from file cache first
	if cachedPath := c.loadInstallPathCache(); cachedPath != "" {
		if fileExists(cachedPath) {
			ui.Debug("Found Unity install path from cache", "path", cachedPath)
			c.installPath = cachedPath
			return cachedPath, nil
		}
		// Cache is stale, will update it later
		ui.Debug("Cached install path no longer exists, will update cache", "", "")
	}

	// Try common default paths before calling Unity Hub
	defaultPaths := c.getDefaultInstallPaths()
	for _, path := range defaultPaths {
		if fileExists(path) {
			ui.Debug("Found Unity install path via default location", "path", path)
			c.installPath = path
			c.saveInstallPathCache(path) // Save to cache
			return path, nil
		}
	}

	// If defaults don't work, query Unity Hub
	if c.hubPath == "" {
		return "", fmt.Errorf("unity hub not found")
	}

	ui.Debug("Querying Unity Hub for install path")
	cmd := exec.Command(c.hubPath, "--", "--headless", "install-path", "--get")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get install path: %w", err)
	}

	c.installPath = strings.TrimSpace(string(output))
	if c.installPath != "" {
		c.saveInstallPathCache(c.installPath) // Save to cache
	}
	return c.installPath, nil
}

// Cache file structure
type installPathCacheData struct {
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
}

// Get cache file path
func (c *Client) getCacheFilePath() string {
	tmpDir := os.TempDir()
	return filepath.Join(tmpDir, "uniforge-install-path.json")
}

// Load install path from cache file
func (c *Client) loadInstallPathCache() string {
	cacheFile := c.getCacheFilePath()

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if !os.IsNotExist(err) {
			ui.Debug("Failed to read cache file", "error", err)
		}
		return ""
	}

	var cache installPathCacheData
	if err := json.Unmarshal(data, &cache); err != nil {
		ui.Debug("Failed to parse cache file", "error", err)
		return ""
	}

	// Check if cache is not too old (24 hours)
	if time.Since(cache.Timestamp) > 24*time.Hour {
		ui.Debug("Cache is older than 24 hours, ignoring")
		return ""
	}

	return cache.Path
}

// Save install path to cache file
func (c *Client) saveInstallPathCache(path string) {
	cacheFile := c.getCacheFilePath()

	cache := installPathCacheData{
		Path:      path,
		Timestamp: time.Now(),
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		ui.Debug("Failed to marshal cache data", "error", err)
		return
	}

	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		ui.Debug("Failed to write cache file", "error", err)
		return
	}

	ui.Debug("Saved install path to cache", "path", cacheFile)
}

func (c *Client) getDefaultInstallPaths() []string {
	var paths []string

	// Check for custom install path from environment variable
	// For users who installed Unity Editors in a custom location (e.g., external SSD)
	if customPath := os.Getenv("UNIFORGE_EDITOR_BASE_PATH"); customPath != "" {
		paths = append(paths, customPath)
	}

	switch runtime.GOOS {
	case "darwin":
		paths = append(paths,
			"/Applications/Unity/Hub/Editor",
			filepath.Join(os.Getenv("HOME"), "Applications", "Unity", "Hub", "Editor"),
		)
	case "windows":
		programFiles := os.Getenv("PROGRAMFILES")
		paths = append(paths,
			filepath.Join(programFiles, "Unity", "Hub", "Editor"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Unity", "Hub", "Editor"),
		)
	case "linux":
		paths = append(paths,
			"/opt/Unity/Hub/Editor",
			filepath.Join(os.Getenv("HOME"), "Unity", "Hub", "Editor"),
		)
	}

	return paths
}

func (c *Client) ListAvailableReleases() ([]ReleaseInfo, error) {
	if c.hubPath == "" {
		return nil, fmt.Errorf("unity hub not found")
	}

	cmd := exec.Command(c.hubPath, "--", "--headless", "editors", "-r")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	return c.parseReleasesList(string(output))
}

func (c *Client) parseReleasesList(output string) ([]ReleaseInfo, error) {
	var releases []ReleaseInfo

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse format: "2022.3.10f1 (Apple Silicon)" or just "2022.3.10f1"
		release := ReleaseInfo{}

		// Check for architecture in parentheses
		if idx := strings.Index(line, "("); idx > 0 {
			release.Version = strings.TrimSpace(line[:idx])
			arch := strings.TrimSpace(line[idx+1:])
			if idx2 := strings.Index(arch, ")"); idx2 > 0 {
				release.Architecture = arch[:idx2]
			}
		} else {
			// Just version, no architecture info
			parts := strings.Fields(line)
			if len(parts) > 0 {
				release.Version = parts[0]
			}
		}

		if release.Version != "" {
			releases = append(releases, release)
		}
	}

	return releases, nil
}

func (c *Client) parseEditorsList(output string) ([]EditorInfo, error) {
	var editors []EditorInfo

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Look for "installed at" pattern to extract path correctly
		if strings.Contains(line, "installed at") {
			parts := strings.Split(line, "installed at")
			if len(parts) == 2 {
				versionPart := strings.TrimSpace(strings.Split(parts[0], ",")[0])
				// Remove architecture info like "(Apple シリコン)" or "(Apple Silicon)"
				if idx := strings.Index(versionPart, "("); idx > 0 {
					versionPart = strings.TrimSpace(versionPart[:idx])
				}
				path := strings.TrimSpace(parts[1])
				editors = append(editors, EditorInfo{
					Version: versionPart,
					Path:    path,
				})
			}
		} else {
			// Fallback to original parsing for other formats
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				editors = append(editors, EditorInfo{
					Version: parts[0],
					Path:    parts[len(parts)-1],
				})
			}
		}
	}

	return editors, nil
}

// moduleMap maps user-friendly module names to Unity Hub CLI module IDs
var moduleMap = map[string]string{
	"android":        "android",
	"ios":            "ios",
	"webgl":          "webgl",
	"windows":        "windows-il2cpp",
	"linux":          "linux-il2cpp",
	"mac":            "mac-il2cpp",
	"documentation":  "documentation",
	"standardassets": "standardassets",
	"example":        "example",
}

// modulePathMap maps Unity Hub CLI module IDs to PlaybackEngines directory names
var modulePathMap = map[string]string{
	"android":        "AndroidPlayer",
	"ios":            "iOSSupport",
	"webgl":          "WebGLSupport",
	"windows-il2cpp": "WindowsStandaloneSupport",
	"linux-il2cpp":   "LinuxStandaloneSupport",
	"mac-il2cpp":     "MacStandaloneSupport",
}

// canonicalModuleIDs holds the Unity Hub module ids that moduleMap resolves to.
// The install TUI reads module ids straight from the release metadata and
// passes them through, so those must be accepted as-is.
var canonicalModuleIDs = func() map[string]bool {
	ids := make(map[string]bool, len(moduleMap))
	for _, id := range moduleMap {
		ids[id] = true
	}
	return ids
}()

// resolveModuleID converts a friendly alias or an already-canonical Unity Hub
// module id into the canonical id, case-insensitively. It returns "" when the
// module is neither.
func resolveModuleID(module string) string {
	lowered := strings.ToLower(module)
	if mapped, ok := moduleMap[lowered]; ok {
		return mapped
	}
	if canonicalModuleIDs[lowered] {
		return lowered
	}
	return ""
}

func (c *Client) mapModules(modules []string) []string {
	var mapped []string
	for _, module := range modules {
		if id := resolveModuleID(module); id != "" {
			mapped = append(mapped, id)
		} else {
			ui.Warn("Unknown module: %s", module)
		}
	}

	return mapped
}

// GetPlaybackEnginesPath returns the PlaybackEngines directory path for an editor
func (c *Client) GetPlaybackEnginesPath(editorPath string) string {
	return getPlaybackEnginesPath(runtime.GOOS, editorPath)
}

func getPlaybackEnginesPath(goos, editorPath string) string {
	baseDir := normalizeEditorBasePath(goos, editorPath)

	switch goos {
	case "darwin":
		return filepath.Join(baseDir, "PlaybackEngines")
	case "windows":
		return filepath.Join(baseDir, "Editor", "Data", "PlaybackEngines")
	case "linux":
		return filepath.Join(baseDir, "Editor", "Data", "PlaybackEngines")
	}
	return ""
}

// getModulesFilePath returns the path to modules.json for a given editor
func (c *Client) getModulesFilePath(editorPath string) string {
	return getModulesFilePath(runtime.GOOS, editorPath)
}

func normalizeEditorBasePath(goos, editorPath string) string {
	editorPath = cleanPathForOS(goos, editorPath)

	switch goos {
	case "darwin":
		executableSuffix := joinPathForOS(goos, "Unity.app", "Contents", "MacOS", "Unity")
		if strings.HasSuffix(editorPath, executableSuffix) {
			return cleanPathForOS(goos, strings.TrimSuffix(editorPath, executableSuffix))
		}
		if strings.HasSuffix(editorPath, ".app") {
			return dirPathForOS(goos, editorPath)
		}
	case "windows":
		if strings.EqualFold(filepath.Ext(editorPath), ".exe") {
			return dirPathForOS(goos, dirPathForOS(goos, editorPath))
		}
	case "linux":
		if strings.HasSuffix(editorPath, joinPathForOS(goos, "Editor", "Unity")) {
			return dirPathForOS(goos, dirPathForOS(goos, editorPath))
		}
	}

	return editorPath
}

func getEditorExecutablePath(goos, editorPath string) string {
	baseDir := normalizeEditorBasePath(goos, editorPath)

	switch goos {
	case "darwin":
		return joinPathForOS(goos, baseDir, "Unity.app", "Contents", "MacOS", "Unity")
	case "windows":
		return joinPathForOS(goos, baseDir, "Editor", "Unity.exe")
	case "linux":
		return joinPathForOS(goos, baseDir, "Editor", "Unity")
	default:
		return joinPathForOS(goos, baseDir, "Unity")
	}
}

func getVersionFilePath(goos, editorPath string) string {
	baseDir := normalizeEditorBasePath(goos, editorPath)

	switch goos {
	case "darwin":
		return joinPathForOS(goos, baseDir, "Unity.app", "Contents", "Resources", "version.txt")
	case "windows", "linux":
		return joinPathForOS(goos, baseDir, "Editor", "Data", "Resources", "version.txt")
	default:
		return ""
	}
}

func getModulesFilePath(goos, editorPath string) string {
	baseDir := normalizeEditorBasePath(goos, editorPath)

	switch goos {
	case "darwin", "windows", "linux":
		return joinPathForOS(goos, baseDir, "modules.json")
	default:
		return ""
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

func dirPathForOS(goos, value string) string {
	if goos == "windows" {
		return filepath.Dir(value)
	}

	return pathpkg.Dir(strings.ReplaceAll(value, "\\", "/"))
}

// readModulesFile reads and parses modules.json
func (c *Client) readModulesFile(editorPath string) ([]moduleFileEntry, error) {
	modulesFilePath := c.getModulesFilePath(editorPath)
	if modulesFilePath == "" {
		return nil, fmt.Errorf("could not determine modules file path")
	}

	data, err := os.ReadFile(modulesFilePath)
	if err != nil {
		return nil, err
	}

	var modules []moduleFileEntry
	if err := json.Unmarshal(data, &modules); err != nil {
		return nil, err
	}

	return modules, nil
}

// IsModuleInstalled checks if a specific module is installed for an editor
func (c *Client) IsModuleInstalled(editorPath string, module string) bool {
	// Map a friendly name or a differently-cased canonical id to the canonical
	// Hub CLI module ID first, so the comparisons below line up.
	moduleID := module
	if id := resolveModuleID(module); id != "" {
		moduleID = id
	}

	// Try to read from modules.json first
	modules, err := c.readModulesFile(editorPath)
	if err == nil {
		for _, m := range modules {
			if m.ID == moduleID {
				// If isInstalled is explicitly set, use that value
				if m.IsInstalled != nil {
					ui.Debug("Module check from modules.json", "module", module, "id", moduleID, "installed", *m.IsInstalled)
					return *m.IsInstalled
				}
				// isInstalled is null, fall through to directory check
				ui.Debug("Module isInstalled is null, checking directory", "module", module, "id", moduleID)
				break
			}
		}
	}

	// Fallback to directory check
	dirName, ok := modulePathMap[moduleID]
	if !ok {
		ui.Debug("Unknown module for path check", "module", module)
		return false
	}

	playbackEnginesPath := c.GetPlaybackEnginesPath(editorPath)
	modulePath := filepath.Join(playbackEnginesPath, dirName)

	exists := fileExists(modulePath)
	ui.Debug("Module check by directory", "module", module, "path", modulePath, "exists", exists)
	return exists
}

// GetMissingModules returns a list of modules that are not installed
func (c *Client) GetMissingModules(editorPath string, modules []string) []string {
	var missing []string
	for _, module := range modules {
		if !c.IsModuleInstalled(editorPath, module) {
			missing = append(missing, module)
		}
	}
	return missing
}

// InstallModules installs additional modules to an existing editor.
// It takes the same lock as installing the editor, because Unity Hub downloads
// into and writes into the same editor directory.
// It returns the modules Unity Hub was actually given: unknown ones are
// dropped, and callers must not report those as installed.
func (c *Client) InstallModules(version string, modules []string) ([]string, error) {
	if c.hubPath == "" {
		return nil, fmt.Errorf("unity hub not found")
	}

	if len(modules) == 0 {
		return nil, nil
	}

	moduleList := c.mapModules(modules)
	if len(moduleList) == 0 {
		return nil, fmt.Errorf("none of the requested modules are known: %s", strings.Join(modules, ", "))
	}

	var installed []string
	err := c.withEditorLock(version, func() error {
		var err error
		installed, err = c.installModulesLocked(version, moduleList)
		return err
	})
	return installed, err
}

// installModulesLocked installs already-resolved canonical module ids with the
// editor lock held.
func (c *Client) installModulesLocked(version string, moduleList []string) ([]string, error) {
	args := []string{"--", "--headless", "install-modules", "--version", version}
	for _, mod := range moduleList {
		args = append(args, "--module", mod)
	}

	// Add --childModules flag to automatically install child modules (e.g., android-open-jdk)
	args = append(args, "--childModules")

	if err := c.executeHubCommand("Installing modules", "install modules", args); err != nil {
		return nil, err
	}
	return moduleList, nil
}

// executeHubCommand runs a Unity Hub CLI command with the given arguments
func (c *Client) executeHubCommand(debugMsg, operation string, args []string) error {
	ui.Debug(debugMsg, "command", c.hubPath, "args", strings.Join(args, " "))

	// Create context that cancels on SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	cmd := exec.CommandContext(ctx, c.hubPath, args...)

	// Unity Hub redraws its progress block with cursor-control sequences even
	// when stdout is a pipe. On a non-TTY that yields an unreadable log and,
	// worse, no output at all until exit when piped through tail or grep, so
	// callers cannot tell a running install from a hung one.
	if ui.IsTTY() && !c.ProgressJSON {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		stdoutFilter := newProgressFilter(os.Stdout, c.ProgressJSON)
		// stderr carries diagnostics for humans, so it stays plain text even
		// when stdout is a machine-readable stream.
		stderrFilter := newProgressFilter(os.Stderr, false)
		defer func() {
			for _, filter := range []*progressFilter{stdoutFilter, stderrFilter} {
				if err := filter.Close(); err != nil {
					ui.Debug("Failed to flush progress filter", "error", err)
				}
			}
		}()
		cmd.Stdout = stdoutFilter
		cmd.Stderr = stderrFilter
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", operation, err)
	}

	// Wait for either command completion or signal
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to %s: %w", operation, err)
		}
		return nil
	case sig := <-sigChan:
		// stderr: stdout may be carrying machine-readable output.
		ui.Note("\nReceived %s, stopping Unity Hub...", sig)
		cancel() // This will send SIGKILL to the process
		<-done   // Wait for process to exit
		return fmt.Errorf("interrupted by %s", sig)
	}
}

// hubInfoData represents the structure of hubInfo.json
type hubInfoData struct {
	Version        string `json:"version"`
	ExecutablePath string `json:"executablePath"`
}

func findUnityHub() string {
	// 1. Check environment variable first
	envPath := os.Getenv("UNIFORGE_HUB_PATH")
	if envPath != "" && fileExists(envPath) {
		return envPath
	}

	// 2. Try to read from hubInfo.json
	if path := getHubPathFromHubInfo(); path != "" {
		return path
	}

	// 3. Try default paths
	paths := getUnityHubPaths()
	for _, path := range paths {
		if fileExists(path) {
			ui.Debug("Found Unity Hub", "path", path)
			return path
		}
	}

	// 4. Try PATH lookup
	pathCmd, err := exec.LookPath("Unity Hub")
	if err == nil {
		return pathCmd
	}

	ui.Warn("Unity Hub not found. Please install Unity Hub or set UNIFORGE_HUB_PATH environment variable")
	return ""
}

// getHubPathFromHubInfo reads the Unity Hub executable path from hubInfo.json
func getHubPathFromHubInfo() string {
	var basePath string
	switch runtime.GOOS {
	case "darwin":
		basePath = filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "UnityHub")
	case "windows":
		basePath = filepath.Join(os.Getenv("APPDATA"), "UnityHub")
	case "linux":
		basePath = filepath.Join(os.Getenv("HOME"), ".config", "UnityHub")
	default:
		return ""
	}

	hubInfoPath := filepath.Join(basePath, "hubInfo.json")
	data, err := os.ReadFile(hubInfoPath)
	if err != nil {
		return ""
	}

	var hubInfo hubInfoData
	if err := json.Unmarshal(data, &hubInfo); err != nil {
		return ""
	}

	if hubInfo.ExecutablePath != "" && fileExists(hubInfo.ExecutablePath) {
		ui.Debug("Found Unity Hub from hubInfo.json", "path", hubInfo.ExecutablePath)
		return hubInfo.ExecutablePath
	}

	return ""
}

func getUnityHubPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Unity Hub.app/Contents/MacOS/Unity Hub",
			filepath.Join(os.Getenv("HOME"), "Applications", "Unity Hub.app", "Contents", "MacOS", "Unity Hub"),
		}
	case "windows":
		programFiles := os.Getenv("PROGRAMFILES")
		return []string{
			filepath.Join(programFiles, "Unity Hub", "Unity Hub.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Unity Hub", "Unity Hub.exe"),
		}
	case "linux":
		return []string{
			"/opt/Unity Hub/Unity Hub",
			filepath.Join(os.Getenv("HOME"), "Unity Hub", "Unity Hub"),
			"/usr/bin/unity-hub",
		}
	default:
		return []string{}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
