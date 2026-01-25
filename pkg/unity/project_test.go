package unity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProject(t *testing.T) {
	tempDir := t.TempDir()
	projectSettingsDir := filepath.Join(tempDir, "ProjectSettings")

	if err := os.MkdirAll(projectSettingsDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	versionFile := filepath.Join(projectSettingsDir, "ProjectVersion.txt")
	content := `m_EditorVersion: 2022.3.10f1
m_EditorVersionWithRevision: 2022.3.10f1 (1234567890ab)`

	if err := os.WriteFile(versionFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write version file: %v", err)
	}

	project, err := LoadProject(tempDir)
	if err != nil {
		t.Fatalf("LoadProject failed: %v", err)
	}

	if project.UnityVersion != "2022.3.10f1" {
		t.Errorf("Expected version 2022.3.10f1, got %s", project.UnityVersion)
	}

	if project.Name != filepath.Base(tempDir) {
		t.Errorf("Expected project name %s, got %s", filepath.Base(tempDir), project.Name)
	}

	absPath, _ := filepath.Abs(tempDir)
	if project.Path != absPath {
		t.Errorf("Expected path %s, got %s", absPath, project.Path)
	}
}

func TestLoadProject_NotUnityProject(t *testing.T) {
	tempDir := t.TempDir()

	_, err := LoadProject(tempDir)
	if err == nil {
		t.Error("Expected error for non-Unity project, got nil")
	}
}

func TestLoadProject_InvalidVersionFile(t *testing.T) {
	tempDir := t.TempDir()
	projectSettingsDir := filepath.Join(tempDir, "ProjectSettings")

	if err := os.MkdirAll(projectSettingsDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	versionFile := filepath.Join(projectSettingsDir, "ProjectVersion.txt")
	content := `invalid content without version`

	if err := os.WriteFile(versionFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write version file: %v", err)
	}

	_, err := LoadProject(tempDir)
	if err == nil {
		t.Error("Expected error for invalid version file, got nil")
	}
}

func TestReadUnityVersion(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "Valid version",
			content: "m_EditorVersion: 2022.3.10f1\n",
			want:    "2022.3.10f1",
			wantErr: false,
		},
		{
			name:    "Valid version with revision",
			content: "m_EditorVersion: 2022.3.10f1\nm_EditorVersionWithRevision: 2022.3.10f1 (1234567890ab)",
			want:    "2022.3.10f1",
			wantErr: false,
		},
		{
			name:    "Version with spaces",
			content: "m_EditorVersion:   2022.3.10f1   \n",
			want:    "2022.3.10f1",
			wantErr: false,
		},
		{
			name:    "No version",
			content: "some other content",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempFile, err := os.CreateTemp("", "version*.txt")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer func() { _ = os.Remove(tempFile.Name()) }()

			if _, err := tempFile.WriteString(tt.content); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}
			_ = tempFile.Close()

			got, err := readUnityVersion(tempFile.Name())
			if (err != nil) != tt.wantErr {
				t.Errorf("readUnityVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("readUnityVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
