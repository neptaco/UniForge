package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Logger struct {
	file       *os.File
	writer     io.Writer
	rawWriter  io.Writer // For file output without colors
	ciMode     bool
	warnings   int
	errors     int
	mutex      sync.Mutex
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	formatter  *Formatter
	showTime   bool
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
			logrus.Warnf("Failed to create log file %s: %v", logFile, err)
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

	// Count warnings and errors
	if level == LogLevelWarning {
		l.warnings++
	} else if level == LogLevelError {
		l.errors++
	}

	// Check if we should show this line
	if !l.formatter.ShouldShow(line) {
		// Still write to file if we have one (without colors)
		if l.rawWriter != nil {
			fmt.Fprintln(l.rawWriter, line)
		}
		return
	}

	// Format the line
	formatted := l.formatter.FormatLine(line)

	if l.ciMode {
		// CI mode: use GitHub Actions annotations
		switch level {
		case LogLevelError:
			fmt.Fprintf(os.Stdout, "::error::%s\n", line)
		case LogLevelWarning:
			fmt.Fprintf(os.Stdout, "::warning::%s\n", line)
		default:
			fmt.Fprintln(os.Stdout, line)
		}
		// Write raw to file
		if l.rawWriter != nil {
			fmt.Fprintln(l.rawWriter, line)
		}
	} else {
		// Normal mode: colorized output to stdout
		if l.showTime {
			timestamp := time.Now().Format("15:04:05.000")
			fmt.Fprintf(os.Stdout, "%s[%s]%s %s\n", ColorGray, timestamp, ColorReset, formatted)
		} else {
			fmt.Fprintln(os.Stdout, formatted)
		}
		// Write raw to file
		if l.rawWriter != nil {
			fmt.Fprintln(l.rawWriter, line)
		}
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
		l.pipeWriter.Close()
	}

	time.Sleep(100 * time.Millisecond)

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
			fmt.Fprint(os.Stdout, summary)
		} else {
			summary := fmt.Sprintf("\n%s=== Summary: %d warnings, %d errors ===%s\n", summaryColor, warnings, errors, ColorReset)
			fmt.Fprint(os.Stdout, summary)
		}
	}

	if l.file != nil {
		return l.file.Close()
	}

	return nil
}
