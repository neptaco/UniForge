package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/neptaco/unity-cli/pkg/logger"
	"github.com/neptaco/unity-cli/pkg/unity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logFollow    bool
	logEditor    bool
	logLines     int
	logRaw       bool
	logFullTrace bool
	logTimestamp bool
)

var logCmd = &cobra.Command{
	Use:   "logs",
	Short: "Display Unity Editor log",
	Long: `Display the Unity Editor log file with syntax highlighting.

The log file location is platform-specific:
  - macOS: ~/Library/Logs/Unity/Editor.log
  - Windows: %LOCALAPPDATA%\Unity\Editor\Editor.log
  - Linux: ~/.config/unity3d/Editor.log

Log lines are colorized:
  - Red: Errors and exceptions
  - Yellow: Warnings
  - Gray: Stack traces and startup noise

Examples:
  # Show last 100 lines (default)
  unity-cli logs

  # Show last 500 lines
  unity-cli logs -n 500

  # Follow log in real-time (like tail -f)
  unity-cli logs -f

  # Follow with timestamps
  unity-cli logs -f -t

  # Show raw output without colors
  unity-cli logs --raw

  # Show full stack traces (including Unity internals)
  unity-cli logs --full-trace

  # Open in text editor
  unity-cli logs --editor`,
	RunE: runLog,
}

func init() {
	rootCmd.AddCommand(logCmd)

	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "Follow log output in real-time")
	logCmd.Flags().BoolVar(&logEditor, "editor", false, "Open log in text editor ($EDITOR or vim)")
	logCmd.Flags().IntVarP(&logLines, "lines", "n", 100, "Number of lines to show")
	logCmd.Flags().BoolVar(&logRaw, "raw", false, "Show raw output without colors or filtering")
	logCmd.Flags().BoolVar(&logFullTrace, "full-trace", false, "Show full stack traces including Unity internals")
	logCmd.Flags().BoolVarP(&logTimestamp, "timestamp", "t", false, "Show timestamp for each line")
}

func runLog(cmd *cobra.Command, args []string) error {
	logPath, err := unity.GetEditorLogPath()
	if err != nil {
		return fmt.Errorf("failed to get log path: %w", err)
	}

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("log file not found: %s", logPath)
	}

	logrus.Debugf("Log file path: %s", logPath)

	if logEditor {
		return openInEditor(logPath)
	}

	if logFollow {
		return followLog(logPath)
	}

	return showLog(logPath, logLines)
}

func openInEditor(logPath string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	cmd := exec.Command(editor, logPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func followLog(logPath string) error {
	noColor := viper.GetBool("no-color") || os.Getenv("NO_COLOR") != ""

	if logRaw || noColor {
		// Use -F to follow file even if it gets recreated (e.g., when switching projects)
		cmd := exec.Command("tail", "-F", logPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Printf("Following %s (Ctrl+C to stop)\n\n", logPath)
		return cmd.Run()
	}

	// Use custom tail with formatting
	fmt.Printf("Following %s (Ctrl+C to stop)\n\n", logPath)

	formatter := logger.NewFormatter(
		logger.WithNoColor(false),
		logger.WithHideStackTrace(!logFullTrace),
	)

	// Use -F to follow file even if it gets recreated (e.g., when switching projects)
	cmd := exec.Command("tail", "-F", logPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if formatter.ShouldShow(line) {
			formatted := formatter.FormatLine(line)
			if logTimestamp {
				ts := time.Now().Format("15:04:05.000")
				fmt.Printf("%s[%s]%s %s\n", logger.ColorGray, ts, logger.ColorReset, formatted)
			} else {
				fmt.Println(formatted)
			}
		}
	}

	return cmd.Wait()
}

func showLog(logPath string, lines int) error {
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Read all lines into a buffer
	var allLines []string
	scanner := bufio.NewScanner(file)

	// Increase buffer size for long lines
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read log file: %w", err)
	}

	// Calculate starting position
	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}

	noColor := viper.GetBool("no-color") || os.Getenv("NO_COLOR") != ""

	if logRaw || noColor {
		// Print raw without formatting
		for i := start; i < len(allLines); i++ {
			fmt.Println(allLines[i])
		}
		return nil
	}

	// Print with formatting
	formatter := logger.NewFormatter(
		logger.WithNoColor(false),
		logger.WithHideStackTrace(!logFullTrace),
	)

	for i := start; i < len(allLines); i++ {
		line := allLines[i]
		if formatter.ShouldShow(line) {
			formatted := formatter.FormatLine(line)
			if logTimestamp {
				// For historical logs, show line number instead of time
				fmt.Printf("%s[%5d]%s %s\n", logger.ColorGray, i+1, logger.ColorReset, formatted)
			} else {
				fmt.Println(formatted)
			}
		}
	}

	return nil
}
