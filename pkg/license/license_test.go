package license

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetStatus_NoLicense(t *testing.T) {
	// This test checks the actual system state
	// It may return HasLicense=true if Unity is installed
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	// Just verify that the function works and returns a valid path
	if status.LicensePath == "" {
		t.Error("Expected non-empty license path")
	}
}

func TestGetLicenseFilePath(t *testing.T) {
	path := getLicenseFilePath()

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
		{
			name:    "Missing serial",
			opts:    ActivateOptions{Username: "user", Password: "pass"},
			wantErr: "serial key is required",
		},
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
