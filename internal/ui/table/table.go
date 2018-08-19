package table

import (
	"bytes"
	"io"
	"strings"

	"text/template"
)

// Table contains data for a table to be printed.
type Table struct {
	columns   []string
	templates []*template.Template
	data      []interface{}
	footer    []string

	CellSeparator  string
	PrintHeader    func(io.Writer, string) error
	PrintSeparator func(io.Writer, string) error
	PrintData      func(io.Writer, int, string) error
	PrintFooter    func(io.Writer, string) error
}

var funcmap = template.FuncMap{
	"join": strings.Join,
}

// New initializes a new Table
func New() *Table {
	p := func(w io.Writer, s string) error {
		_, err := w.Write(append([]byte(s), '\n'))
		return err
	}
	return &Table{
		CellSeparator:  "  ",
		PrintHeader:    p,
		PrintSeparator: p,
		PrintData: func(w io.Writer, _ int, s string) error {
			return p(w, s)
		},
		PrintFooter: p,
	}
}

// AddColumn adds a new header field with the header and format, which is
// expected to be template string compatible with text/template. When compiling
// the format fails, AddColumn panics.
func (t *Table) AddColumn(header, format string) {
	t.columns = append(t.columns, header)
	tmpl, err := template.New("template for " + header).Funcs(funcmap).Parse(format)
	if err != nil {
		panic(err)
	}

	t.templates = append(t.templates, tmpl)
}

// AddRow adds a new row to the table, which is filled with data.
func (t *Table) AddRow(data interface{}) {
	t.data = append(t.data, data)
}

// AddFooter prints line after the table
func (t *Table) AddFooter(line string) {
	t.footer = append(t.footer, line)
}

func printLine(w io.Writer, print func(io.Writer, string) error, sep string, data []string, widths []int) error {
	var fields [][]string

	maxLines := 1
	for _, d := range data {
		lines := strings.Split(d, "\n")
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
		fields = append(fields, lines)
	}

	for i := 0; i < maxLines; i++ {
		var s string

		for fieldNum, lines := range fields {
			var v string

			if i < len(lines) {
				v += lines[i]
			}

			// apply padding
			pad := widths[fieldNum] - len(v)
			if pad > 0 {
				v += strings.Repeat(" ", pad)
			}

			if fieldNum > 0 {
				v = sep + v
			}

			s += v
		}

		err := print(w, strings.TrimRight(s, " "))
		if err != nil {
			return err
		}
	}

	return nil
}

// Write prints the table to w.
func (t *Table) Write(w io.Writer) error {
	columns := len(t.templates)
	if columns == 0 {
		return nil
	}

	// collect all data fields from all columns
	lines := make([][]string, 0, len(t.data))
	buf := bytes.NewBuffer(nil)

	for _, data := range t.data {
		row := make([]string, 0, len(t.templates))
		for _, tmpl := range t.templates {
			err := tmpl.Execute(buf, data)
			if err != nil {
				return err
			}

			row = append(row, string(buf.Bytes()))
			buf.Reset()
		}
		lines = append(lines, row)
	}

	// find max width for each cell
	columnWidths := make([]int, columns)
	for i, desc := range t.columns {
		for _, line := range strings.Split(desc, "\n") {
			if columnWidths[i] < len(line) {
				columnWidths[i] = len(desc)
			}
		}
	}
	for _, line := range lines {
		for i, content := range line {
			for _, l := range strings.Split(content, "\n") {
				if columnWidths[i] < len(l) {
					columnWidths[i] = len(l)
				}
			}
		}
	}

	// calculate the total width of the table
	totalWidth := 0
	for _, width := range columnWidths {
		totalWidth += width
	}
	totalWidth += (columns - 1) * len(t.CellSeparator)

	// write header
	if len(t.columns) > 0 {
		err := printLine(w, t.PrintHeader, t.CellSeparator, t.columns, columnWidths)
		if err != nil {
			return err
		}

		// draw separation line
		err = t.PrintSeparator(w, strings.Repeat("-", totalWidth))
		if err != nil {
			return err
		}
	}

	// write all the lines
	for i, line := range lines {
		print := func(w io.Writer, s string) error {
			return t.PrintData(w, i, s)
		}
		err := printLine(w, print, t.CellSeparator, line, columnWidths)
		if err != nil {
			return err
		}
	}

	// draw separation line
	err := t.PrintSeparator(w, strings.Repeat("-", totalWidth))
	if err != nil {
		return err
	}

	if len(t.footer) > 0 {
		// write the footer
		for _, line := range t.footer {
			err := t.PrintFooter(w, line)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
