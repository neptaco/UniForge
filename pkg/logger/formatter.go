package logger

import (
	"fmt"
	"regexp"
	"strings"
)

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
	ColorGreen  = "\033[32m"
	ColorGray   = "\033[90m"
	ColorBold   = "\033[1m"
)

// LogLevel represents the type of log line
type LogLevel int

const (
	LogLevelNormal LogLevel = iota
	LogLevelInfo
	LogLevelWarning
	LogLevelError
	LogLevelStackTrace
	LogLevelNoise
)

// Default max line length before truncation
const DefaultMaxLineLength = 500

// Formatter handles Unity log formatting with colors and filtering
type Formatter struct {
	noColor            bool
	hideStackTrace     bool     // Hide non-project stack traces
	hideAllStackTraces bool     // Hide all stack traces completely
	maxLineLength      int      // Max line length before truncation (0 = no limit)
	projectPaths       []string // Paths to keep in stack traces (e.g., "Assets/")
}

// FormatterOption configures a Formatter
type FormatterOption func(*Formatter)

// WithNoColor disables color output
func WithNoColor(noColor bool) FormatterOption {
	return func(f *Formatter) {
		f.noColor = noColor
	}
}

// WithHideStackTrace hides non-project stack trace lines
func WithHideStackTrace(hide bool) FormatterOption {
	return func(f *Formatter) {
		f.hideStackTrace = hide
	}
}

// WithHideAllStackTraces hides all stack traces completely
func WithHideAllStackTraces(hide bool) FormatterOption {
	return func(f *Formatter) {
		f.hideAllStackTraces = hide
	}
}

// WithMaxLineLength sets the maximum line length before truncation
func WithMaxLineLength(length int) FormatterOption {
	return func(f *Formatter) {
		f.maxLineLength = length
	}
}

// WithProjectPaths sets paths to keep in stack traces
func WithProjectPaths(paths []string) FormatterOption {
	return func(f *Formatter) {
		f.projectPaths = paths
	}
}

// NewFormatter creates a new Formatter
func NewFormatter(opts ...FormatterOption) *Formatter {
	f := &Formatter{
		projectPaths:  []string{"Assets/", "Packages/"},
		maxLineLength: DefaultMaxLineLength,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Noise patterns - things we want to dim
var noisePatterns = []string{
	"Mono path[",
	"Loading GUID",
	"Refreshing native plugins",
	"Preloading",
	"GI:",
	"Initialize engine version",
	"Compiling shader",
	"Shader warmup",
	"UnloadTime:",
	"DisplayProgressbar:",
	"Registering precompiled user dll",
	"Native extension for",
	"- Completed reload",
	"- Starting playmode",
	"Reloading assemblies for play mode",
	"Begin MonoManager ReloadAssembly",
	"Native extension for",
	"Initializing Unity.PackageManager",
	"[Package Manager]",
	"[Licensing::",
	"Domain Reload Profiling:",
	"Total time for reloading assemblies",
	"Launched and calculation",
}

// Error patterns
var errorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\berror\b`),
	regexp.MustCompile(`(?i)exception\b`),
	regexp.MustCompile(`(?i)\bfailed\b`),
	regexp.MustCompile(`(?i)^error CS\d+`),
	regexp.MustCompile(`(?i)^Assets/.*\.cs\(\d+,\d+\):\s*error`),
}

// Warning patterns
var warningPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bwarning\b`),
	regexp.MustCompile(`(?i)^warning CS\d+`),
	regexp.MustCompile(`(?i)^Assets/.*\.cs\(\d+,\d+\):\s*warning`),
}

// Stack trace patterns (applied after TrimSpace)
var stackTracePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^at\s+`),                            // "at UnityEngine.Debug.Log..."
	regexp.MustCompile(`^\(Filename:`),                      // "(Filename: Assets/..."
	regexp.MustCompile(`^UnityEngine\.\w+.*:`),              // "UnityEngine.Debug:Log..."
	regexp.MustCompile(`^UnityEditor\.\w+.*:`),              // "UnityEditor.Menu:..."
	regexp.MustCompile(`^System\.\w+`),                      // "System.Threading.ExecutionContext:..."
	regexp.MustCompile(`^Mono\.\w+`),                        // "Mono.Security..."
	regexp.MustCompile(`^Microsoft\.\w+`),                   // "Microsoft.CSharp..."
	regexp.MustCompile(`^\w+\.\w+[^:]*:[^(]+\(.*\)$`),       // "MyClass.Method:Call (args)" - no (at ...)
	regexp.MustCompile(`^\w+\.\w+[^:]*:[^(]+\(.*\)\s*\(at`), // "MyClass.Method:Call<T> (args) (at Assets/..."
	regexp.MustCompile(`^\w+\.\w+/<>.*:.*\(.*\)`),           // "Class/<>c__DisplayClass:Method ()" - lambda
	regexp.MustCompile(`^in\s+<`),                           // "in <filename unknown>"
	regexp.MustCompile(`^\[0x[0-9a-f]+\]`),                  // "[0x00000] in ..."
	regexp.MustCompile(`^Rethrow as \w+:`),                  // "Rethrow as TargetInvocationException:"
}

// ClassifyLine determines the log level of a line
func (f *Formatter) ClassifyLine(line string) LogLevel {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return LogLevelNormal
	}

	// Check for noise FIRST (so [Licensing::] etc. are always gray even if they contain "error")
	for _, noise := range noisePatterns {
		if strings.Contains(trimmed, noise) {
			return LogLevelNoise
		}
	}

	// Check for stack trace
	for _, pattern := range stackTracePatterns {
		if pattern.MatchString(trimmed) {
			return LogLevelStackTrace
		}
	}

	// Check for error
	for _, pattern := range errorPatterns {
		if pattern.MatchString(trimmed) {
			return LogLevelError
		}
	}

	// Check for warning
	for _, pattern := range warningPatterns {
		if pattern.MatchString(trimmed) {
			return LogLevelWarning
		}
	}

	return LogLevelNormal
}

// Non-project stack trace prefixes (always filter out)
var nonProjectPrefixes = []string{
	"System.",
	"UnityEngine.",
	"UnityEditor.",
	"Mono.",
	"Microsoft.",
	"Cysharp.",
}

// Non-project paths in stack traces (filter out)
var nonProjectPaths = []string{
	"Library/PackageCache/",
	"./Library/PackageCache/",
}

// IsProjectStackTrace checks if a stack trace line is from the project
func (f *Formatter) IsProjectStackTrace(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Always filter out known non-project prefixes
	for _, prefix := range nonProjectPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return false
		}
	}

	// Filter out non-project paths (Library/PackageCache, etc.)
	for _, path := range nonProjectPaths {
		if strings.Contains(line, path) {
			return false
		}
	}

	// Check if it contains project paths
	for _, path := range f.projectPaths {
		if strings.Contains(line, path) {
			return true
		}
	}

	// (Filename: ...) lines - check the path inside
	if strings.HasPrefix(trimmed, "(Filename:") {
		for _, path := range f.projectPaths {
			if strings.Contains(line, path) {
				return true
			}
		}
		return false
	}

	return false
}

// truncateLine truncates a line if it exceeds maxLineLength
func (f *Formatter) truncateLine(line string) string {
	if f.maxLineLength > 0 && len(line) > f.maxLineLength {
		return line[:f.maxLineLength] + "..."
	}
	return line
}

// FormatLine formats a log line with appropriate colors
func (f *Formatter) FormatLine(line string) string {
	level := f.ClassifyLine(line)

	// Handle stack trace filtering
	if level == LogLevelStackTrace {
		if f.hideStackTrace && !f.IsProjectStackTrace(line) {
			return "" // Hide this line
		}
	}

	// Truncate long lines
	line = f.truncateLine(line)

	if f.noColor {
		return line
	}

	switch level {
	case LogLevelError:
		return fmt.Sprintf("%s%s%s%s", ColorBold, ColorRed, line, ColorReset)
	case LogLevelWarning:
		return fmt.Sprintf("%s%s%s", ColorYellow, line, ColorReset)
	case LogLevelStackTrace:
		return fmt.Sprintf("%s%s%s", ColorGray, line, ColorReset)
	case LogLevelNoise:
		return fmt.Sprintf("%s%s%s", ColorGray, line, ColorReset)
	default:
		return line
	}
}

// ShouldShow returns whether the line should be displayed
func (f *Formatter) ShouldShow(line string) bool {
	// Hide empty lines
	if strings.TrimSpace(line) == "" {
		return false
	}

	level := f.ClassifyLine(line)
	if level == LogLevelStackTrace {
		if f.hideAllStackTraces {
			return false // Hide all stack traces
		}
		if f.hideStackTrace {
			return f.IsProjectStackTrace(line)
		}
	}
	return true
}
