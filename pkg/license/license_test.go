package license

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetStatus(t *testing.T) {
	// This test checks the actual system state
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	// Verify that the function works and returns valid paths
	if status.LicensePath == "" {
		t.Error("Expected non-empty license path")
	}
	if status.HubConfigPath == "" {
		t.Error("Expected non-empty hub config path")
	}

	// Verify license type is set correctly
	if status.HasLicense {
		if status.LicenseType == LicenseTypeNone {
			t.Error("HasLicense is true but LicenseType is none")
		}
	} else {
		if status.LicenseType != LicenseTypeNone {
			t.Error("HasLicense is false but LicenseType is not none")
		}
	}
}

func TestGetSerialLicenseFilePath(t *testing.T) {
	path := getSerialLicenseFilePath()

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, "Library", "Application Support", "Unity", "Unity_lic.ulf")
		if path != expected {
			t.Errorf("Expected %s, got %s", expected, path)
		}
	case "windows":
		expected := filepath.Join("C:", "ProgramData", "Unity", "Unity_lic.ulf")
		if path != expected {
			t.Errorf("Expected %s, got %s", expected, path)
		}
	case "linux":
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".local", "share", "unity3d", "Unity", "Unity_lic.ulf")
		if path != expected {
			t.Errorf("Expected %s, got %s", expected, path)
		}
	}
}

func TestGetUnityHubConfigPath(t *testing.T) {
	path := getUnityHubConfigPath()

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, "Library", "Application Support", "UnityHub", "userInfoKey.json")
		if path != expected {
			t.Errorf("Expected %s, got %s", expected, path)
		}
	case "windows":
		// Windows path depends on APPDATA
		if path == "" {
			t.Error("Expected non-empty path for Windows")
		}
	case "linux":
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".config", "UnityHub", "userInfoKey.json")
		if path != expected {
			t.Errorf("Expected %s, got %s", expected, path)
		}
	}
}

func TestGetLicensingServerConfig(t *testing.T) {
	// Test with environment variable
	originalEnv := os.Getenv("UNITY_LICENSING_SERVER")
	originalBuildEnv := os.Getenv("UNITY_BUILD_SERVER")
	defer func() {
		_ = os.Setenv("UNITY_LICENSING_SERVER", originalEnv)
		_ = os.Setenv("UNITY_BUILD_SERVER", originalBuildEnv)
	}()

	// Test Licensing Server
	_ = os.Setenv("UNITY_LICENSING_SERVER", "https://license.example.com")
	_ = os.Unsetenv("UNITY_BUILD_SERVER")
	config := getLicensingServerConfig()
	if config.URL != "https://license.example.com" {
		t.Errorf("Expected https://license.example.com, got %s", config.URL)
	}
	if config.IsBuildServer {
		t.Error("Expected IsBuildServer to be false")
	}

	// Test Build Server
	_ = os.Setenv("UNITY_BUILD_SERVER", "true")
	config = getLicensingServerConfig()
	if !config.IsBuildServer {
		t.Error("Expected IsBuildServer to be true")
	}

	// Test without environment variable
	_ = os.Unsetenv("UNITY_LICENSING_SERVER")
	_ = os.Unsetenv("UNITY_BUILD_SERVER")
	config = getLicensingServerConfig()
	// Should return empty if no config file exists
	// (actual value depends on system state)
}

func TestFindJSONValue(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		key      string
		expected string
	}{
		{
			name:     "Simple value",
			content:  `{"licensingServiceBaseUrl": "https://example.com"}`,
			key:      `"licensingServiceBaseUrl"`,
			expected: "https://example.com",
		},
		{
			name:     "Value with spaces",
			content:  `{"licensingServiceBaseUrl" : "https://example.com"}`,
			key:      `"licensingServiceBaseUrl"`,
			expected: "https://example.com",
		},
		{
			name:     "Key not found",
			content:  `{"other": "value"}`,
			key:      `"licensingServiceBaseUrl"`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findJSONValue(tt.content, tt.key)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestLicenseType(t *testing.T) {
	// Verify license type constants
	if LicenseTypeNone != "none" {
		t.Errorf("Expected 'none', got %s", LicenseTypeNone)
	}
	if LicenseTypeSerial != "serial" {
		t.Errorf("Expected 'serial', got %s", LicenseTypeSerial)
	}
	if LicenseTypeHub != "hub" {
		t.Errorf("Expected 'hub', got %s", LicenseTypeHub)
	}
	if LicenseTypeServer != "server" {
		t.Errorf("Expected 'server', got %s", LicenseTypeServer)
	}
	if LicenseTypeBuildServer != "build_server" {
		t.Errorf("Expected 'build_server', got %s", LicenseTypeBuildServer)
	}
}

func TestFindJSONBoolValue(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		key      string
		expected bool
	}{
		{
			name:     "enableFloatingApi true",
			content:  `{"enableFloatingApi": true, "licensingServiceBaseUrl": "http://example.com"}`,
			key:      `"enableFloatingApi"`,
			expected: true,
		},
		{
			name:     "enableFloatingApi false",
			content:  `{"enableFloatingApi": false, "licensingServiceBaseUrl": "http://example.com"}`,
			key:      `"enableFloatingApi"`,
			expected: false,
		},
		{
			name:     "Key not found",
			content:  `{"licensingServiceBaseUrl": "http://example.com"}`,
			key:      `"enableFloatingApi"`,
			expected: false,
		},
		{
			name:     "enableFloatingApi with spaces",
			content:  `{"enableFloatingApi" : true}`,
			key:      `"enableFloatingApi"`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findJSONBoolValue(tt.content, tt.key)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	manager := NewManager("/path/to/unity", 600)

	if manager.editorPath != "/path/to/unity" {
		t.Errorf("Expected editor path /path/to/unity, got %s", manager.editorPath)
	}

	expectedTimeout := int64(600 * 1e9) // 600 seconds in nanoseconds
	if manager.timeout.Nanoseconds() != expectedTimeout {
		t.Errorf("Expected timeout %d, got %d", expectedTimeout, manager.timeout.Nanoseconds())
	}
}

func TestNewManager_DefaultTimeout(t *testing.T) {
	manager := NewManager("/path/to/unity", 0)

	expectedTimeout := int64(300 * 1e9) // 300 seconds (default) in nanoseconds
	if manager.timeout.Nanoseconds() != expectedTimeout {
		t.Errorf("Expected default timeout %d, got %d", expectedTimeout, manager.timeout.Nanoseconds())
	}
}

func TestActivateOptions_Validation(t *testing.T) {
	manager := NewManager("/nonexistent/unity", 1)

	tests := []struct {
		name    string
		opts    ActivateOptions
		wantErr string
	}{
		{
			name:    "Missing username",
			opts:    ActivateOptions{Password: "pass", Serial: "serial"},
			wantErr: "username is required",
		},
		{
			name:    "Missing password",
			opts:    ActivateOptions{Username: "user", Serial: "serial"},
			wantErr: "password is required",
		},
		// Note: Serial is now optional for Personal license
		// Plus/Pro license requires serial, but validation is done by Unity CLI
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.Activate(tt.opts)
			if err == nil {
				t.Error("Expected error, got nil")
				return
			}
			if err.Error() != tt.wantErr {
				t.Errorf("Expected error '%s', got '%s'", tt.wantErr, err.Error())
			}
		})
	}
}
