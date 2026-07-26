package hub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// hubProgressFrame reproduces one redraw frame emitted by Unity Hub's headless
// installer: cursor-up, clear-to-end, then the two progress lines.
func hubProgressFrame(percent string) string {
	return "\033[2A\033[0JProgress:\n[Unity (6000.4.11f1)] downloading " + percent + "%\n"
}

func TestProgressFilterStripsANSIControlSequences(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)

	for _, percent := range []string{"0.12", "10.40", "55.03", "99.87"} {
		if _, err := filter.Write([]byte(hubProgressFrame(percent))); err != nil {
			t.Fatalf("write frame: %v", err)
		}
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	if strings.Contains(buf.String(), "\033[") {
		t.Errorf("output retains ANSI control sequences: %q", buf.String())
	}
}

func TestProgressFilterEmitsOneLinePerStep(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)

	// Feed a dense progress stream in 0.25% increments, as Unity Hub does.
	for percent := 0.0; percent <= 100.0; percent += 0.25 {
		if _, err := filter.Write([]byte(hubProgressFrameFloat(percent))); err != nil {
			t.Fatalf("write frame: %v", err)
		}
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	lines := nonEmptyLines(buf.String())
	// 0, 5, 10, ... 100 => 21 steps at a 5% granularity.
	if len(lines) != 21 {
		t.Fatalf("got %d progress lines, want 21 (5%% steps); output:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "0%") {
		t.Errorf("first line = %q, want it to report 0%%", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "100%") {
		t.Errorf("last line = %q, want it to report 100%%", lines[len(lines)-1])
	}
	for _, line := range lines {
		if !strings.Contains(line, "downloading") {
			t.Errorf("line = %q, want it to keep the phase name", line)
		}
	}
}

func TestProgressFilterPassesThroughNonProgressLines(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)

	if _, err := filter.Write([]byte("Unity Hub 3.19.5\n")); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if _, err := filter.Write([]byte(hubProgressFrame("50.00"))); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	if _, err := filter.Write([]byte("Editor is successfully installed\n")); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"Unity Hub 3.19.5", "Editor is successfully installed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

// Unity Hub writes to a pipe, so frames arrive split at arbitrary byte offsets.
func TestProgressFilterHandlesWritesSplitMidLine(t *testing.T) {
	var whole bytes.Buffer
	wholeFilter := newProgressFilter(&whole, false)
	stream := hubProgressFrame("0.00") + hubProgressFrame("50.00") + hubProgressFrame("100.00")
	if _, err := wholeFilter.Write([]byte(stream)); err != nil {
		t.Fatalf("write stream: %v", err)
	}
	if err := wholeFilter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	var chunked bytes.Buffer
	chunkedFilter := newProgressFilter(&chunked, false)
	for i := 0; i < len(stream); i += 7 {
		end := min(i+7, len(stream))
		if _, err := chunkedFilter.Write([]byte(stream[i:end])); err != nil {
			t.Fatalf("write chunk: %v", err)
		}
	}
	if err := chunkedFilter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	if whole.String() != chunked.String() {
		t.Errorf("chunked write produced different output:\nwhole:\n%s\nchunked:\n%s", whole.String(), chunked.String())
	}
}

// Passthrough log lines must survive verbatim: Unity Hub indents continuation
// lines of its diagnostics, and losing that changes the reported message.
func TestProgressFilterPreservesLogLineIndentation(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)

	if _, err := filter.Write([]byte("Install failed\n\tCaused by: disk full\r\n")); err != nil {
		t.Fatalf("write lines: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	want := "Install failed\n\tCaused by: disk full\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

// The splitter already consumed the line terminator, so anything still at the
// end of a passthrough line is content.
func TestProgressFilterPreservesLogLineTrailingWhitespace(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)

	if _, err := filter.Write([]byte("detail: value \t\r\n")); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	want := "detail: value \t\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

// Whitespace-only redraw frames and an indented Progress: header carry no
// information and must still be suppressed.
func TestProgressFilterSuppressesEmptyAndHeaderLines(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)

	if _, err := filter.Write([]byte("   \n\033[2A\033[0J\n  Progress:  \n")); err != nil {
		t.Fatalf("write lines: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("got %q, want no output", buf.String())
	}
}

// A bracketed diagnostic that happens to end in a percentage is not progress.
// Unity Hub's progress label always carries the version in parentheses, which
// is what separates it from a severity tag.
func TestProgressFilterDoesNotTreatBracketedDiagnosticAsProgress(t *testing.T) {
	for _, line := range []string{
		"[Warning] disk usage 97%",
		"[Warning] CPU 97%",
		"[Error] disk 12%",
	} {
		var buf bytes.Buffer
		filter := newProgressFilter(&buf, false)

		if _, err := filter.Write([]byte(line + "\n")); err != nil {
			t.Fatalf("write line: %v", err)
		}
		if err := filter.Close(); err != nil {
			t.Fatalf("close filter: %v", err)
		}

		if buf.String() != line+"\n" {
			t.Errorf("got %q, want %q passed through verbatim", buf.String(), line+"\n")
		}
	}
}

// Unity Hub builds the label from the parent item uid and the item display
// name, so a module's label looks nothing like the editor's and carries no
// parentheses. Both must be recognised as progress.
func TestProgressFilterRecognisesModuleProgress(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)

	if _, err := filter.Write([]byte("[6000.4.11f1-arm64 - Android Build Support] downloading 30.00%\n")); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	want := "[6000.4.11f1-arm64 - Android Build Support] downloading 30%\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

// Every other Unity Hub item state carries no percentage, so those lines pass
// through untouched rather than being throttled as progress.
func TestProgressFilterPassesThroughNonDownloadingStates(t *testing.T) {
	states := []string{
		"[Unity (6000.4.11f1)] queued for install.",
		"[Unity (6000.4.11f1)] validating download...",
		"[Unity (6000.4.11f1)] installing...",
		"[Unity (6000.4.11f1)] cleaning up download...",
		"[Unity (6000.4.11f1)] installed successfully.",
	}

	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)
	for _, state := range states {
		if _, err := filter.Write([]byte(state + "\n")); err != nil {
			t.Fatalf("write line: %v", err)
		}
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	want := strings.Join(states, "\n") + "\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

// Unity Hub never indents its progress lines, so an indented diagnostic that
// merely ends in a percentage must stay a verbatim log line.
func TestProgressFilterDoesNotTreatIndentedDiagnosticAsProgress(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)

	line := "  [Warning] disk usage 97%  "
	if _, err := filter.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	if buf.String() != line+"\n" {
		t.Errorf("got %q, want it passed through verbatim as %q", buf.String(), line+"\n")
	}
}

// A progress line padded with trailing spaces still has to be recognised as
// progress rather than passed through as a log line.
func TestProgressFilterMatchesPaddedProgressLine(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)

	if _, err := filter.Write([]byte("[Unity (6000.4.11f1)] downloading 42.00%  \n")); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	want := "[Unity (6000.4.11f1)] downloading 40%\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

// Unity Hub runs with --lang=ja, so its messages can be multi-byte, and a pipe
// splits them at arbitrary byte offsets.
func TestProgressFilterHandlesMultibyteSplitAcrossWrites(t *testing.T) {
	message := "エディターのインストールに失敗しました\n"

	var buf bytes.Buffer
	filter := newProgressFilter(&buf, false)
	for i := 0; i < len(message); i += 5 {
		end := min(i+5, len(message))
		if _, err := filter.Write([]byte(message[i:end])); err != nil {
			t.Fatalf("write chunk: %v", err)
		}
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	if got := strings.TrimSpace(buf.String()); got != strings.TrimSpace(message) {
		t.Errorf("got %q, want %q", got, strings.TrimSpace(message))
	}
}

func TestJSONProgressFilterEmitsOneEventPerLine(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, true)

	if _, err := filter.Write([]byte(hubProgressFrame("0.00"))); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	if _, err := filter.Write([]byte(hubProgressFrame("50.10"))); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	if _, err := filter.Write([]byte("Editor is successfully installed\n")); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	lines := nonEmptyLines(buf.String())
	if len(lines) != 3 {
		t.Fatalf("got %d events, want 3; output:\n%s", len(lines), buf.String())
	}

	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("event %q is not valid JSON: %v", line, err)
		}
		if _, ok := event["type"]; !ok {
			t.Errorf("event %q has no type field", line)
		}
	}

	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("decode first event: %v", err)
	}
	if first["type"] != "progress" {
		t.Errorf("first event type = %v, want progress", first["type"])
	}
	if first["phase"] != "downloading" {
		t.Errorf("first event phase = %v, want downloading", first["phase"])
	}
	if first["percent"] != float64(0) {
		t.Errorf("first event percent = %v, want 0", first["percent"])
	}

	var last map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &last); err != nil {
		t.Fatalf("decode last event: %v", err)
	}
	if last["type"] != "log" {
		t.Errorf("last event type = %v, want log", last["type"])
	}
	if last["message"] != "Editor is successfully installed" {
		t.Errorf("last event message = %v", last["message"])
	}
}

func TestJSONProgressFilterEmitsNoANSI(t *testing.T) {
	var buf bytes.Buffer
	filter := newProgressFilter(&buf, true)

	if _, err := filter.Write([]byte(hubProgressFrame("12.34"))); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	if strings.Contains(buf.String(), "\033[") {
		t.Errorf("JSON output retains ANSI control sequences: %q", buf.String())
	}
}

func hubProgressFrameFloat(percent float64) string {
	return hubProgressFrame(fmt.Sprintf("%.2f", percent))
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
