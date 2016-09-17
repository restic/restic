package main

import (
	"fmt"
	"io"
	"strings"
)

type Table struct {
	Header string
	Rows   [][]interface{}

	RowFormat string
}

func NewTable() Table {
	return Table{
		Rows: [][]interface{}{},
	}
}

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

const TimeFormat = "2006-01-02 15:04:05"
