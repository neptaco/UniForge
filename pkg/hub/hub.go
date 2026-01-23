package hub

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type Client struct {
	hubPath         string
	installPath     string // Cache for install path
	installPathInit bool   // Whether install path has been initialized
}

type EditorInfo struct {
	Version   string
	Path      string
	Modules   []string
	Changeset string // Changeset from Unity executable
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

func NewClient() *Client {
	return &Client{
		hubPath: findUnityHub(),
	}
}

func (c *Client) ListInstalledEditors() ([]EditorInfo, error) {
	if c.hubPath == "" {
		return nil, fmt.Errorf("Unity Hub not found")
	}

	cmd := exec.Command(c.hubPath, "--", "--headless", "editors", "-i")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list editors: %w", err)
	}

	return c.parseEditorsList(string(output))
}

func (c *Client) InstallEditor(version string, modules []string) error {
	return c.InstallEditorWithOptions(InstallOptions{
		Version: version,
		Modules: modules,
	})
}

func (c *Client) InstallEditorWithOptions(options InstallOptions) error {
	if c.hubPath == "" {
		return fmt.Errorf("Unity Hub not found")
	}

	args := []string{"--", "--headless", "install", "--version", options.Version}

	// Add changeset if provided (required for versions not in release list)
	if options.Changeset != "" {
		args = append(args, "--changeset", options.Changeset)
		logrus.Debugf("Using changeset: %s", options.Changeset)
	}

	// Add architecture if specified, otherwise auto-detect
	architecture := options.Architecture
	if architecture == "" {
		architecture = c.detectArchitecture()
	}
	if architecture != "" {
		args = append(args, "--architecture", architecture)
		logrus.Debugf("Using architecture: %s", architecture)
	}

	// Add modules
	if len(options.Modules) > 0 {
		moduleList := c.mapModules(options.Modules)
		if len(moduleList) > 0 {
			args = append(args, "--module")
			args = append(args, moduleList...)
		}
	}

	logrus.Debugf("Installing Unity Editor with command: %s %s", c.hubPath, strings.Join(args, " "))

	cmd := exec.Command(c.hubPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install Unity Editor: %w", err)
	}

	return nil
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
			logrus.Debugf("Found Unity Editor %s via directory check at: %s", version, editorPath)

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
			logrus.Debugf("Found changeset from version.txt: %s", changeset)
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
		logrus.Debugf("Unity executable not found at: %s", unityExec)
		return ""
	}

	cmd := exec.Command(unityExec, "-version")
	output, err := cmd.Output()
	if err != nil {
		logrus.Debugf("Failed to get Unity version: %v", err)
		return ""
	}

	// Parse output like "2022.3.59f1 (630718f645a5)"
	versionStr := strings.TrimSpace(string(output))
	if idx := strings.Index(versionStr, "("); idx > 0 {
		if idx2 := strings.Index(versionStr, ")"); idx2 > idx {
			changeset := strings.TrimSpace(versionStr[idx+1 : idx2])
			logrus.Debugf("Found changeset from Unity executable: %s", changeset)
			return changeset
		}
	}

	return ""
}

// readChangesetFromVersionFile reads changeset from Unity's version.txt file
func (c *Client) readChangesetFromVersionFile(filepath string) string {
	data, err := os.ReadFile(filepath)
	if err != nil {
		logrus.Debugf("Failed to read version.txt: %v", err)
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
			return "", fmt.Errorf("Unity Hub install path not available")
		}
		return c.installPath, nil
	}

	// Initialize install path (only once)
	c.installPathInit = true

	// Try to load from file cache first
	if cachedPath := c.loadInstallPathCache(); cachedPath != "" {
		if fileExists(cachedPath) {
			logrus.Debugf("Found Unity install path from cache: %s", cachedPath)
			c.installPath = cachedPath
			return cachedPath, nil
		}
		// Cache is stale, will update it later
		logrus.Debug("Cached install path no longer exists, will update cache")
	}

	// Try common default paths before calling Unity Hub
	defaultPaths := c.getDefaultInstallPaths()
	for _, path := range defaultPaths {
		if fileExists(path) {
			logrus.Debugf("Found Unity install path via default location: %s", path)
			c.installPath = path
			c.saveInstallPathCache(path) // Save to cache
			return path, nil
		}
	}

	// If defaults don't work, query Unity Hub
	if c.hubPath == "" {
		return "", fmt.Errorf("Unity Hub not found")
	}

	logrus.Debug("Querying Unity Hub for install path...")
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
	return filepath.Join(tmpDir, "unity-cli-install-path.json")
}

// Load install path from cache file
func (c *Client) loadInstallPathCache() string {
	cacheFile := c.getCacheFilePath()

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if !os.IsNotExist(err) {
			logrus.Debugf("Failed to read cache file: %v", err)
		}
		return ""
	}

	var cache installPathCacheData
	if err := json.Unmarshal(data, &cache); err != nil {
		logrus.Debugf("Failed to parse cache file: %v", err)
		return ""
	}

	// Check if cache is not too old (24 hours)
	if time.Since(cache.Timestamp) > 24*time.Hour {
		logrus.Debug("Cache is older than 24 hours, ignoring")
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
		logrus.Debugf("Failed to marshal cache data: %v", err)
		return
	}

	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		logrus.Debugf("Failed to write cache file: %v", err)
		return
	}

	logrus.Debugf("Saved install path to cache: %s", cacheFile)
}

func (c *Client) getDefaultInstallPaths() []string {
	var paths []string

	// Check for custom install path from environment variable
	if customPath := os.Getenv("UNITY_HUB_INSTALL_PATH"); customPath != "" {
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
		return nil, fmt.Errorf("Unity Hub not found")
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

func (c *Client) mapModules(modules []string) []string {
	moduleMap := map[string]string{
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

	var mapped []string
	for _, module := range modules {
		if mappedModule, ok := moduleMap[strings.ToLower(module)]; ok {
			mapped = append(mapped, mappedModule)
		} else {
			logrus.Warnf("Unknown module: %s", module)
		}
	}

	return mapped
}

func findUnityHub() string {
	envPath := os.Getenv("UNITY_CLI_HUB_PATH")
	if envPath != "" && fileExists(envPath) {
		return envPath
	}

	paths := getUnityHubPaths()
	for _, path := range paths {
		if fileExists(path) {
			logrus.Debugf("Found Unity Hub at: %s", path)
			return path
		}
	}

	pathCmd, err := exec.LookPath("Unity Hub")
	if err == nil {
		return pathCmd
	}

	logrus.Warn("Unity Hub not found. Please install Unity Hub or set UNITY_CLI_HUB_PATH environment variable")
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
