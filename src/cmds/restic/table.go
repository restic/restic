package main

import (
	"fmt"
	"io"
	"strings"
)

// Table contains data for a table to be printed.
type Table struct {
	Header string
	Rows   [][]interface{}

	RowFormat string
}

// NewTable initializes a new Table.
func NewTable() Table {
	return Table{
		Rows: [][]interface{}{},
	}
}

// Write prints the table to w.
func (t Table) Write(w io.Writer) error {
	_, err := fmt.Fprintln(w, t.Header)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, strings.Repeat("-", 70))
	if err != nil {
		return err
	}

	for _, row := range t.Rows {
		_, err = fmt.Fprintf(w, t.RowFormat+"\n", row...)
		if err != nil {
			return err
		}
	}

	return nil
}

// TimeFormat is the format used for all timestamps printed by restic.
const TimeFormat = "2006-01-02 15:04:05"
