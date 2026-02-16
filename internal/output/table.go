package output

import (
	"strings"
)

const (
	// maxColumnWidth is the maximum display width for a column value before truncation.
	maxColumnWidth = 50
	// columnPadding is the number of spaces between columns.
	columnPadding = 2
)

// Table formats data into aligned columns for human-readable output.
type Table struct {
	Headers []string
	Rows    [][]string
}

// NewTable creates a new table with the given column headers.
func NewTable(headers ...string) *Table {
	return &Table{
		Headers: headers,
	}
}

// AddRow appends a row of values to the table. Returns the table for chaining.
func (t *Table) AddRow(values ...string) *Table {
	t.Rows = append(t.Rows, values)
	return t
}

// Render returns the table as a formatted, aligned string with header underlines.
func (t *Table) Render() string {
	if len(t.Headers) == 0 {
		return ""
	}

	numCols := len(t.Headers)

	// Calculate max width per column, capped at maxColumnWidth.
	widths := make([]int, numCols)
	for i, h := range t.Headers {
		if len(h) > widths[i] {
			widths[i] = len(h)
		}
	}
	for _, row := range t.Rows {
		for i := 0; i < numCols && i < len(row); i++ {
			val := Truncate(row[i], maxColumnWidth)
			if len(val) > widths[i] {
				widths[i] = len(val)
			}
		}
	}

	// Cap widths at maxColumnWidth.
	for i := range widths {
		if widths[i] > maxColumnWidth {
			widths[i] = maxColumnWidth
		}
	}

	pad := strings.Repeat(" ", columnPadding)
	var b strings.Builder

	// Print headers in UPPERCASE.
	for i, h := range t.Headers {
		upper := strings.ToUpper(h)
		if i > 0 {
			b.WriteString(pad)
		}
		b.WriteString(upper)
		if i < numCols-1 {
			b.WriteString(strings.Repeat(" ", widths[i]-len(upper)))
		}
	}
	b.WriteString("\n")

	// Print dashes under each header.
	for i, w := range widths {
		if i > 0 {
			b.WriteString(pad)
		}
		b.WriteString(strings.Repeat("-", w))
	}
	b.WriteString("\n")

	// Print rows.
	for _, row := range t.Rows {
		for i := 0; i < numCols; i++ {
			if i > 0 {
				b.WriteString(pad)
			}
			val := ""
			if i < len(row) {
				val = Truncate(row[i], maxColumnWidth)
			}
			b.WriteString(val)
			if i < numCols-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-len(val)))
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}
