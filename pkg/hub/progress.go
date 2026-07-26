package hub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// progressStepPercent is the granularity at which progress is reported on
// non-TTY streams. Unity Hub redraws roughly every 0.25%, which is unreadable
// once it lands in a log file.
const progressStepPercent = 5

var (
	// ansiSequence matches the CSI escape sequences Unity Hub uses to redraw
	// its progress block (e.g. ESC[2A to move up, ESC[0J to clear).
	ansiSequence = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

	// hubProgressLine matches "[Unity (6000.4.11f1)] downloading 99.24%".
	//
	// Unity Hub's headless CLI writes a label of the form
	// `[<parentItem><itemDisplayName>]` and follows it with the item state.
	// "downloading" is the only state that carries a percentage — the others
	// ("installing...", "validating download...", "queued for install.") have
	// no number — so anchoring on that word is what separates real progress
	// from a diagnostic of the same shape, such as "[Warning] CPU 97%".
	// The label itself is dynamic and cannot be constrained further.
	hubProgressLine = regexp.MustCompile(`^(\[.*\])[ \t]+(downloading)[ \t]+([0-9]+(?:\.[0-9]+)?)%[ \t]*$`)
)

// progressFilter converts Unity Hub's ANSI-redrawing progress output into
// line-oriented output. Unity Hub assumes a terminal even when its stdout is a
// pipe, so without this the output is unreadable in log files and yields
// nothing at all until the process exits when piped through tail or grep.
type progressFilter struct {
	out io.Writer
	enc *json.Encoder // non-nil in JSON Lines mode

	pending   []byte
	lastStep  int
	lastLabel string
	lastPhase string
}

// progressEvent is one machine-readable progress record.
type progressEvent struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	Phase   string `json:"phase"`
	Percent int    `json:"percent"`
}

// logEvent is one machine-readable passthrough line.
type logEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// newProgressFilter returns a filter writing line-oriented progress to out,
// as plain text or as one JSON object per line. Callers must Close it to flush
// a trailing partial line.
func newProgressFilter(out io.Writer, jsonMode bool) *progressFilter {
	filter := &progressFilter{out: out}
	if jsonMode {
		filter.enc = json.NewEncoder(out)
	}
	return filter
}

func (f *progressFilter) Write(p []byte) (int, error) {
	total := len(p)

	// Buffer bytes rather than runes: Unity Hub writes to a pipe, so a
	// multi-byte character can be split across two Write calls.
	for rest := p; len(rest) > 0; {
		// Unity Hub terminates redraw frames with either newline or carriage
		// return; treat both as line boundaries.
		i := bytes.IndexAny(rest, "\n\r")
		if i < 0 {
			f.pending = append(f.pending, rest...)
			break
		}
		f.pending = append(f.pending, rest[:i]...)
		if err := f.flushLine(); err != nil {
			return 0, err
		}
		rest = rest[i+1:]
	}

	return total, nil
}

// Close flushes any buffered partial line.
func (f *progressFilter) Close() error {
	return f.flushLine()
}

func (f *progressFilter) flushLine() error {
	line := string(f.pending)
	f.pending = f.pending[:0]

	// Roughly half the lines in a redraw frame carry no escape sequence, and
	// ReplaceAllString allocates whether or not it matches.
	if strings.IndexByte(line, 0x1b) >= 0 {
		line = ansiSequence.ReplaceAllString(line, "")
	}

	// The split already consumed the line terminator, so whatever whitespace
	// remains is content — Unity Hub indents continuation lines of its
	// diagnostics. Only the discard decisions look at a trimmed copy; anything
	// emitted is passed through verbatim.
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}

	// The "Progress:" header only exists to anchor the redraw block.
	if trimmed == "Progress:" {
		return nil
	}

	if match := hubProgressLine.FindStringSubmatch(line); match != nil {
		return f.writeProgress(match[1], match[2], match[3])
	}

	f.lastLabel = ""
	f.lastPhase = ""
	return f.emit(logEvent{Type: "log", Message: line}, line)
}

// writeProgress emits at most one line per progressStepPercent per phase.
func (f *progressFilter) writeProgress(label, phase, percent string) error {
	value, err := strconv.ParseFloat(percent, 64)
	if err != nil {
		// Not a percentage after all; pass the line through unchanged.
		line := fmt.Sprintf("%s %s %s%%", label, phase, percent)
		return f.emit(logEvent{Type: "log", Message: line}, line)
	}

	step := int(value) / progressStepPercent
	if step == f.lastStep && label == f.lastLabel && phase == f.lastPhase {
		return nil
	}
	f.lastStep = step
	f.lastLabel = label
	f.lastPhase = phase

	rounded := step * progressStepPercent
	return f.emit(
		progressEvent{
			Type:    "progress",
			Label:   strings.Trim(label, "[]"),
			Phase:   phase,
			Percent: rounded,
		},
		fmt.Sprintf("%s %s %d%%", label, phase, rounded),
	)
}

// emit writes the event in JSON mode, or the plain text line otherwise.
func (f *progressFilter) emit(event any, text string) error {
	if f.enc != nil {
		if err := f.enc.Encode(event); err != nil {
			return fmt.Errorf("encode progress event: %w", err)
		}
		return nil
	}
	_, err := fmt.Fprintln(f.out, text)
	return err
}
