package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The JSON output mode is documented as one JSON object per line, so the
// terminal result must be a single compact object — not indented like the
// other commands' --output json.
func TestEmitResultWritesOneCompactJSONLine(t *testing.T) {
	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)

	run := &installRun{cmd: command, version: "6000.4.11f1", jsonOutput: true}
	if err := run.emitResult("installed", "/Applications/Unity/Hub/Editor/6000.4.11f1/Unity.app", []string{"ios"}); err != nil {
		t.Fatalf("emitResult: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1; output:\n%s", len(lines), output.String())
	}

	var result installResult
	if err := json.Unmarshal([]byte(lines[0]), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Type != "result" {
		t.Errorf("Type = %q, want result", result.Type)
	}
	if result.Status != "installed" {
		t.Errorf("Status = %q, want installed", result.Status)
	}
	if result.Version != "6000.4.11f1" {
		t.Errorf("Version = %q, want 6000.4.11f1", result.Version)
	}
	if len(result.Modules) != 1 || result.Modules[0] != "ios" {
		t.Errorf("Modules = %v, want [ios]", result.Modules)
	}
}

func TestEmitResultOmitsEmptyOptionalFields(t *testing.T) {
	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)

	run := &installRun{cmd: command, version: "6000.4.11f1", jsonOutput: true}
	if err := run.emitResult("already-installed", "", nil); err != nil {
		t.Fatalf("emitResult: %v", err)
	}

	for _, absent := range []string{`"path"`, `"modules"`} {
		if strings.Contains(output.String(), absent) {
			t.Errorf("output contains %s for an empty value: %s", absent, output.String())
		}
	}
}

// The interactive picker renders terminal frames to stdout, so it cannot run
// while stdout is a JSON Lines stream.
func TestInstallRejectsInteractivePickerInJSONMode(t *testing.T) {
	originalOutput, originalProject := installOutput, installProject
	installOutput, installProject = "json", ""
	t.Cleanup(func() {
		installOutput, installProject = originalOutput, originalProject
	})

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)

	err := runInstall(command, nil)
	if err == nil {
		t.Fatal("runInstall returned nil; want an error instead of launching the picker")
	}
	if !strings.Contains(err.Error(), "json") {
		t.Errorf("error = %q, want it to explain the JSON-mode restriction", err)
	}
	if output.Len() != 0 {
		t.Errorf("wrote %q to stdout; the JSON stream must stay clean", output.String())
	}
}

func TestStatusPrinterSuppressesChatterInJSONMode(t *testing.T) {
	if newStatusPrinter("json").enabled {
		t.Error("status printer is enabled in json mode; chatter would corrupt the stream")
	}
	if !newStatusPrinter("text").enabled {
		t.Error("status printer is disabled in text mode")
	}
}
