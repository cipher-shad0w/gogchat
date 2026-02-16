package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// Format represents the output format type.
type Format string

const (
	// FormatHuman outputs human-readable formatted text.
	FormatHuman Format = "human"
	// FormatJSON outputs JSON.
	FormatJSON Format = "json"
)

// Formatter handles output formatting and dispatch.
type Formatter struct {
	Format Format
	Quiet  bool
}

// NewFormatter creates a new Formatter based on the given mode flags.
func NewFormatter(jsonMode, quiet bool) *Formatter {
	f := &Formatter{
		Format: FormatHuman,
		Quiet:  quiet,
	}
	if jsonMode {
		f.Format = FormatJSON
	}
	return f
}

// Print dispatches data to either human or JSON output.
// In JSON mode, data is marshaled to indented JSON on stdout.
// In human mode, data is printed using fmt default formatting.
func (f *Formatter) Print(data interface{}) error {
	if f.Format == FormatJSON {
		return PrintJSON(data)
	}
	_, err := fmt.Fprintln(os.Stdout, data)
	return err
}

// PrintRaw prints raw JSON. In JSON mode it pretty-prints the raw JSON;
// in human mode it also pretty-prints for readability.
func (f *Formatter) PrintRaw(raw json.RawMessage) error {
	return PrintRawJSON(raw)
}

// PrintMessage prints an informational message to stdout.
// Suppressed in quiet mode.
func (f *Formatter) PrintMessage(msg string) {
	if f.Quiet {
		return
	}
	fmt.Fprintln(os.Stdout, msg)
}

// PrintError prints an error message to stderr. Always printed regardless of quiet mode.
func (f *Formatter) PrintError(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

// PrintSuccess prints a success message with a checkmark prefix to stdout.
// Suppressed in quiet mode.
func (f *Formatter) PrintSuccess(msg string) {
	if f.Quiet {
		return
	}
	fmt.Fprintf(os.Stdout, "✓ %s\n", msg)
}

// IsJSON returns true if the formatter is in JSON output mode.
func (f *Formatter) IsJSON() bool {
	return f.Format == FormatJSON
}
