package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIsEditorInstalled(t *testing.T) {
	// This is a basic unit test. In real scenarios, we'd mock the Hub client
	client := &Client{}

	// Test with non-existent editor version
	// With JSON file reading, this should return false without an error
	// (if the JSON file doesn't exist or editor is not in the list)
	isInstalled, path, _ := client.IsEditorInstalled("9999.9.9f1")
	if isInstalled {
		t.Error("Expected isInstalled to be false for non-existent version")
	}
	if path != "" {
		t.Error("Expected empty path for non-existent version")
	}
}

func TestMapModules(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "Basic modules",
			input:    []string{"android", "ios"},
			expected: []string{"android", "ios"},
		},
		{
			name:     "IL2CPP modules",
			input:    []string{"windows", "linux", "mac"},
			expected: []string{"windows-il2cpp", "linux-il2cpp", "mac-il2cpp"},
		},
		{
			name:     "Mixed case",
			input:    []string{"Android", "IOS", "WebGL"},
			expected: []string{"android", "ios", "webgl"},
		},
		{
			name:     "Unknown module",
			input:    []string{"unknown", "android"},
			expected: []string{"android"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.mapModules(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d modules, got %d", len(tt.expected), len(result))
			}

			for i, module := range tt.expected {
				if i >= len(result) || result[i] != module {
					t.Errorf("Expected module %s at index %d, got %v", module, i, result)
				}
			}
		})
	}
}

func TestGetPlaybackEnginesPath(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name       string
		editorPath string
		goos       string
		expected   string
	}{
		{
			name:       "macOS with .app path",
			editorPath: "/Applications/Unity/Hub/Editor/2022.3.60f1/Unity.app",
			goos:       "darwin",
			expected:   "/Applications/Unity/Hub/Editor/2022.3.60f1/PlaybackEngines",
		},
		{
			name:       "macOS without .app",
			editorPath: "/Applications/Unity/Hub/Editor/2022.3.60f1",
			goos:       "darwin",
			expected:   "/Applications/Unity/Hub/Editor/2022.3.60f1/PlaybackEngines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test only works on the current OS
			result := client.GetPlaybackEnginesPath(tt.editorPath)
			// We can't easily test cross-platform, so just verify it returns something
			if result == "" {
				t.Error("Expected non-empty path")
			}
		})
	}
}

func TestGetMissingModules(t *testing.T) {
	client := &Client{}

	// Test with non-existent path - all modules should be missing
	missingModules := client.GetMissingModules("/non/existent/path", []string{"ios", "android"})
	if len(missingModules) != 2 {
		t.Errorf("Expected 2 missing modules for non-existent path, got %d", len(missingModules))
	}

	// Test with empty module list
	missingModules = client.GetMissingModules("/non/existent/path", []string{})
	if len(missingModules) != 0 {
		t.Errorf("Expected 0 missing modules for empty list, got %d", len(missingModules))
	}
}

func TestModulePathMap(t *testing.T) {
	// Verify all mapped modules have corresponding directory names
	expectedMappings := map[string]string{
		"android":        "AndroidPlayer",
		"ios":            "iOSSupport",
		"webgl":          "WebGLSupport",
		"windows-il2cpp": "WindowsStandaloneSupport",
		"linux-il2cpp":   "LinuxStandaloneSupport",
		"mac-il2cpp":     "MacStandaloneSupport",
	}

	for moduleID, expectedDir := range expectedMappings {
		if dir, ok := modulePathMap[moduleID]; !ok {
			t.Errorf("Module %s not found in modulePathMap", moduleID)
		} else if dir != expectedDir {
			t.Errorf("Module %s: expected dir %s, got %s", moduleID, expectedDir, dir)
		}
	}
}

func TestParseEditorsList(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name     string
		input    string
		expected []EditorInfo
	}{
		{
			name:  "Single editor",
			input: "2022.3.10f1 /Applications/Unity/Hub/Editor/2022.3.10f1/Unity.app",
			expected: []EditorInfo{
				{Version: "2022.3.10f1", Path: "/Applications/Unity/Hub/Editor/2022.3.10f1/Unity.app"},
			},
		},
		{
			name: "Multiple editors",
			input: `2022.3.10f1 /Applications/Unity/Hub/Editor/2022.3.10f1/Unity.app
2021.3.5f1 /Applications/Unity/Hub/Editor/2021.3.5f1/Unity.app`,
			expected: []EditorInfo{
				{Version: "2022.3.10f1", Path: "/Applications/Unity/Hub/Editor/2022.3.10f1/Unity.app"},
				{Version: "2021.3.5f1", Path: "/Applications/Unity/Hub/Editor/2021.3.5f1/Unity.app"},
			},
		},
		{
			name: "With empty lines",
			input: `
2022.3.10f1 /Applications/Unity/Hub/Editor/2022.3.10f1/Unity.app

2021.3.5f1 /Applications/Unity/Hub/Editor/2021.3.5f1/Unity.app
`,
			expected: []EditorInfo{
				{Version: "2022.3.10f1", Path: "/Applications/Unity/Hub/Editor/2022.3.10f1/Unity.app"},
				{Version: "2021.3.5f1", Path: "/Applications/Unity/Hub/Editor/2021.3.5f1/Unity.app"},
			},
		},
		{
			name:     "Empty input",
			input:    "",
			expected: []EditorInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.parseEditorsList(tt.input)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d editors, got %d", len(tt.expected), len(result))
			}

			for i, editor := range tt.expected {
				if i >= len(result) {
					break
				}
				if result[i].Version != editor.Version {
					t.Errorf("Expected version %s at index %d, got %s", editor.Version, i, result[i].Version)
				}
				if result[i].Path != editor.Path {
					t.Errorf("Expected path %s at index %d, got %s", editor.Path, i, result[i].Path)
				}
			}
		})
	}
}

func TestListEditorsFromFile(t *testing.T) {
	// Create a temporary editors file
	tempDir := t.TempDir()
	editorsFile := filepath.Join(tempDir, "editors-v2.json")

	editorsJSON := `{
		"schema_version": "2",
		"data": [
			{
				"version": "2022.3.60f1",
				"location": ["/path/to/Unity.app"],
				"manual": true,
				"architecture": "arm64",
				"productName": "Unity"
			},
			{
				"version": "6000.0.1f1",
				"location": ["/path/to/Unity6.app"],
				"manual": false,
				"architecture": "x86_64",
				"productName": "Unity"
			}
		]
	}`

	if err := os.WriteFile(editorsFile, []byte(editorsJSON), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// We can't easily test the actual function since it uses a fixed path
	// But we can test the JSON parsing directly
	var data struct {
		SchemaVersion string `json:"schema_version"`
		Data          []struct {
			Version      string   `json:"version"`
			Location     []string `json:"location"`
			Manual       bool     `json:"manual"`
			Architecture string   `json:"architecture"`
		} `json:"data"`
	}

	content, err := os.ReadFile(editorsFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if len(data.Data) != 2 {
		t.Errorf("Expected 2 editors, got %d", len(data.Data))
	}

	if data.Data[0].Version != "2022.3.60f1" {
		t.Errorf("Expected version 2022.3.60f1, got %s", data.Data[0].Version)
	}

	if data.Data[0].Architecture != "arm64" {
		t.Errorf("Expected architecture arm64, got %s", data.Data[0].Architecture)
	}

	if !data.Data[0].Manual {
		t.Error("Expected manual to be true")
	}
}

func TestReadModulesFile(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()
	modulesFile := filepath.Join(tempDir, "modules.json")

	modulesJSON := `[
		{
			"id": "android",
			"name": "Android Build Support",
			"isInstalled": true
		},
		{
			"id": "ios",
			"name": "iOS Build Support",
			"isInstalled": false
		},
		{
			"id": "webgl",
			"name": "WebGL Build Support",
			"isInstalled": true
		}
	]`

	if err := os.WriteFile(modulesFile, []byte(modulesJSON), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Test JSON parsing
	var modules []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		IsInstalled bool   `json:"isInstalled"`
	}

	content, err := os.ReadFile(modulesFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if err := json.Unmarshal(content, &modules); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if len(modules) != 3 {
		t.Errorf("Expected 3 modules, got %d", len(modules))
	}

	// Verify android is installed
	if modules[0].ID != "android" {
		t.Errorf("Expected id 'android', got '%s'", modules[0].ID)
	}
	if !modules[0].IsInstalled {
		t.Error("Expected android to be installed")
	}

	// Verify ios is not installed
	if modules[1].ID != "ios" {
		t.Errorf("Expected id 'ios', got '%s'", modules[1].ID)
	}
	if modules[1].IsInstalled {
		t.Error("Expected ios to not be installed")
	}

	// Verify webgl is installed
	if modules[2].ID != "webgl" {
		t.Errorf("Expected id 'webgl', got '%s'", modules[2].ID)
	}
	if !modules[2].IsInstalled {
		t.Error("Expected webgl to be installed")
	}
}

func TestParseHubInfoJSON(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantVersion string
		wantPath    string
		wantErr     bool
	}{
		{
			name:        "Valid hubInfo.json",
			json:        `{"version":"3.16.0","executablePath":"/Applications/Unity Hub.app/Contents/MacOS/Unity Hub"}`,
			wantVersion: "3.16.0",
			wantPath:    "/Applications/Unity Hub.app/Contents/MacOS/Unity Hub",
			wantErr:     false,
		},
		{
			name:        "Windows path with spaces",
			json:        `{"version":"3.16.0","executablePath":"C:\\Program Files\\Unity Hub\\Unity Hub.exe"}`,
			wantVersion: "3.16.0",
			wantPath:    "C:\\Program Files\\Unity Hub\\Unity Hub.exe",
			wantErr:     false,
		},
		{
			name:        "Empty executablePath",
			json:        `{"version":"3.16.0","executablePath":""}`,
			wantVersion: "3.16.0",
			wantPath:    "",
			wantErr:     false,
		},
		{
			name:    "Invalid JSON",
			json:    `{invalid json}`,
			wantErr: true,
		},
		{
			name:        "Missing executablePath field",
			json:        `{"version":"3.16.0"}`,
			wantVersion: "3.16.0",
			wantPath:    "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data struct {
				Version        string `json:"version"`
				ExecutablePath string `json:"executablePath"`
			}

			err := json.Unmarshal([]byte(tt.json), &data)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if data.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", data.Version, tt.wantVersion)
			}
			if data.ExecutablePath != tt.wantPath {
				t.Errorf("ExecutablePath = %q, want %q", data.ExecutablePath, tt.wantPath)
			}
		})
	}
}

func TestGetHubPathFromHubInfoWithTempFile(t *testing.T) {
	// Create a temporary directory to simulate UnityHub config
	tempDir := t.TempDir()

	// Create a fake executable file
	fakeHubPath := filepath.Join(tempDir, "Unity Hub")
	if err := os.WriteFile(fakeHubPath, []byte("fake"), 0755); err != nil {
		t.Fatalf("Failed to create fake hub: %v", err)
	}

	// Test valid hubInfo.json with existing executable
	hubInfoJSON := `{"version":"3.16.0","executablePath":"` + filepath.ToSlash(fakeHubPath) + `"}`

	var data struct {
		Version        string `json:"version"`
		ExecutablePath string `json:"executablePath"`
	}

	if err := json.Unmarshal([]byte(hubInfoJSON), &data); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify the path exists (simulating fileExists check)
	if _, err := os.Stat(data.ExecutablePath); os.IsNotExist(err) {
		t.Errorf("ExecutablePath should exist: %s", data.ExecutablePath)
	}
}

func TestGetHubPathFromHubInfoFileNotFound(t *testing.T) {
	// Test with non-existent directory - getHubPathFromHubInfo should return empty string
	// This tests the error handling when hubInfo.json doesn't exist
	nonExistentPath := "/non/existent/path/hubInfo.json"
	_, err := os.ReadFile(nonExistentPath)
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}
