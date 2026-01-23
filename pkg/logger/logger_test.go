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

func TestFormatterClassifyLine(t *testing.T) {
	formatter := NewFormatter()

	tests := []struct {
		name     string
		line     string
		expected LogLevel
	}{
		{
			name:     "Error line",
			line:     "Error: Something went wrong",
			expected: LogLevelError,
		},
		{
			name:     "Exception line",
			line:     "NullReferenceException: Object reference not set",
			expected: LogLevelError,
		},
		{
			name:     "Warning line",
			line:     "Warning: Something is not optimal",
			expected: LogLevelWarning,
		},
		{
			name:     "Stack trace line",
			line:     "System.Threading.ExecutionContext:RunInternal ()",
			expected: LogLevelStackTrace,
		},
		{
			name:     "Unity stack trace",
			line:     "UnityEngine.Debug:Log (System.Object)",
			expected: LogLevelStackTrace,
		},
		{
			name:     "Project stack trace without at",
			line:     "UnityMCPBridge.WebSocketClient:ScheduleReconnectAsync ()",
			expected: LogLevelStackTrace,
		},
		{
			name:     "Project stack trace with at",
			line:     "UnityMCPBridge.MCPBridgeService:OnError (string) (at Assets/Editor/UnityMCPBridge/MCPBridgeService.cs:384)",
			expected: LogLevelStackTrace,
		},
		{
			name:     "Lambda stack trace",
			line:     "UnityMCPBridge.WebSocketClient/<>c__DisplayClass53_0:<ConnectAsync>b__1 () (at Assets/Editor/UnityMCPBridge/WebSocketClient.cs:152)",
			expected: LogLevelStackTrace,
		},
		{
			name:     "Noise line",
			line:     "Mono path[0] = '/Applications/Unity'",
			expected: LogLevelNoise,
		},
		{
			name:     "Normal line",
			line:     "Processing file...",
			expected: LogLevelNormal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := formatter.ClassifyLine(tt.line)
			if level != tt.expected {
				t.Errorf("ClassifyLine(%q) = %v, want %v", tt.line, level, tt.expected)
			}
		})
	}
}

func TestFormatterFormatLine(t *testing.T) {
	formatter := NewFormatter(WithNoColor(false))

	tests := []struct {
		name        string
		line        string
		shouldColor bool
	}{
		{
			name:        "Error gets red",
			line:        "Error: test",
			shouldColor: true,
		},
		{
			name:        "Warning gets yellow",
			line:        "Warning: test",
			shouldColor: true,
		},
		{
			name:        "Normal no color",
			line:        "Normal line",
			shouldColor: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := formatter.FormatLine(tt.line)
			hasColor := strings.Contains(formatted, "\033[")
			if hasColor != tt.shouldColor {
				t.Errorf("FormatLine(%q) hasColor=%v, want %v", tt.line, hasColor, tt.shouldColor)
			}
		})
	}
}

func TestFormatterNoColor(t *testing.T) {
	formatter := NewFormatter(WithNoColor(true))

	formatted := formatter.FormatLine("Error: test")
	if strings.Contains(formatted, "\033[") {
		t.Error("Expected no color codes when noColor is true")
	}
}

func TestFormatterStackTraceFiltering(t *testing.T) {
	formatter := NewFormatter(WithHideStackTrace(true))

	tests := []struct {
		name       string
		line       string
		shouldShow bool
	}{
		{
			name:       "Project stack trace with Assets path",
			line:       "MyScript:Start () (at Assets/Scripts/MyScript.cs:10)",
			shouldShow: true,
		},
		{
			name:       "Unity internal stack trace",
			line:       "UnityEngine.Debug:Log (System.Object)",
			shouldShow: false,
		},
		{
			name:       "System stack trace",
			line:       "System.Threading.ExecutionContext:RunInternal ()",
			shouldShow: false,
		},
		{
			name:       "Normal line",
			line:       "Processing...",
			shouldShow: true,
		},
		{
			name:       "Filename line with Assets",
			line:       "(Filename: Assets/Editor/MyScript.cs Line: 123)",
			shouldShow: true,
		},
		{
			name:       "Filename line with Unity internal",
			line:       "(Filename: /Users/bokken/build/output/unity/unity/Runtime/Export/Scripting/UnitySynchronizationContext.cs Line: 153)",
			shouldShow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldShow := formatter.ShouldShow(tt.line)
			if shouldShow != tt.shouldShow {
				t.Errorf("ShouldShow(%q) = %v, want %v", tt.line, shouldShow, tt.shouldShow)
			}
		})
	}
}

func TestLoggerStats(t *testing.T) {
	logger := &Logger{
		formatter: NewFormatter(),
	}

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
		writer:    &buf,
		ciMode:    false,
		formatter: NewFormatter(WithNoColor(true)),
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
}

func TestLoggerClose(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	logger := New(logFile, false)
	logger.warnings = 2
	logger.errors = 1

	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
