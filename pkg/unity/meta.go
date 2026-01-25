package unity

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MetaCheckResult holds the result of meta file checking
type MetaCheckResult struct {
	MissingMeta    []string            // Assets without .meta files
	OrphanMeta     []string            // .meta files without corresponding assets
	DuplicateGUIDs map[string][]string // GUID -> list of files with that GUID
}

// HasErrors returns true if there are any errors (missing meta or duplicate GUIDs)
func (r *MetaCheckResult) HasErrors() bool {
	return len(r.MissingMeta) > 0 || len(r.DuplicateGUIDs) > 0
}

// HasWarnings returns true if there are any warnings (orphan meta)
func (r *MetaCheckResult) HasWarnings() bool {
	return len(r.OrphanMeta) > 0
}

// MetaChecker checks Unity project meta file integrity
type MetaChecker struct {
	project *Project
}

// NewMetaChecker creates a new MetaChecker
func NewMetaChecker(project *Project) *MetaChecker {
	return &MetaChecker{
		project: project,
	}
}

// excludedDirs are directories that should be excluded from meta checking
var excludedDirs = map[string]bool{
	".git":            true,
	"Library":         true,
	"Temp":            true,
	"Logs":            true,
	"obj":             true,
	"Build":           true,
	"Builds":          true,
	"UserSettings":    true,
	"ProjectSettings": true, // ProjectSettings files don't need .meta
}

// excludedFiles are files that should be excluded from meta checking
var excludedFiles = map[string]bool{
	".gitignore":         true,
	".gitattributes":     true,
	".DS_Store":          true,
	"Thumbs.db":          true,
	"manifest.json":      true, // Packages/manifest.json
	"packages-lock.json": true, // Packages/packages-lock.json
}

// metaRequiredRoots are root directories where .meta files are required
var metaRequiredRoots = map[string]bool{
	"Assets":   true,
	"Packages": true,
}

// Check performs meta file integrity check
func (c *MetaChecker) Check() (*MetaCheckResult, error) {
	result := &MetaCheckResult{
		MissingMeta:    []string{},
		OrphanMeta:     []string{},
		DuplicateGUIDs: make(map[string][]string),
	}

	// Track all assets and meta files
	assets := make(map[string]bool)  // asset path -> exists
	metas := make(map[string]bool)   // meta path -> exists
	guids := make(map[string]string) // GUID -> first file path

	// Walk the project directory
	err := filepath.Walk(c.project.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path from project root
		relPath, err := filepath.Rel(c.project.Path, path)
		if err != nil {
			return err
		}

		// Skip root
		if relPath == "." {
			return nil
		}

		// Check if we should skip this directory
		if info.IsDir() {
			baseName := filepath.Base(path)
			if excludedDirs[baseName] {
				return filepath.SkipDir
			}
			// Only track directories inside Assets/ or Packages/
			if isInsideMetaRequiredRoot(relPath) {
				assets[relPath] = true
			}
			return nil
		}

		// Skip files not inside Assets/ or Packages/
		if !isInsideMetaRequiredRoot(relPath) {
			return nil
		}

		// Check if file should be excluded
		baseName := filepath.Base(path)
		if excludedFiles[baseName] {
			return nil
		}

		// Track meta files and assets separately
		if strings.HasSuffix(path, ".meta") {
			metas[relPath] = true

			// Extract GUID from meta file
			guid, err := extractGUID(path)
			if err == nil && guid != "" {
				if existingPath, exists := guids[guid]; exists {
					// Duplicate GUID found
					if _, ok := result.DuplicateGUIDs[guid]; !ok {
						result.DuplicateGUIDs[guid] = []string{existingPath}
					}
					result.DuplicateGUIDs[guid] = append(result.DuplicateGUIDs[guid], relPath)
				} else {
					guids[guid] = relPath
				}
			}
		} else {
			assets[relPath] = true
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk project directory: %w", err)
	}

	// Check for missing meta files
	for asset := range assets {
		metaPath := asset + ".meta"
		if !metas[metaPath] {
			result.MissingMeta = append(result.MissingMeta, asset)
		}
	}

	// Check for orphan meta files
	for meta := range metas {
		// Get asset path by removing .meta suffix
		assetPath := strings.TrimSuffix(meta, ".meta")
		if !assets[assetPath] {
			result.OrphanMeta = append(result.OrphanMeta, meta)
		}
	}

	return result, nil
}

// isInsideMetaRequiredRoot checks if the path is inside Assets/ or Packages/
// Returns true only for items inside these directories, not the directories themselves
func isInsideMetaRequiredRoot(relPath string) bool {
	parts := strings.SplitN(relPath, string(filepath.Separator), 2)
	// Need at least 2 parts: "Assets/something" or "Packages/something"
	if len(parts) < 2 {
		return false
	}
	return metaRequiredRoots[parts[0]]
}

// Fix removes orphan meta files
// Returns list of deleted files
func (c *MetaChecker) Fix(dryRun bool) ([]string, error) {
	result, err := c.Check()
	if err != nil {
		return nil, err
	}

	deleted := []string{}
	for _, orphan := range result.OrphanMeta {
		fullPath := filepath.Join(c.project.Path, orphan)
		if dryRun {
			deleted = append(deleted, orphan)
		} else {
			if err := os.Remove(fullPath); err != nil {
				return deleted, fmt.Errorf("failed to remove %s: %w", orphan, err)
			}
			deleted = append(deleted, orphan)
		}
	}

	return deleted, nil
}

// extractGUID reads a .meta file and extracts the GUID
func extractGUID(metaPath string) (string, error) {
	file, err := os.Open(metaPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "guid:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", scanner.Err()
}
