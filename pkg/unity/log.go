package unity

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// GetEditorLogPath returns the platform-specific path to Unity Editor log
func GetEditorLogPath() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(home, "Library", "Logs", "Unity", "Editor.log"), nil

	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			return "", fmt.Errorf("LOCALAPPDATA environment variable not set")
		}
		return filepath.Join(localAppData, "Unity", "Editor", "Editor.log"), nil

	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(home, ".config", "unity3d", "Editor.log"), nil

	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}
