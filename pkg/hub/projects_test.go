package hub

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseProjectsFile(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected int
		wantErr  bool
	}{
		{
			name: "Valid projects file",
			json: `{
				"schema_version": "v1",
				"data": {
					"/path/to/project1": {
						"title": "Project1",
						"path": "/path/to/project1",
						"version": "2022.3.60f1",
						"lastModified": 1700000000000
					},
					"/path/to/project2": {
						"title": "Project2",
						"path": "/path/to/project2",
						"version": "6000.3.2f1",
						"lastModified": 1700000000000
					}
				}
			}`,
			expected: 2,
			wantErr:  false,
		},
		{
			name: "Empty data",
			json: `{
				"schema_version": "v1",
				"data": {}
			}`,
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "Invalid JSON",
			json:     `{invalid}`,
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data projectsFileData
			err := json.Unmarshal([]byte(tt.json), &data)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(data.Data) != tt.expected {
				t.Errorf("Expected %d projects, got %d", tt.expected, len(data.Data))
			}
		})
	}
}

func createTestClient(t *testing.T, projectsJSON string) *Client {
	t.Helper()
	tempDir := t.TempDir()
	projectsFile := filepath.Join(tempDir, "projects-v1.json")

	if err := os.WriteFile(projectsFile, []byte(projectsJSON), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	return &Client{projectsFileOverride: projectsFile}
}

func TestFindProjectsByName(t *testing.T) {
	projectsJSON := `{
		"schema_version": "v1",
		"data": {
			"/path/to/my-game": {
				"title": "my-game",
				"path": "/path/to/my-game",
				"version": "2022.3.60f1"
			},
			"/path/to/my-game-client": {
				"title": "my-game-client",
				"path": "/path/to/my-game-client",
				"version": "2022.3.60f1"
			},
			"/path/to/other-project": {
				"title": "other-project",
				"path": "/path/to/other-project",
				"version": "6000.3.2f1"
			}
		}
	}`

	client := createTestClient(t, projectsJSON)

	tests := []struct {
		name     string
		query    string
		expected int
	}{
		{
			name:     "Exact match",
			query:    "my-game",
			expected: 1, // exact match returns only exact
		},
		{
			name:     "Prefix match",
			query:    "my-g",
			expected: 2, // my-game and my-game-client
		},
		{
			name:     "Contains match",
			query:    "game",
			expected: 2, // my-game and my-game-client
		},
		{
			name:     "No match",
			query:    "nonexistent",
			expected: 0,
		},
		{
			name:     "Case insensitive",
			query:    "MY-GAME",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := client.FindProjectsByName(tt.query)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(matches) != tt.expected {
				t.Errorf("Expected %d matches, got %d", tt.expected, len(matches))
			}
		})
	}
}

func TestGetProjectByName(t *testing.T) {
	projectsJSON := `{
		"schema_version": "v1",
		"data": {
			"/path/to/project-a": {
				"title": "project-a",
				"path": "/path/to/project-a",
				"version": "2022.3.60f1"
			},
			"/path/to/project-b": {
				"title": "project-b",
				"path": "/path/to/project-b",
				"version": "2022.3.60f1"
			},
			"/path/to/unique": {
				"title": "unique",
				"path": "/path/to/unique",
				"version": "6000.3.2f1"
			}
		}
	}`

	client := createTestClient(t, projectsJSON)

	t.Run("Single match", func(t *testing.T) {
		project, err := client.GetProjectByName("unique")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
			return
		}
		if project.Title != "unique" {
			t.Errorf("Expected title 'unique', got '%s'", project.Title)
		}
	})

	t.Run("Multiple matches returns error", func(t *testing.T) {
		_, err := client.GetProjectByName("project")
		if err == nil {
			t.Error("Expected error for multiple matches, got nil")
			return
		}

		var multiErr *MultipleMatchError
		if !errors.As(err, &multiErr) {
			t.Errorf("Expected MultipleMatchError, got %T", err)
			return
		}

		if len(multiErr.Matches) != 2 {
			t.Errorf("Expected 2 matches in error, got %d", len(multiErr.Matches))
		}
	})

	t.Run("No match returns error", func(t *testing.T) {
		_, err := client.GetProjectByName("nonexistent")
		if err == nil {
			t.Error("Expected error for no match, got nil")
		}
	})
}

func TestGetProjectByIndex(t *testing.T) {
	projectsJSON := `{
		"schema_version": "v1",
		"data": {
			"/path/to/project1": {
				"title": "project1",
				"path": "/path/to/project1",
				"version": "2022.3.60f1"
			},
			"/path/to/project2": {
				"title": "project2",
				"path": "/path/to/project2",
				"version": "6000.3.2f1"
			}
		}
	}`

	client := createTestClient(t, projectsJSON)

	t.Run("Valid index", func(t *testing.T) {
		project, err := client.GetProjectByIndex(1)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
			return
		}
		if project == nil {
			t.Error("Expected project, got nil")
		}
	})

	t.Run("Index 0 is invalid", func(t *testing.T) {
		_, err := client.GetProjectByIndex(0)
		if err == nil {
			t.Error("Expected error for index 0, got nil")
		}
	})

	t.Run("Index out of range", func(t *testing.T) {
		_, err := client.GetProjectByIndex(100)
		if err == nil {
			t.Error("Expected error for out of range index, got nil")
		}
	})
}

func TestGetProject(t *testing.T) {
	projectsJSON := `{
		"schema_version": "v1",
		"data": {
			"/path/to/my-project": {
				"title": "my-project",
				"path": "/path/to/my-project",
				"version": "2022.3.60f1"
			}
		}
	}`

	client := createTestClient(t, projectsJSON)

	t.Run("By index", func(t *testing.T) {
		project, err := client.GetProject("1")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
			return
		}
		if project == nil {
			t.Error("Expected project, got nil")
		}
	})

	t.Run("By name", func(t *testing.T) {
		project, err := client.GetProject("my-project")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
			return
		}
		if project.Title != "my-project" {
			t.Errorf("Expected title 'my-project', got '%s'", project.Title)
		}
	})
}

func TestMultipleMatchError(t *testing.T) {
	err := &MultipleMatchError{
		Query: "test",
		Matches: []ProjectInfo{
			{Title: "test1"},
			{Title: "test2"},
		},
	}

	if err.Error() != "multiple projects match 'test': found 2" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}
}

func TestProjectInfoFields(t *testing.T) {
	projectsJSON := `{
		"schema_version": "v1",
		"data": {
			"/path/to/project": {
				"title": "My Project",
				"path": "/path/to/project",
				"version": "2022.3.60f1",
				"lastModified": 1700000000000
			}
		}
	}`

	client := createTestClient(t, projectsJSON)

	projects, err := client.ListProjects()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(projects))
	}

	p := projects[0]
	if p.Title != "My Project" {
		t.Errorf("Expected title 'My Project', got '%s'", p.Title)
	}
	if p.Path != "/path/to/project" {
		t.Errorf("Expected path '/path/to/project', got '%s'", p.Path)
	}
	if p.Version != "2022.3.60f1" {
		t.Errorf("Expected version '2022.3.60f1', got '%s'", p.Version)
	}
	if p.LastModified.IsZero() {
		t.Error("Expected non-zero LastModified")
	}
}

func TestListProjectsEmptyFile(t *testing.T) {
	projectsJSON := `{"schema_version": "v1", "data": {}}`
	client := createTestClient(t, projectsJSON)

	projects, err := client.ListProjects()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(projects) != 0 {
		t.Errorf("Expected 0 projects, got %d", len(projects))
	}
}

func TestListProjectsFileNotFound(t *testing.T) {
	client := &Client{projectsFileOverride: "/nonexistent/path/projects-v1.json"}

	projects, err := client.ListProjects()
	if err != nil {
		t.Fatalf("Unexpected error for missing file: %v", err)
	}

	if len(projects) != 0 {
		t.Errorf("Expected 0 projects for missing file, got %d", len(projects))
	}
}

func TestTitleFallbackToDirectoryName(t *testing.T) {
	// Project without title
	projectsJSON := `{
		"schema_version": "v1",
		"data": {
			"/path/to/my-project-dir": {
				"path": "/path/to/my-project-dir",
				"version": "2022.3.60f1"
			}
		}
	}`

	client := createTestClient(t, projectsJSON)

	projects, err := client.ListProjects()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(projects))
	}

	// Title should fall back to directory name
	if projects[0].Title != "my-project-dir" {
		t.Errorf("Expected title 'my-project-dir', got '%s'", projects[0].Title)
	}
}
