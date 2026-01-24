// Package ui provides user interface utilities for CLI output
package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/mattn/go-isatty"
)

var (
	// Styles
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	// Logger for debug output
	logger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
	})

	// Debug mode flag
	debugMode = false
)

// SetDebugMode enables or disables debug output
func SetDebugMode(enabled bool) {
	debugMode = enabled
	if enabled {
		logger.SetLevel(log.DebugLevel)
	} else {
		logger.SetLevel(log.WarnLevel)
	}
}

// Info prints an informational message
func Info(format string, args ...any) {
	fmt.Println(infoStyle.Render(fmt.Sprintf(format, args...)))
}

// Success prints a success message with checkmark
func Success(format string, args ...any) {
	fmt.Println(successStyle.Render("✓ " + fmt.Sprintf(format, args...)))
}

// Warn prints a warning message
func Warn(format string, args ...any) {
	fmt.Println(warnStyle.Render("⚠ " + fmt.Sprintf(format, args...)))
}

// Error prints an error message to stderr
func Error(format string, args ...any) {
	fmt.Fprintln(os.Stderr, errorStyle.Render("✗ " + fmt.Sprintf(format, args...)))
}

// Muted prints a muted/secondary message
func Muted(format string, args ...any) {
	fmt.Println(mutedStyle.Render(fmt.Sprintf(format, args...)))
}

// Print prints a plain message without styling
func Print(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

// Debug prints a debug message (only if debug mode is enabled)
func Debug(msg string, keyvals ...any) {
	if debugMode {
		logger.Debug(msg, keyvals...)
	}
}

// spinnerModel is the bubbletea model for spinner
type spinnerModel struct {
	spinner  spinner.Model
	message  string
	quitting bool
	done     bool
	err      error
}

type taskDoneMsg struct {
	err error
}

func (m spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}
	case taskDoneMsg:
		m.done = true
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m spinnerModel) View() string {
	if m.done || m.quitting {
		return ""
	}
	return fmt.Sprintf("%s %s", m.spinner.View(), m.message)
}

// isTTY checks if stdout is a terminal
func isTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// WithSpinner runs a task with a spinner and returns the result
func WithSpinner[T any](message string, task func() (T, error)) (T, error) {
	// Skip spinner if not a TTY
	if !isTTY() {
		return task()
	}

	var result T
	var taskErr error

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := spinnerModel{
		spinner: s,
		message: message,
	}

	p := tea.NewProgram(m)

	// Run task in goroutine
	go func() {
		result, taskErr = task()
		p.Send(taskDoneMsg{err: taskErr})
	}()

	// Run spinner
	if _, err := p.Run(); err != nil {
		return result, err
	}

	return result, taskErr
}

// WithSpinnerNoResult runs a task with a spinner that doesn't return a value
func WithSpinnerNoResult(message string, task func() error) error {
	_, err := WithSpinner(message, func() (struct{}, error) {
		return struct{}{}, task()
	})
	return err
}

// StartSpinner starts a spinner and returns a stop function
// Use this for long-running operations where you need more control
func StartSpinner(message string) func(success bool, resultMsg string) {
	done := make(chan struct{})
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	go func() {
		for {
			select {
			case <-done:
				return
			default:
				fmt.Printf("\r%s %s", s.View(), message)
				time.Sleep(100 * time.Millisecond)
				s, _ = s.Update(s.Tick())
			}
		}
	}()

	return func(success bool, resultMsg string) {
		close(done)
		fmt.Print("\r\033[K") // Clear line
		if success {
			Success("%s", resultMsg)
		} else {
			Error("%s", resultMsg)
		}
	}
}
