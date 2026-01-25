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
		defer func() { _ = logger.Close() }()

		if logger.file == nil {
			t.Error("Expected file to be opened")
		}

		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			t.Error("Log file was not created")
		}
	})

	t.Run("Without log file", func(t *testing.T) {
		logger := New("-", false)
		defer func() { _ = logger.Close() }()

		if logger.file != nil {
			t.Error("Expected file to be nil")
		}
	})

	t.Run("CI mode", func(t *testing.T) {
		logger := New("-", true)
		defer func() { _ = logger.Close() }()

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

func TestFormatterGetNoiseCategory(t *testing.T) {
	formatter := NewFormatter()

	tests := []struct {
		name     string
		line     string
		expected NoiseCategory
	}{
		{
			name:     "Licensing log",
			line:     "[Licensing::Module] Successfully connected to LicensingClient",
			expected: NoiseCategoryLicensing,
		},
		{
			name:     "Package Manager log",
			line:     "[Package Manager] Registered 73 packages",
			expected: NoiseCategoryPackage,
		},
		{
			name:     "Memory configuration",
			line:     "memorysetup-bucket-allocator-granularity=16",
			expected: NoiseCategoryMemory,
		},
		{
			name:     "UnityMemory log",
			line:     "[UnityMemory] Configuration Parameters",
			expected: NoiseCategoryMemory,
		},
		{
			name:     "Assembly reload",
			line:     "Begin MonoManager ReloadAssembly",
			expected: NoiseCategoryAssembly,
		},
		{
			name:     "Domain reload profiling",
			line:     "Domain Reload Profiling: 10945ms",
			expected: NoiseCategoryAssembly,
		},
		{
			name:     "gRPC log",
			line:     "info: Microsoft.AspNetCore.Hosting.Diagnostics[1]",
			expected: NoiseCategoryGRPC,
		},
		{
			name:     "Subsystems log",
			line:     "[Subsystems] Discovering subsystems at path",
			expected: NoiseCategorySubsystems,
		},
		{
			name:     "Other noise - Mono path",
			line:     "Mono path[0] = '/Applications/Unity'",
			expected: NoiseCategoryOther,
		},
		{
			name:     "Other noise - PhysX",
			line:     "[PhysX] Initialized MultithreadedTaskDispatcher",
			expected: NoiseCategoryOther,
		},
		{
			name:     "Normal log - not noise",
			line:     "Build completed successfully",
			expected: NoiseCategoryNone,
		},
		{
			name:     "Error log - not noise",
			line:     "Error: Something went wrong",
			expected: NoiseCategoryNone,
		},
		{
			name:     "Asset Pipeline Refresh",
			line:     "Application.AssetDatabase Initial Refresh Start",
			expected: NoiseCategoryAssetImport,
		},
		{
			name:     "Shader compilation",
			line:     "Compiling shader 'Hidden/BlitCopy'",
			expected: NoiseCategoryShader,
		},
		{
			name:     "Mesh data optimization",
			line:     "Compiling mesh data optimization",
			expected: NoiseCategoryShader,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := formatter.GetNoiseCategory(tt.line)
			if category != tt.expected {
				t.Errorf("GetNoiseCategory(%q) = %v, want %v", tt.line, category, tt.expected)
			}
		})
	}
}

func TestFormatterNoisePriority(t *testing.T) {
	formatter := NewFormatter()

	// Noise logs containing "error" keyword should still be classified as Noise, not Error
	tests := []struct {
		name     string
		line     string
		expected LogLevel
	}{
		{
			name:     "Licensing error is noise, not error",
			line:     "[Licensing::Module] Error: Access token is unavailable",
			expected: LogLevelNoise,
		},
		{
			name:     "Licensing client error is noise",
			line:     "[Licensing::Client] Error: Code 500 while processing request",
			expected: LogLevelNoise,
		},
		{
			name:     "Real error without noise pattern",
			line:     "Error: Build failed",
			expected: LogLevelError,
		},
		{
			name:     "Exception in filename is not error",
			line:     " 0.1 kb     0.0% Packages/com.unity.purchasing/Runtime/Purchasing/Core/Exceptions/StoreCreationException.cs",
			expected: LogLevelNormal,
		},
		{
			name:     "Build size report with Exception path is not error",
			line:     "Error:  0.1 kb     0.0% Packages/com.unity.purchasing/Runtime/Purchasing/Core/Exceptions/StoreCreationException.cs",
			expected: LogLevelNormal,
		},
		{
			name:     "Real exception is error",
			line:     "NullReferenceException: Object reference not set to an instance of an object",
			expected: LogLevelError,
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
	_ = logger.Close()
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
