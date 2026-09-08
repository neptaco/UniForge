package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Transport represents the IPC transport type.
type Transport string

const (
	// TransportUnix is a Unix domain socket.
	TransportUnix Transport = "unix"
	// TransportNamedPipe is a Windows named pipe.
	TransportNamedPipe Transport = "namedPipe"
	// TransportTCP is a TCP socket (for cross-machine or testing).
	TransportTCP Transport = "tcp"
)

// Info is the daemon's advertised state, persisted to daemon.json.
// Application-specific fields (e.g., protocol version) belong in Metadata,
// not as top-level fields—this keeps daemon discovery free of app-layer concerns.
type Info struct {
	PID       int             `json:"pid"`
	Transport Transport       `json:"transport"`
	Endpoint  string          `json:"endpoint,omitempty"`
	Host      string          `json:"host,omitempty"`
	Port      int             `json:"port,omitempty"`
	StartedAt int64           `json:"startedAt"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// ReadInfo reads the daemon info file for the given config.
// Returns os.ErrNotExist if no daemon info file exists.
func ReadInfo(config Config) (*Info, error) {
	path, err := config.infoPath()
	if err != nil {
		return nil, err
	}

	var content []byte
	readDeadline := time.Now().Add(2 * time.Second)
	for {
		content, err = os.ReadFile(path)
		if err == nil || runtime.GOOS != "windows" || errors.Is(err, os.ErrNotExist) || time.Now().After(readDeadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		return nil, err
	}

	var info Info
	if err := json.Unmarshal(content, &info); err != nil {
		return nil, fmt.Errorf("parse daemon info: %w", err)
	}
	return &info, nil
}

func writeInfo(config Config, info Info) error {
	if err := prepareRuntimeDir(config); err != nil {
		return err
	}

	path, err := config.infoPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".daemon-info-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Rename(tempPath, path)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := os.Rename(tempPath, path)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func removeInfoIfPID(config Config, pid int) error {
	info, err := ReadInfo(config)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.PID != pid {
		return nil
	}
	return removeInfo(config)
}

func removeInfo(config Config) error {
	path, err := config.infoPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
