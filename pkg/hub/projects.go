package hub

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/neptaco/uniforge/pkg/ui"
)

// ProjectInfo represents a Unity project registered in Unity Hub
type ProjectInfo struct {
	Title        string
	Path         string
	Version      string
	LastModified time.Time
	GitBranch    string // Current git branch
	GitStatus    string // "clean", "dirty", or "N uncommitted"
}

// projectsFileData represents the structure of projects-v1.json
type projectsFileData struct {
	SchemaVersion string                  `json:"schema_version"`
	Data          map[string]projectEntry `json:"data"`
}

type projectEntry struct {
	Title        string `json:"title,omitempty"`
	Path         string `json:"path,omitempty"`
	Version      string `json:"version,omitempty"`
	LastModified int64  `json:"lastModified,omitempty"`
	CloudProject string `json:"cloudProjectId,omitempty"`
	ProjectName  string `json:"projectName,omitempty"`
}

// ListProjects returns all projects registered in Unity Hub
func (c *Client) ListProjects() ([]ProjectInfo, error) {
	projectsFilePath := c.getProjectsFilePath()
	if projectsFilePath == "" {
		return nil, fmt.Errorf("could not determine Unity Hub projects file path")
	}

	data, err := os.ReadFile(projectsFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []ProjectInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read projects file: %w", err)
	}

	var projectsData projectsFileData
	if err := json.Unmarshal(data, &projectsData); err != nil {
		return nil, fmt.Errorf("failed to parse projects file: %w", err)
	}

	var result []ProjectInfo
	for _, entry := range projectsData.Data {
		info := ProjectInfo{
			Path:    entry.Path,
			Title:   entry.Title,
			Version: entry.Version,
		}

		// Use directory name as title if not specified
		if info.Title == "" {
			info.Title = filepath.Base(entry.Path)
		}

		// Parse last modified timestamp (milliseconds since epoch)
		if entry.LastModified > 0 {
			info.LastModified = time.UnixMilli(entry.LastModified)
		}

		result = append(result, info)
	}

	return result, nil
}

// ListProjectsWithGit returns all projects with Git information
func (c *Client) ListProjectsWithGit() ([]ProjectInfo, error) {
	projects, err := c.ListProjects()
	if err != nil {
		return nil, err
	}

	for i := range projects {
		c.fillGitInfo(&projects[i])
	}

	return projects, nil
}

// MultipleMatchError is returned when multiple projects match the search query
type MultipleMatchError struct {
	Query    string
	Matches  []ProjectInfo
}

func (e *MultipleMatchError) Error() string {
	return fmt.Sprintf("multiple projects match '%s': found %d", e.Query, len(e.Matches))
}

// FindProjectsByName finds all projects matching the name (case-insensitive)
// Returns matches in priority order: exact match, prefix match, contains match
func (c *Client) FindProjectsByName(name string) ([]ProjectInfo, error) {
	projects, err := c.ListProjects()
	if err != nil {
		return nil, err
	}

	nameLower := strings.ToLower(name)

	// First try exact match
	var exact []ProjectInfo
	for _, p := range projects {
		if strings.ToLower(p.Title) == nameLower {
			exact = append(exact, p)
		}
	}
	if len(exact) > 0 {
		return exact, nil
	}

	// Then try prefix match
	var prefix []ProjectInfo
	for _, p := range projects {
		if strings.HasPrefix(strings.ToLower(p.Title), nameLower) {
			prefix = append(prefix, p)
		}
	}
	if len(prefix) > 0 {
		return prefix, nil
	}

	// Finally try contains
	var contains []ProjectInfo
	for _, p := range projects {
		if strings.Contains(strings.ToLower(p.Title), nameLower) {
			contains = append(contains, p)
		}
	}
	return contains, nil
}

// GetProjectByName finds a project by name (case-insensitive partial match)
// Returns MultipleMatchError if multiple projects match
func (c *Client) GetProjectByName(name string) (*ProjectInfo, error) {
	matches, err := c.FindProjectsByName(name)
	if err != nil {
		return nil, err
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("project not found: %s", name)
	}

	if len(matches) > 1 {
		return nil, &MultipleMatchError{Query: name, Matches: matches}
	}

	c.fillGitInfo(&matches[0])
	return &matches[0], nil
}

// GetProjectByIndex finds a project by index (1-based)
func (c *Client) GetProjectByIndex(index int) (*ProjectInfo, error) {
	projects, err := c.ListProjects()
	if err != nil {
		return nil, err
	}

	if index < 1 || index > len(projects) {
		return nil, fmt.Errorf("project index out of range: %d (1-%d)", index, len(projects))
	}

	p := projects[index-1]
	c.fillGitInfo(&p)
	return &p, nil
}

// GetProject finds a project by name or index
func (c *Client) GetProject(nameOrIndex string) (*ProjectInfo, error) {
	// Try as index first
	if index, err := strconv.Atoi(nameOrIndex); err == nil {
		return c.GetProjectByIndex(index)
	}

	// Otherwise treat as name
	return c.GetProjectByName(nameOrIndex)
}

// getProjectsFilePath returns the path to Unity Hub's projects-v1.json
func (c *Client) getProjectsFilePath() string {
	// Allow override for testing
	if c.projectsFileOverride != "" {
		return c.projectsFileOverride
	}

	var basePath string

	switch runtime.GOOS {
	case "darwin":
		basePath = filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "UnityHub")
	case "windows":
		basePath = filepath.Join(os.Getenv("APPDATA"), "UnityHub")
	case "linux":
		basePath = filepath.Join(os.Getenv("HOME"), ".config", "UnityHub")
	default:
		return ""
	}

	return filepath.Join(basePath, "projects-v1.json")
}

// fillGitInfo populates Git branch and status information for a project
func (c *Client) fillGitInfo(project *ProjectInfo) {
	gitDir := filepath.Join(project.Path, ".git")
	if !fileExists(gitDir) {
		project.GitBranch = ""
		project.GitStatus = ""
		return
	}

	// Get current branch
	cmd := exec.Command("git", "-C", project.Path, "rev-parse", "--abbrev-ref", "HEAD")
	if output, err := cmd.Output(); err == nil {
		project.GitBranch = strings.TrimSpace(string(output))
	}

	// Get status
	cmd = exec.Command("git", "-C", project.Path, "status", "--porcelain")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) == 1 && lines[0] == "" {
			project.GitStatus = "clean"
		} else {
			uncommitted := len(lines)
			project.GitStatus = fmt.Sprintf("%d uncommitted", uncommitted)
		}
	}

	// Check ahead/behind
	cmd = exec.Command("git", "-C", project.Path, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	if output, err := cmd.Output(); err == nil {
		parts := strings.Fields(strings.TrimSpace(string(output)))
		if len(parts) == 2 {
			behind, _ := strconv.Atoi(parts[0])
			ahead, _ := strconv.Atoi(parts[1])
			if ahead > 0 || behind > 0 {
				var status []string
				if ahead > 0 {
					status = append(status, fmt.Sprintf("%d ahead", ahead))
				}
				if behind > 0 {
					status = append(status, fmt.Sprintf("%d behind", behind))
				}
				// Combine with existing status
				if project.GitStatus == "clean" {
					project.GitStatus = strings.Join(status, ", ")
				} else {
					project.GitStatus = project.GitStatus + ", " + strings.Join(status, ", ")
				}
			}
		}
	}

	ui.Debug("Git info for project", "path", project.Path, "branch", project.GitBranch, "status", project.GitStatus)
}
