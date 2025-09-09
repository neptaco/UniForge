package unity

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/neptaco/unity-cli/pkg/hub"
)

type Editor struct {
	Version string
	Path    string
}

func NewEditor(version string) *Editor {
	return &Editor{
		Version: version,
	}
}

func (e *Editor) GetPath() (string, error) {
	if e.Path != "" {
		return e.Path, nil
	}

	hubClient := hub.NewClient()
	editors, err := hubClient.ListInstalledEditors()
	if err != nil {
		return "", fmt.Errorf("failed to list editors: %w", err)
	}

	for _, editor := range editors {
		if editor.Version == e.Version {
			e.Path = e.getExecutablePath(editor.Path)
			return e.Path, nil
		}
	}

	return "", fmt.Errorf("Unity Editor %s not found. Please install it using: unity-cli install --version %s", e.Version, e.Version)
}

func (e *Editor) getExecutablePath(installPath string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(installPath, "Unity.app", "Contents", "MacOS", "Unity")
	case "windows":
		return filepath.Join(installPath, "Editor", "Unity.exe")
	case "linux":
		return filepath.Join(installPath, "Editor", "Unity")
	default:
		return filepath.Join(installPath, "Unity")
	}
}

func (e *Editor) Exists() bool {
	path, err := e.GetPath()
	if err != nil {
		return false
	}
	
	_, err = os.Stat(path)
	return err == nil
}