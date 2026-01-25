package unity

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestProject(t *testing.T) (*Project, string) {
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

	// Create Assets directory
	assetsDir := filepath.Join(tempDir, "Assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		t.Fatalf("Failed to create Assets directory: %v", err)
	}

	project, err := LoadProject(tempDir)
	if err != nil {
		t.Fatalf("Failed to load project: %v", err)
	}

	return project, tempDir
}

func createAssetWithMeta(t *testing.T, dir, name, guid string) {
	assetPath := filepath.Join(dir, name)
	if err := os.WriteFile(assetPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create asset: %v", err)
	}

	metaContent := "fileFormatVersion: 2\nguid: " + guid + "\n"
	metaPath := assetPath + ".meta"
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		t.Fatalf("Failed to create meta: %v", err)
	}
}

func createAssetWithoutMeta(t *testing.T, dir, name string) {
	assetPath := filepath.Join(dir, name)
	if err := os.WriteFile(assetPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create asset: %v", err)
	}
}

func createOrphanMeta(t *testing.T, dir, name, guid string) {
	metaContent := "fileFormatVersion: 2\nguid: " + guid + "\n"
	metaPath := filepath.Join(dir, name+".meta")
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		t.Fatalf("Failed to create orphan meta: %v", err)
	}
}

func TestMetaChecker_Check_NoIssues(t *testing.T) {
	project, tempDir := setupTestProject(t)
	assetsDir := filepath.Join(tempDir, "Assets")

	// Create properly paired assets inside Assets/
	createAssetWithMeta(t, assetsDir, "Script.cs", "abc123")
	createAssetWithMeta(t, assetsDir, "Material.mat", "def456")

	// Note: Root level folders (Assets, ProjectSettings) don't need .meta files
	// Only files inside Assets/ and Packages/ need .meta files

	checker := NewMetaChecker(project)
	result, err := checker.Check()
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if len(result.MissingMeta) != 0 {
		t.Errorf("Expected 0 missing meta, got %d: %v", len(result.MissingMeta), result.MissingMeta)
	}
	if len(result.OrphanMeta) != 0 {
		t.Errorf("Expected 0 orphan meta, got %d: %v", len(result.OrphanMeta), result.OrphanMeta)
	}
	if len(result.DuplicateGUIDs) != 0 {
		t.Errorf("Expected 0 duplicate GUIDs, got %d", len(result.DuplicateGUIDs))
	}
}

func TestMetaChecker_Check_MissingMeta(t *testing.T) {
	project, tempDir := setupTestProject(t)
	assetsDir := filepath.Join(tempDir, "Assets")

	// Create asset without meta
	createAssetWithoutMeta(t, assetsDir, "MissingMeta.cs")

	checker := NewMetaChecker(project)
	result, err := checker.Check()
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	// Should have missing meta (asset + folders without meta)
	hasMissingMeta := false
	for _, path := range result.MissingMeta {
		if filepath.Base(path) == "MissingMeta.cs" {
			hasMissingMeta = true
			break
		}
	}
	if !hasMissingMeta {
		t.Errorf("Expected MissingMeta.cs in missing meta list, got: %v", result.MissingMeta)
	}

	if !result.HasErrors() {
		t.Error("Expected HasErrors() to return true")
	}
}

func TestMetaChecker_Check_OrphanMeta(t *testing.T) {
	project, tempDir := setupTestProject(t)
	assetsDir := filepath.Join(tempDir, "Assets")

	// Create orphan meta (no corresponding asset)
	createOrphanMeta(t, assetsDir, "Deleted.cs", "orphan123")

	checker := NewMetaChecker(project)
	result, err := checker.Check()
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	hasOrphanMeta := false
	for _, path := range result.OrphanMeta {
		if filepath.Base(path) == "Deleted.cs.meta" {
			hasOrphanMeta = true
			break
		}
	}
	if !hasOrphanMeta {
		t.Errorf("Expected Deleted.cs.meta in orphan meta list, got: %v", result.OrphanMeta)
	}

	if !result.HasWarnings() {
		t.Error("Expected HasWarnings() to return true")
	}
}

func TestMetaChecker_Check_DuplicateGUIDs(t *testing.T) {
	project, tempDir := setupTestProject(t)
	assetsDir := filepath.Join(tempDir, "Assets")

	// Create two assets with same GUID
	duplicateGUID := "duplicate123"
	createAssetWithMeta(t, assetsDir, "First.cs", duplicateGUID)
	createAssetWithMeta(t, assetsDir, "Second.cs", duplicateGUID)

	checker := NewMetaChecker(project)
	result, err := checker.Check()
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if len(result.DuplicateGUIDs) != 1 {
		t.Errorf("Expected 1 duplicate GUID, got %d", len(result.DuplicateGUIDs))
	}

	files, exists := result.DuplicateGUIDs[duplicateGUID]
	if !exists {
		t.Error("Expected duplicate GUID to be detected")
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 files with duplicate GUID, got %d", len(files))
	}

	if !result.HasErrors() {
		t.Error("Expected HasErrors() to return true")
	}
}

func TestMetaChecker_Check_ExcludedDirectories(t *testing.T) {
	project, tempDir := setupTestProject(t)

	// Create files in excluded directories (should be ignored)
	libraryDir := filepath.Join(tempDir, "Library")
	if err := os.MkdirAll(libraryDir, 0755); err != nil {
		t.Fatalf("Failed to create Library directory: %v", err)
	}
	createAssetWithoutMeta(t, libraryDir, "CachedFile.txt")

	tempDirUnity := filepath.Join(tempDir, "Temp")
	if err := os.MkdirAll(tempDirUnity, 0755); err != nil {
		t.Fatalf("Failed to create Temp directory: %v", err)
	}
	createAssetWithoutMeta(t, tempDirUnity, "TempFile.txt")

	checker := NewMetaChecker(project)
	result, err := checker.Check()
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	// Files in excluded directories should not appear in results
	for _, path := range result.MissingMeta {
		if filepath.Base(path) == "CachedFile.txt" || filepath.Base(path) == "TempFile.txt" {
			t.Errorf("Excluded file should not appear in results: %s", path)
		}
	}
}

func TestMetaChecker_Check_ExcludedFiles(t *testing.T) {
	project, tempDir := setupTestProject(t)
	assetsDir := filepath.Join(tempDir, "Assets")

	// Create excluded files (should be ignored)
	excludedFiles := []string{".gitignore", ".gitattributes", ".DS_Store", "Thumbs.db"}
	for _, name := range excludedFiles {
		createAssetWithoutMeta(t, assetsDir, name)
	}

	checker := NewMetaChecker(project)
	result, err := checker.Check()
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	// Excluded files should not appear in results
	for _, path := range result.MissingMeta {
		baseName := filepath.Base(path)
		for _, excluded := range excludedFiles {
			if baseName == excluded {
				t.Errorf("Excluded file should not appear in results: %s", path)
			}
		}
	}
}

func TestMetaChecker_Fix(t *testing.T) {
	project, tempDir := setupTestProject(t)
	assetsDir := filepath.Join(tempDir, "Assets")

	// Create orphan meta files
	createOrphanMeta(t, assetsDir, "Orphan1.cs", "orphan1")
	createOrphanMeta(t, assetsDir, "Orphan2.cs", "orphan2")

	checker := NewMetaChecker(project)

	// Test dry run first
	deleted, err := checker.Fix(true)
	if err != nil {
		t.Fatalf("Fix dry run failed: %v", err)
	}
	if len(deleted) != 2 {
		t.Errorf("Expected 2 files to be deleted in dry run, got %d", len(deleted))
	}

	// Verify files still exist after dry run
	if _, err := os.Stat(filepath.Join(assetsDir, "Orphan1.cs.meta")); os.IsNotExist(err) {
		t.Error("Orphan1.cs.meta should still exist after dry run")
	}

	// Test actual fix
	deleted, err = checker.Fix(false)
	if err != nil {
		t.Fatalf("Fix failed: %v", err)
	}
	if len(deleted) != 2 {
		t.Errorf("Expected 2 files to be deleted, got %d", len(deleted))
	}

	// Verify files are actually deleted
	if _, err := os.Stat(filepath.Join(assetsDir, "Orphan1.cs.meta")); !os.IsNotExist(err) {
		t.Error("Orphan1.cs.meta should be deleted")
	}
	if _, err := os.Stat(filepath.Join(assetsDir, "Orphan2.cs.meta")); !os.IsNotExist(err) {
		t.Error("Orphan2.cs.meta should be deleted")
	}
}

func TestExtractGUID(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "Valid GUID",
			content: "fileFormatVersion: 2\nguid: abc123def456\n",
			want:    "abc123def456",
			wantErr: false,
		},
		{
			name:    "GUID with spaces",
			content: "guid:   abc123def456   \n",
			want:    "abc123def456",
			wantErr: false,
		},
		{
			name:    "No GUID",
			content: "fileFormatVersion: 2\n",
			want:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempFile, err := os.CreateTemp("", "meta*.meta")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer func() { _ = os.Remove(tempFile.Name()) }()

			if _, err := tempFile.WriteString(tt.content); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}
			_ = tempFile.Close()

			got, err := extractGUID(tempFile.Name())
			if (err != nil) != tt.wantErr {
				t.Errorf("extractGUID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractGUID() = %v, want %v", got, tt.want)
			}
		})
	}
}
