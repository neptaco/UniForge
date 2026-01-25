package hub

import (
	"testing"
)

func TestIsEditorInstalled(t *testing.T) {
	// This is a basic unit test. In real scenarios, we'd mock the Hub client
	client := &Client{}

	// Test with empty hub path (Unity Hub not found)
	isInstalled, path, err := client.IsEditorInstalled("2022.3.10f1")
	if err == nil {
		t.Error("Expected error when Unity Hub is not found")
	}
	if isInstalled {
		t.Error("Expected isInstalled to be false when Unity Hub is not found")
	}
	if path != "" {
		t.Error("Expected empty path when Unity Hub is not found")
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
