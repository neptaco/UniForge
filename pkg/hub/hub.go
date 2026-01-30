package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/neptaco/uniforge/pkg/ui"
)

type Client struct {
	hubPath              string
	installPath          string // Cache for install path
	installPathInit      bool   // Whether install path has been initialized
	projectsFileOverride string // For testing: override projects file path
	NoCache              bool   // Skip reading from cache (still writes to cache)
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
			paths = append(paths, filepath.Join(drive, "\\Unity", "Hub", "Editor"))
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

func (c *Client) InstallEditor(version string, modules []string) error {
	return c.InstallEditorWithOptions(InstallOptions{
		Version: version,
		Modules: modules,
	})
}

func (c *Client) InstallEditorWithOptions(options InstallOptions) error {
	if c.hubPath == "" {
		return fmt.Errorf("unity hub not found")
	}

	args := []string{"--", "--headless", "install", "--version", options.Version}

	// Add changeset if provided (required for versions not in release list)
	if options.Changeset != "" {
		args = append(args, "--changeset", options.Changeset)
		ui.Debug("Using changeset", "changeset", options.Changeset)
	}

	// Add architecture if specified, otherwise auto-detect
	architecture := options.Architecture
	if architecture == "" {
		architecture = c.detectArchitecture()
	}
	if architecture != "" {
		args = append(args, "--architecture", architecture)
		ui.Debug("Using architecture", "arch", architecture)
	}

	// Add modules
	if len(options.Modules) > 0 {
		moduleList := c.mapModules(options.Modules)
		if len(moduleList) > 0 {
			args = append(args, "--module")
			args = append(args, moduleList...)
			// Add --childModules flag to automatically install child modules (e.g., android-open-jdk)
			args = append(args, "--childModules")
		}
	}

	return c.executeHubCommand("Installing Unity Editor", "install Unity Editor", args)
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
	// First, try to read from version.txt file (fastest method)
	var versionFilePath string
	switch runtime.GOOS {
	case "darwin":
		if strings.HasSuffix(editorPath, ".app") {
			versionFilePath = filepath.Join(editorPath, "Contents", "Resources", "version.txt")
		} else {
			versionFilePath = filepath.Join(editorPath, "Unity.app", "Contents", "Resources", "version.txt")
		}
	case "windows":
		// Windows: C:\Program Files\Unity\Hub\Editor\2022.3.20f1\Editor\Data\Resources\version.txt
		if strings.HasSuffix(editorPath, ".exe") {
			// If it's already pointing to Unity.exe, go up to find Data folder
			versionFilePath = filepath.Join(filepath.Dir(editorPath), "Data", "Resources", "version.txt")
		} else {
			versionFilePath = filepath.Join(editorPath, "Editor", "Data", "Resources", "version.txt")
		}
	case "linux":
		versionFilePath = filepath.Join(editorPath, "Editor", "Data", "Resources", "version.txt")
	}

	// Try to read version.txt file
	if fileExists(versionFilePath) {
		changeset := c.readChangesetFromVersionFile(versionFilePath)
		if changeset != "" {
			ui.Debug("Found changeset from version.txt", "changeset", changeset)
			return changeset
		}
	}

	// Fallback to running Unity -version
	var unityExec string
	switch runtime.GOOS {
	case "darwin":
		if strings.HasSuffix(editorPath, ".app") {
			unityExec = filepath.Join(editorPath, "Contents", "MacOS", "Unity")
		} else {
			unityExec = filepath.Join(editorPath, "Unity.app", "Contents", "MacOS", "Unity")
		}
	case "windows":
		if strings.HasSuffix(editorPath, ".exe") {
			unityExec = editorPath
		} else {
			unityExec = filepath.Join(editorPath, "Editor", "Unity.exe")
		}
	case "linux":
		unityExec = filepath.Join(editorPath, "Editor", "Unity")
	}

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

func (c *Client) mapModules(modules []string) []string {
	var mapped []string
	for _, module := range modules {
		if mappedModule, ok := moduleMap[strings.ToLower(module)]; ok {
			mapped = append(mapped, mappedModule)
		} else {
			ui.Warn("Unknown module: %s", module)
		}
	}

	return mapped
}

// GetPlaybackEnginesPath returns the PlaybackEngines directory path for an editor
func (c *Client) GetPlaybackEnginesPath(editorPath string) string {
	switch runtime.GOOS {
	case "darwin":
		// macOS: PlaybackEngines is at the same level as Unity.app
		// e.g., /Applications/Unity/Hub/Editor/2022.3.60f1/PlaybackEngines/
		baseDir := editorPath
		if strings.HasSuffix(editorPath, ".app") {
			baseDir = filepath.Dir(editorPath)
		}
		return filepath.Join(baseDir, "PlaybackEngines")
	case "windows":
		// Windows: C:\Program Files\Unity\Hub\Editor\2022.3.60f1\Editor\Data\PlaybackEngines
		if strings.HasSuffix(editorPath, ".exe") {
			// editorPath is Unity.exe, go up to Editor, then Data/PlaybackEngines
			return filepath.Join(filepath.Dir(editorPath), "Data", "PlaybackEngines")
		}
		// editorPath is the version directory
		return filepath.Join(editorPath, "Editor", "Data", "PlaybackEngines")
	case "linux":
		// Linux: similar to Windows
		return filepath.Join(editorPath, "Editor", "Data", "PlaybackEngines")
	}
	return ""
}

// getModulesFilePath returns the path to modules.json for a given editor
func (c *Client) getModulesFilePath(editorPath string) string {
	switch runtime.GOOS {
	case "darwin":
		// macOS: modules.json is at the same level as Unity.app
		baseDir := editorPath
		if strings.HasSuffix(editorPath, ".app") {
			baseDir = filepath.Dir(editorPath)
		}
		return filepath.Join(baseDir, "modules.json")
	case "windows":
		// Windows: C:\Program Files\Unity\Hub\Editor\2022.3.60f1\modules.json
		if strings.HasSuffix(editorPath, ".exe") {
			// editorPath is Unity.exe, go up to version directory
			return filepath.Join(filepath.Dir(filepath.Dir(editorPath)), "modules.json")
		}
		return filepath.Join(editorPath, "modules.json")
	case "linux":
		// Linux: similar structure
		return filepath.Join(editorPath, "modules.json")
	}
	return ""
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
	// Map user-friendly name to Hub CLI module ID first
	moduleID := module
	if mapped, ok := moduleMap[strings.ToLower(module)]; ok {
		moduleID = mapped
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

// InstallModules installs additional modules to an existing editor
func (c *Client) InstallModules(version string, modules []string) error {
	if c.hubPath == "" {
		return fmt.Errorf("unity hub not found")
	}

	if len(modules) == 0 {
		return nil
	}

	args := []string{"--", "--headless", "install-modules", "--version", version}

	moduleList := c.mapModules(modules)
	if len(moduleList) > 0 {
		args = append(args, "--module")
		args = append(args, moduleList...)
	}

	// Add --childModules flag to automatically install child modules (e.g., android-open-jdk)
	args = append(args, "--childModules")

	return c.executeHubCommand("Installing modules", "install modules", args)
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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

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
		ui.Muted("\nReceived %s, stopping Unity Hub...", sig)
		cancel() // This will send SIGKILL to the process
		<-done   // Wait for process to exit
		return fmt.Errorf("interrupted by %s", sig)
	}
}

func findUnityHub() string {
	envPath := os.Getenv("UNIFORGE_HUB_PATH")
	if envPath != "" && fileExists(envPath) {
		return envPath
	}

	paths := getUnityHubPaths()
	for _, path := range paths {
		if fileExists(path) {
			ui.Debug("Found Unity Hub", "path", path)
			return path
		}
	}

	pathCmd, err := exec.LookPath("Unity Hub")
	if err == nil {
		return pathCmd
	}

	ui.Warn("Unity Hub not found. Please install Unity Hub or set UNIFORGE_HUB_PATH environment variable")
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
