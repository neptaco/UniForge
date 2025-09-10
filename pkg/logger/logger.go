package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Logger struct {
	file       *os.File
	writer     io.Writer
	ciMode     bool
	warnings   int
	errors     int
	mutex      sync.Mutex
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
}

func New(logFile string, ciMode bool) *Logger {
	l := &Logger{
		ciMode: ciMode,
	}

	var writers []io.Writer

	if logFile != "" && logFile != "-" {
		file, err := os.Create(logFile)
		if err != nil {
			logrus.Warnf("Failed to create log file %s: %v", logFile, err)
		} else {
			l.file = file
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
	for scanner.Scan() {
		line := scanner.Text()
		l.processLine(line)
	}
}

func (l *Logger) processLine(line string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	lowerLine := strings.ToLower(line)

	if strings.Contains(lowerLine, "warning:") || strings.Contains(lowerLine, "warn:") {
		l.warnings++
		if l.ciMode {
			fmt.Fprintf(l.writer, "::warning::%s\n", line)
		} else {
			fmt.Fprintf(l.writer, "[%s] [WARN] %s\n", timestamp, line)
		}
	} else if strings.Contains(lowerLine, "error:") || strings.Contains(lowerLine, "fail") {
		l.errors++
		if l.ciMode {
			fmt.Fprintf(l.writer, "::error::%s\n", line)
		} else {
			fmt.Fprintf(l.writer, "[%s] [ERROR] %s\n", timestamp, line)
		}
	} else {
		if l.ciMode {
			fmt.Fprintln(l.writer, line)
		} else {
			fmt.Fprintf(l.writer, "[%s] %s\n", timestamp, line)
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
		summary := fmt.Sprintf("\n=== Build Summary ===\nWarnings: %d\nErrors: %d\n", warnings, errors)
		fmt.Fprint(l.writer, summary)
	}

	if l.file != nil {
		return l.file.Close()
	}

	return nil
}
