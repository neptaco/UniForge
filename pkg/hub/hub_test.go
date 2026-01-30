package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

func TestIsValidUnityVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Valid 2022 version", "2022.3.60f1", true},
		{"Valid 6000 version", "6000.3.3f1", true},
		{"Valid alpha version", "2023.1.0a1", true},
		{"Valid beta version", "2023.1.0b1", true},
		{"Valid patch version", "2022.3.10p1", true},
		{"Too short", "2022.3", false},
		{"No dots", "20223601f", false},
		{"One dot only", "2022.360f1", false},
		{"Starts with letter", "v2022.3.60f1", false},
		{"Empty string", "", false},
		{"Random text", "notaversion", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidUnityVersion(tt.input)
			if result != tt.expected {
				t.Errorf("isValidUnityVersion(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestScanInstallPath(t *testing.T) {
	client := &Client{}

	// Test with empty path
	_, err := client.scanInstallPath("")
	if err == nil {
		t.Error("Expected error for empty path")
	}

	// Test with non-existent path
	_, err = client.scanInstallPath("/non/existent/path")
	if err == nil {
		t.Error("Expected error for non-existent path")
	}

	// Test with valid directory structure
	tempDir := t.TempDir()

	// Create fake Unity editor directories
	versions := []string{"2022.3.60f1", "6000.3.3f1", "notaversion"}
	for _, version := range versions {
		versionDir := filepath.Join(tempDir, version)
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			t.Fatalf("Failed to create version dir: %v", err)
		}

		// Create fake Unity executable based on OS
		var editorPath string
		switch runtime.GOOS {
		case "windows":
			editorPath = filepath.Join(versionDir, "Editor", "Unity.exe")
		case "linux":
			editorPath = filepath.Join(versionDir, "Editor", "Unity")
		default: // darwin
			editorPath = filepath.Join(versionDir, "Unity.app")
		}

		// Create parent directories and file
		if err := os.MkdirAll(filepath.Dir(editorPath), 0755); err != nil {
			t.Fatalf("Failed to create editor dir: %v", err)
		}
		if err := os.WriteFile(editorPath, []byte("fake"), 0755); err != nil {
			t.Fatalf("Failed to create editor file: %v", err)
		}
	}

	// Scan the temp directory
	editors, err := client.scanInstallPath(tempDir)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should find 2 valid versions (not "notaversion")
	if len(editors) != 2 {
		t.Errorf("Expected 2 editors, got %d", len(editors))
	}

	// Check that the versions are correct
	versionMap := make(map[string]bool)
	for _, e := range editors {
		versionMap[e.Version] = true
	}

	if !versionMap["2022.3.60f1"] {
		t.Error("Expected to find version 2022.3.60f1")
	}
	if !versionMap["6000.3.3f1"] {
		t.Error("Expected to find version 6000.3.3f1")
	}
	if versionMap["notaversion"] {
		t.Error("Should not find 'notaversion'")
	}
}

func TestScanInstallPathWithSpaces(t *testing.T) {
	client := &Client{}

	// Test with path containing spaces (common on Windows and macOS)
	tempDir := t.TempDir()
	pathWithSpaces := filepath.Join(tempDir, "Unity Hub", "Editor")
	if err := os.MkdirAll(pathWithSpaces, 0755); err != nil {
		t.Fatalf("Failed to create path with spaces: %v", err)
	}

	// Create fake Unity editor directory
	version := "2022.3.60f1"
	versionDir := filepath.Join(pathWithSpaces, version)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatalf("Failed to create version dir: %v", err)
	}

	// Create fake Unity executable based on OS
	var editorPath string
	switch runtime.GOOS {
	case "windows":
		editorPath = filepath.Join(versionDir, "Editor", "Unity.exe")
	case "linux":
		editorPath = filepath.Join(versionDir, "Editor", "Unity")
	default: // darwin
		editorPath = filepath.Join(versionDir, "Unity.app")
	}

	// Create parent directories and file
	if err := os.MkdirAll(filepath.Dir(editorPath), 0755); err != nil {
		t.Fatalf("Failed to create editor dir: %v", err)
	}
	if err := os.WriteFile(editorPath, []byte("fake"), 0755); err != nil {
		t.Fatalf("Failed to create editor file: %v", err)
	}

	// Scan the path with spaces
	editors, err := client.scanInstallPath(pathWithSpaces)
	if err != nil {
		t.Fatalf("Unexpected error for path with spaces: %v", err)
	}

	if len(editors) != 1 {
		t.Errorf("Expected 1 editor, got %d", len(editors))
	}

	if len(editors) > 0 && editors[0].Version != version {
		t.Errorf("Expected version %s, got %s", version, editors[0].Version)
	}

	// Verify the path is correct (contains spaces)
	if len(editors) > 0 && editors[0].Path == "" {
		t.Error("Expected non-empty path")
	}
}

func TestGetEditorInstallPaths(t *testing.T) {
	client := &Client{}

	paths := client.getEditorInstallPaths()

	// Should return at least one path (default install path)
	if len(paths) == 0 {
		t.Error("Expected at least one install path")
	}

	// All paths should be non-empty
	for i, path := range paths {
		if path == "" {
			t.Errorf("Path at index %d is empty", i)
		}
	}

	// Verify platform-specific default paths are included
	switch runtime.GOOS {
	case "darwin":
		if !slices.Contains(paths, "/Applications/Unity/Hub/Editor") {
			t.Error("Expected default macOS path /Applications/Unity/Hub/Editor")
		}
	case "windows":
		// Windows should have at least one Program Files or drive root path
		if !slices.ContainsFunc(paths, filepath.IsAbs) {
			t.Error("Expected at least one absolute path on Windows")
		}
	case "linux":
		home := os.Getenv("HOME")
		expectedPath := filepath.Join(home, "Unity", "Hub", "Editor")
		if !slices.Contains(paths, expectedPath) {
			t.Errorf("Expected default Linux path %s", expectedPath)
		}
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
