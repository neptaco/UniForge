package unity

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Project struct {
	Path         string
	UnityVersion string
	Changeset    string
	Name         string
}

func LoadProject(projectPath string) (*Project, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	versionFile := filepath.Join(absPath, "ProjectSettings", "ProjectVersion.txt")
	if _, err := os.Stat(versionFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a Unity project: ProjectVersion.txt not found at %s", versionFile)
	}

	version, changeset, err := readUnityVersionWithChangeset(versionFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read Unity version: %w", err)
	}

	return &Project{
		Path:         absPath,
		UnityVersion: version,
		Changeset:    changeset,
		Name:         filepath.Base(absPath),
	}, nil
}

func readUnityVersion(versionFile string) (string, error) {
	version, _, err := readUnityVersionWithChangeset(versionFile)
	return version, err
}

func readUnityVersionWithChangeset(versionFile string) (string, string, error) {
	file, err := os.Open(versionFile)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = file.Close() }()

	var version, changeset string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Parse version
		if strings.HasPrefix(line, "m_EditorVersion:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				version = strings.TrimSpace(parts[1])
			}
		}

		// Parse version with changeset
		// Format: m_EditorVersionWithRevision: 2022.3.10f1 (ff3792e53c62)
		if strings.HasPrefix(line, "m_EditorVersionWithRevision:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				revision := strings.TrimSpace(parts[1])
				// Extract changeset from parentheses
				if idx := strings.Index(revision, "("); idx > 0 {
					if idx2 := strings.Index(revision, ")"); idx2 > idx {
						changeset = strings.TrimSpace(revision[idx+1 : idx2])
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", "", err
	}

	if version == "" {
		return "", "", fmt.Errorf("m_EditorVersion not found in ProjectVersion.txt")
	}

	return version, changeset, nil
}
