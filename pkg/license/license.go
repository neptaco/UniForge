package license

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Manager handles Unity license operations
type Manager struct {
	editorPath string
	timeout    time.Duration
}

// NewManager creates a new license Manager
func NewManager(editorPath string, timeoutSeconds int) *Manager {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 300 * time.Second // Default 5 minutes
	}
	return &Manager{
		editorPath: editorPath,
		timeout:    timeout,
	}
}

// ActivateOptions holds options for license activation
type ActivateOptions struct {
	Username string
	Password string
	Serial   string
}

// Activate activates Unity license
// For Personal license, serial is not required.
// For Plus/Pro license, serial is required.
func (m *Manager) Activate(opts ActivateOptions) error {
	if opts.Username == "" {
		return fmt.Errorf("username is required")
	}
	if opts.Password == "" {
		return fmt.Errorf("password is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	args := []string{
		"-batchmode",
		"-quit",
	}

	// Add serial only if provided (required for Plus/Pro, not needed for Personal)
	if opts.Serial != "" {
		args = append(args, "-serial", opts.Serial)
	}

	args = append(args, "-username", opts.Username, "-password", opts.Password)

	cmd := exec.CommandContext(ctx, m.editorPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("activation timed out after %v", m.timeout)
		}
		return fmt.Errorf("activation failed: %w", err)
	}

	return nil
}

// Return returns the Unity license
func (m *Manager) Return() error {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	args := []string{
		"-batchmode",
		"-quit",
		"-returnlicense",
	}

	cmd := exec.CommandContext(ctx, m.editorPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("return timed out after %v", m.timeout)
		}
		return fmt.Errorf("license return failed: %w", err)
	}

	return nil
}

// LicenseType represents the type of Unity license
type LicenseType string

const (
	LicenseTypeNone        LicenseType = "none"
	LicenseTypeSerial      LicenseType = "serial"       // Traditional serial key activation (Unity_lic.ulf)
	LicenseTypeHub         LicenseType = "hub"          // Unity Hub login
	LicenseTypeServer      LicenseType = "server"       // Unity Licensing Server (floating license)
	LicenseTypeBuildServer LicenseType = "build_server" // Unity Build Server (floating license for CI)
)

// Status represents the current license status
type Status struct {
	HasLicense    bool
	LicenseType   LicenseType
	LicensePath   string // For serial license
	HubConfigPath string // For Unity Hub
	ServerURL     string // For Licensing Server
}

// GetStatus checks the current license status across all license types
func GetStatus() (*Status, error) {
	status := &Status{
		HasLicense:  false,
		LicenseType: LicenseTypeNone,
	}

	// Check 1: Traditional serial license (Unity_lic.ulf)
	licensePath := getSerialLicenseFilePath()
	status.LicensePath = licensePath
	if fileExists(licensePath) {
		status.HasLicense = true
		status.LicenseType = LicenseTypeSerial
		return status, nil
	}

	// Check 2: Unity Hub login
	hubConfigPath := getUnityHubConfigPath()
	status.HubConfigPath = hubConfigPath
	if fileExists(hubConfigPath) {
		status.HasLicense = true
		status.LicenseType = LicenseTypeHub
		return status, nil
	}

	// Check 3: Licensing Server / Build Server
	serverConfig := getLicensingServerConfig()
	status.ServerURL = serverConfig.URL
	if serverConfig.URL != "" {
		status.HasLicense = true
		if serverConfig.IsBuildServer {
			status.LicenseType = LicenseTypeBuildServer
		} else {
			status.LicenseType = LicenseTypeServer
		}
		return status, nil
	}

	return status, nil
}

// serverConfigResult holds the result of licensing server config detection
type serverConfigResult struct {
	URL           string
	IsBuildServer bool
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// getSerialLicenseFilePath returns the Unity license file path for serial activation
func getSerialLicenseFilePath() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Unity", "Unity_lic.ulf")
	case "windows":
		return filepath.Join("C:", "ProgramData", "Unity", "Unity_lic.ulf")
	case "linux":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "unity3d", "Unity", "Unity_lic.ulf")
	default:
		return ""
	}
}

// getUnityHubConfigPath returns the Unity Hub user config path
func getUnityHubConfigPath() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "UnityHub", "userInfoKey.json")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, _ := os.UserHomeDir()
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "UnityHub", "userInfoKey.json")
	case "linux":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "UnityHub", "userInfoKey.json")
	default:
		return ""
	}
}

// getLicensingServerConfig returns the configured Licensing Server URL and type
func getLicensingServerConfig() serverConfigResult {
	result := serverConfigResult{}

	// Check environment variable first
	if url := os.Getenv("UNITY_LICENSING_SERVER"); url != "" {
		result.URL = url
		// Check if Build Server is specified via env
		result.IsBuildServer = os.Getenv("UNITY_BUILD_SERVER") == "true"
		return result
	}

	// Check services-config.json
	configPaths := getServicesConfigPaths()
	for _, configPath := range configPaths {
		if cfg := readServerConfigFromFile(configPath); cfg.URL != "" {
			return cfg
		}
	}

	return result
}

// getServicesConfigPaths returns possible paths for services-config.json
func getServicesConfigPaths() []string {
	var paths []string

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		paths = append(paths,
			filepath.Join(home, "Library", "Application Support", "Unity", "config", "services-config.json"),
			"/Library/Application Support/Unity/config/services-config.json",
		)
	case "windows":
		paths = append(paths,
			filepath.Join("C:", "ProgramData", "Unity", "config", "services-config.json"),
		)
	case "linux":
		home, _ := os.UserHomeDir()
		paths = append(paths,
			filepath.Join(home, ".config", "unity3d", "Unity", "services-config.json"),
			"/usr/share/unity3d/config/services-config.json",
		)
	}

	return paths
}

// readServerConfigFromFile reads the licensing server config from services-config.json
func readServerConfigFromFile(configPath string) serverConfigResult {
	result := serverConfigResult{}

	if !fileExists(configPath) {
		return result
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return result
	}

	content := string(data)

	// Try to find licensing server URL patterns
	patterns := []string{
		`"licensingServiceBaseUrl"`,
		`"licensing_url"`,
		`"license_server"`,
	}

	for _, pattern := range patterns {
		if url := findJSONValue(content, pattern); url != "" {
			result.URL = url
			break
		}
	}

	// Check if this is a Build Server configuration
	// Build Server uses "enableFloatingApi": true
	if findJSONBoolValue(content, `"enableFloatingApi"`) {
		result.IsBuildServer = true
	}

	return result
}

// findJSONBoolValue checks if a JSON boolean key is set to true
func findJSONBoolValue(content, key string) bool {
	pos := indexOf(content, key)
	if pos == -1 {
		return false
	}

	// Move past the key
	idx := pos + len(key)

	// Skip whitespace and colon
	for idx < len(content) && (content[idx] == ' ' || content[idx] == ':' || content[idx] == '\t' || content[idx] == '\n') {
		idx++
	}

	// Check for "true"
	if idx+4 <= len(content) && content[idx:idx+4] == "true" {
		return true
	}

	return false
}

// findJSONValue is a simple helper to extract JSON string value
func findJSONValue(content, key string) string {
	idx := 0
	for {
		pos := indexOf(content[idx:], key)
		if pos == -1 {
			break
		}
		idx += pos + len(key)

		// Skip whitespace and colon
		for idx < len(content) && (content[idx] == ' ' || content[idx] == ':' || content[idx] == '\t' || content[idx] == '\n') {
			idx++
		}

		// Check for string value
		if idx < len(content) && content[idx] == '"' {
			idx++
			end := indexOf(content[idx:], `"`)
			if end != -1 {
				return content[idx : idx+end]
			}
		}
	}
	return ""
}

// indexOf returns the index of substr in s, or -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
