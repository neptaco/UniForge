package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Logger struct {
	file         *os.File
	writer       io.Writer
	rawWriter    io.Writer // For file output without colors
	ciMode       bool
	warnings     int
	errors       int
	mutex        sync.Mutex
	pipeReader   *io.PipeReader
	pipeWriter   *io.PipeWriter
	formatter    *Formatter
	showTime     bool
	currentGroup NoiseCategory // Current active group in CI mode
}

type LoggerOption func(*Logger)

func WithCIMode(ci bool) LoggerOption {
	return func(l *Logger) {
		l.ciMode = ci
	}
}

func WithFormatter(f *Formatter) LoggerOption {
	return func(l *Logger) {
		l.formatter = f
	}
}

func WithShowTime(show bool) LoggerOption {
	return func(l *Logger) {
		l.showTime = show
	}
}

func New(logFile string, ciMode bool) *Logger {
	return NewWithOptions(logFile, WithCIMode(ciMode))
}

func NewWithOptions(logFile string, opts ...LoggerOption) *Logger {
	l := &Logger{
		formatter: NewFormatter(),
		showTime:  false,
	}

	for _, opt := range opts {
		opt(l)
	}

	var writers []io.Writer

	if logFile != "" && logFile != "-" {
		file, err := os.Create(logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to create log file %s: %v\n", logFile, err)
		} else {
			l.file = file
			l.rawWriter = file
			writers = append(writers, file)
		}
	}

	writers = append(writers, os.Stdout)

	if len(writers) > 1 {
		l.writer = io.MultiWriter(writers...)
	} else {
		l.writer = writers[0]
	}

	l.pipeReader, l.pipeWriter = io.Pipe()

	go l.processLogs()

	return l
}

func (l *Logger) Write(p []byte) (n int, err error) {
	return l.pipeWriter.Write(p)
}

func (l *Logger) processLogs() {
	scanner := bufio.NewScanner(l.pipeReader)
	// Increase buffer for long lines
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		l.processLine(line)
	}
}

func (l *Logger) processLine(line string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	level := l.formatter.ClassifyLine(line)
	noiseCategory := l.formatter.GetNoiseCategory(line)

	// Count warnings and errors (but not for noise lines that contain "error" keyword)
	if noiseCategory == NoiseCategoryNone {
		switch level {
		case LogLevelWarning:
			l.warnings++
		case LogLevelError:
			l.errors++
		}
	}

	// Always write raw to file
	if l.rawWriter != nil {
		_, _ = fmt.Fprintln(l.rawWriter, line)
	}

	if l.ciMode {
		l.processLineCIMode(line, level, noiseCategory)
	} else {
		l.processLineNormalMode(line, level)
	}
}

func (l *Logger) processLineCIMode(line string, level LogLevel, noiseCategory NoiseCategory) {
	// Filter stack traces in CI mode (non-project stack traces)
	if level == LogLevelStackTrace {
		if !l.formatter.IsProjectStackTrace(line) {
			return // Hide non-project stack traces
		}
	}

	// Handle noise grouping
	if noiseCategory != NoiseCategoryNone {
		// Start a new group if category changed
		if l.currentGroup != noiseCategory {
			l.endGroup()
			l.startGroup(noiseCategory)
		}
		_, _ = fmt.Fprintln(os.Stdout, line)
		return
	}

	// Not a noise line - end any active group
	l.endGroup()

	// Output with annotations for errors/warnings
	switch level {
	case LogLevelError:
		_, _ = fmt.Fprintf(os.Stdout, "::error::%s\n", line)
	case LogLevelWarning:
		_, _ = fmt.Fprintf(os.Stdout, "::warning::%s\n", line)
	case LogLevelStackTrace:
		// Project stack trace - show it
		_, _ = fmt.Fprintln(os.Stdout, line)
	default:
		_, _ = fmt.Fprintln(os.Stdout, line)
	}
}

func (l *Logger) processLineNormalMode(line string, level LogLevel) {
	// Check if we should show this line
	if !l.formatter.ShouldShow(line) {
		return
	}

	// Format the line
	formatted := l.formatter.FormatLine(line)

	if l.showTime {
		timestamp := time.Now().Format("15:04:05.000")
		_, _ = fmt.Fprintf(os.Stdout, "%s[%s]%s %s\n", ColorGray, timestamp, ColorReset, formatted)
	} else {
		_, _ = fmt.Fprintln(os.Stdout, formatted)
	}
}

func (l *Logger) startGroup(category NoiseCategory) {
	if category != NoiseCategoryNone {
		l.currentGroup = category
		_, _ = fmt.Fprintf(os.Stdout, "::group::%s\n", string(category))
	}
}

func (l *Logger) endGroup() {
	if l.currentGroup != NoiseCategoryNone {
		_, _ = fmt.Fprintln(os.Stdout, "::endgroup::")
		l.currentGroup = NoiseCategoryNone
	}
}

func (l *Logger) HasWarnings() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.warnings > 0
}

func (l *Logger) HasErrors() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.errors > 0
}

func (l *Logger) GetStats() (warnings, errors int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.warnings, l.errors
}

func (l *Logger) Close() error {
	if l.pipeWriter != nil {
		_ = l.pipeWriter.Close()
	}

	time.Sleep(100 * time.Millisecond)

	// End any active group in CI mode
	l.mutex.Lock()
	if l.ciMode && l.currentGroup != NoiseCategoryNone {
		_, _ = fmt.Fprintln(os.Stdout, "::endgroup::")
		l.currentGroup = NoiseCategoryNone
	}
	l.mutex.Unlock()

	warnings, errors := l.GetStats()
	if warnings > 0 || errors > 0 {
		var summaryColor string
		if errors > 0 {
			summaryColor = ColorRed
		} else if warnings > 0 {
			summaryColor = ColorYellow
		}

		if l.formatter.noColor {
			summary := fmt.Sprintf("\n=== Summary: %d warnings, %d errors ===\n", warnings, errors)
			_, _ = fmt.Fprint(os.Stdout, summary)
		} else {
			summary := fmt.Sprintf("\n%s=== Summary: %d warnings, %d errors ===%s\n", summaryColor, warnings, errors, ColorReset)
			_, _ = fmt.Fprint(os.Stdout, summary)
		}
	}

	if l.file != nil {
		return l.file.Close()
	}

	return nil
}
