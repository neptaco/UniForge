package license

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Manager handles Unity license operations
type Manager struct {
	editorPath string
	timeout    time.Duration
}

// NewManager creates a new license Manager
func NewManager(editorPath string, timeoutSeconds int) *Manager {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 300 * time.Second // Default 5 minutes
	}
	return &Manager{
		editorPath: editorPath,
		timeout:    timeout,
	}
}

// ActivateOptions holds options for license activation
type ActivateOptions struct {
	Username string
	Password string
	Serial   string
}

// Activate activates Unity license
func (m *Manager) Activate(opts ActivateOptions) error {
	if opts.Username == "" {
		return fmt.Errorf("username is required")
	}
	if opts.Password == "" {
		return fmt.Errorf("password is required")
	}
	if opts.Serial == "" {
		return fmt.Errorf("serial key is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	args := []string{
		"-batchmode",
		"-quit",
		"-serial", opts.Serial,
		"-username", opts.Username,
		"-password", opts.Password,
	}

	cmd := exec.CommandContext(ctx, m.editorPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("activation timed out after %v", m.timeout)
		}
		return fmt.Errorf("activation failed: %w", err)
	}

	return nil
}

// Return returns the Unity license
func (m *Manager) Return() error {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	args := []string{
		"-batchmode",
		"-quit",
		"-returnlicense",
	}

	cmd := exec.CommandContext(ctx, m.editorPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("return timed out after %v", m.timeout)
		}
		return fmt.Errorf("license return failed: %w", err)
	}

	return nil
}

// Status represents the current license status
type Status struct {
	HasLicense  bool
	LicensePath string
}

// GetStatus checks the current license status
func GetStatus() (*Status, error) {
	licensePath := getLicenseFilePath()
	_, err := os.Stat(licensePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Status{
				HasLicense:  false,
				LicensePath: licensePath,
			}, nil
		}
		return nil, fmt.Errorf("failed to check license file: %w", err)
	}

	return &Status{
		HasLicense:  true,
		LicensePath: licensePath,
	}, nil
}

// getLicenseFilePath returns the Unity license file path for the current OS
func getLicenseFilePath() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Unity", "Unity_lic.ulf")
	case "windows":
		return filepath.Join("C:", "ProgramData", "Unity", "Unity_lic.ulf")
	case "linux":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "unity3d", "Unity", "Unity_lic.ulf")
	default:
		return ""
	}
}
