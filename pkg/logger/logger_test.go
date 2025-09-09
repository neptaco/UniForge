package logger

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	t.Run("With log file", func(t *testing.T) {
		tempDir := t.TempDir()
		logFile := filepath.Join(tempDir, "test.log")
		
		logger := New(logFile, false)
		defer logger.Close()
		
		if logger.file == nil {
			t.Error("Expected file to be opened")
		}
		
		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			t.Error("Log file was not created")
		}
	})
	
	t.Run("Without log file", func(t *testing.T) {
		logger := New("-", false)
		defer logger.Close()
		
		if logger.file != nil {
			t.Error("Expected file to be nil")
		}
	})
	
	t.Run("CI mode", func(t *testing.T) {
		logger := New("-", true)
		defer logger.Close()
		
		if !logger.ciMode {
			t.Error("Expected CI mode to be enabled")
		}
	})
}

func TestLoggerProcessLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		ciMode   bool
		wantWarn bool
		wantErr  bool
	}{
		{
			name:     "Warning line",
			line:     "Warning: Something is not optimal",
			ciMode:   false,
			wantWarn: true,
			wantErr:  false,
		},
		{
			name:     "Error line",
			line:     "Error: Something went wrong",
			ciMode:   false,
			wantWarn: false,
			wantErr:  true,
		},
		{
			name:     "Failure line",
			line:     "Build failed",
			ciMode:   false,
			wantWarn: false,
			wantErr:  true,
		},
		{
			name:     "Normal line",
			line:     "Processing file...",
			ciMode:   false,
			wantWarn: false,
			wantErr:  false,
		},
		{
			name:     "Warning in CI mode",
			line:     "warn: deprecated feature",
			ciMode:   true,
			wantWarn: true,
			wantErr:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := &Logger{
				writer: &buf,
				ciMode: tt.ciMode,
			}
			
			logger.processLine(tt.line)
			
			if tt.wantWarn && logger.warnings != 1 {
				t.Errorf("Expected 1 warning, got %d", logger.warnings)
			}
			
			if tt.wantErr && logger.errors != 1 {
				t.Errorf("Expected 1 error, got %d", logger.errors)
			}
			
			output := buf.String()
			if output == "" {
				t.Error("Expected some output")
			}
			
			if tt.ciMode {
				if tt.wantWarn && !strings.Contains(output, "::warning::") {
					t.Error("Expected CI warning format")
				}
				if tt.wantErr && !strings.Contains(output, "::error::") {
					t.Error("Expected CI error format")
				}
			} else {
				if tt.wantWarn && !strings.Contains(output, "[WARN]") {
					t.Error("Expected warning tag")
				}
				if tt.wantErr && !strings.Contains(output, "[ERROR]") {
					t.Error("Expected error tag")
				}
			}
		})
	}
}

func TestLoggerStats(t *testing.T) {
	logger := &Logger{}
	
	logger.warnings = 5
	logger.errors = 3
	
	if !logger.HasWarnings() {
		t.Error("HasWarnings() should return true")
	}
	
	if !logger.HasErrors() {
		t.Error("HasErrors() should return true")
	}
	
	warnings, errors := logger.GetStats()
	if warnings != 5 {
		t.Errorf("Expected 5 warnings, got %d", warnings)
	}
	if errors != 3 {
		t.Errorf("Expected 3 errors, got %d", errors)
	}
}

func TestLoggerWrite(t *testing.T) {
	var buf bytes.Buffer
	logger := &Logger{
		writer: &buf,
		ciMode: false,
	}
	
	logger.pipeReader, logger.pipeWriter = io.Pipe()
	go logger.processLogs()
	
	message := []byte("Test message\n")
	n, err := logger.Write(message)
	
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	
	if n != len(message) {
		t.Errorf("Write() wrote %d bytes, want %d", n, len(message))
	}
	
	time.Sleep(100 * time.Millisecond)
	logger.Close()
	
	output := buf.String()
	if !strings.Contains(output, "Test message") {
		t.Error("Expected message not found in output")
	}
}

func TestLoggerClose(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	
	logger := New(logFile, false)
	logger.warnings = 2
	logger.errors = 1
	
	var buf bytes.Buffer
	logger.writer = &buf
	
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
	
	output := buf.String()
	if !strings.Contains(output, "Build Summary") {
		t.Error("Expected build summary in output")
	}
	if !strings.Contains(output, "Warnings: 2") {
		t.Error("Expected warning count in summary")
	}
	if !strings.Contains(output, "Errors: 1") {
		t.Error("Expected error count in summary")
	}
}