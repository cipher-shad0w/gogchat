package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// PrintJSON marshals data with indentation and prints it to stdout.
func PrintJSON(data interface{}) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, string(out))
	return err
}

// PrintRawJSON pretty-prints raw JSON bytes to stdout.
func PrintRawJSON(raw json.RawMessage) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		// If we can't indent (e.g. invalid JSON), print as-is.
		_, writeErr := fmt.Fprintln(os.Stdout, string(raw))
		return writeErr
	}
	_, err := fmt.Fprintln(os.Stdout, buf.String())
	return err
}

// FormatTime converts a Google API datetime string (RFC 3339) to a
// human-readable local time format. If parsing fails, the original
// string is returned unchanged.
func FormatTime(t string) string {
	if t == "" {
		return ""
	}

	parsed, err := time.Parse(time.RFC3339Nano, t)
	if err != nil {
		// Try RFC 3339 without nanoseconds.
		parsed, err = time.Parse(time.RFC3339, t)
		if err != nil {
			return t
		}
	}

	local := parsed.Local()
	now := time.Now()

	// If it's today, show just the time.
	if local.Year() == now.Year() && local.YearDay() == now.YearDay() {
		return local.Format("3:04 PM")
	}

	// If it's this year, show month and day with time.
	if local.Year() == now.Year() {
		return local.Format("Jan 2, 3:04 PM")
	}

	// Otherwise show full date.
	return local.Format("Jan 2, 2006 3:04 PM")
}

// Truncate truncates a string to maxLen characters, appending "..." if truncated.
// If maxLen is less than or equal to 3, the string is truncated to maxLen without ellipsis.
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	// Replace newlines with spaces for display purposes.
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")

	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
