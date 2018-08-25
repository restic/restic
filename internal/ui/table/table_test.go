package table

import (
	"bytes"
	"strings"
	"testing"
)

func TestTable(t *testing.T) {
	var tests = []struct {
		create func(t testing.TB) *Table
		output string
	}{
		{
			func(t testing.TB) *Table {
				return New()
			},
			"",
		},
		{
			func(t testing.TB) *Table {
				table := New()
				table.AddColumn("first column", "data: {{.First}}")
				table.AddRow(struct{ First string }{"first data field"})
				return table
			},
			`
first column
----------------------
data: first data field
----------------------
`,
		},
		{
			func(t testing.TB) *Table {
				table := New()
				table.AddColumn("  first column  ", "data: {{.First}}")
				table.AddRow(struct{ First string }{"d"})
				return table
			},
			`
  first column
----------------
data: d
----------------
`,
		},
		{
			func(t testing.TB) *Table {
				table := New()
				table.AddColumn("first column", "data: {{.First}}")
				table.AddRow(struct{ First string }{"first data field"})
				table.AddRow(struct{ First string }{"second data field"})
				table.AddFooter("footer1")
				table.AddFooter("footer2")
				return table
			},
			`
first column
-----------------------
data: first data field
data: second data field
-----------------------
footer1
footer2
`,
		},
		{
			func(t testing.TB) *Table {
				table := New()
				table.AddColumn("  first name", `{{printf "%12s" .FirstName}}`)
				table.AddColumn("last name", "{{.LastName}}")
				table.AddRow(struct{ FirstName, LastName string }{"firstname", "lastname"})
				table.AddRow(struct{ FirstName, LastName string }{"John", "Doe"})
				table.AddRow(struct{ FirstName, LastName string }{"Johann", "van den Berjen"})
				return table
			},
			`
  first name  last name
----------------------------
   firstname  lastname
        John  Doe
      Johann  van den Berjen
----------------------------
`,
		},
		{
			func(t testing.TB) *Table {
				table := New()
				table.AddColumn("host name", `{{.Host}}`)
				table.AddColumn("time", `{{.Time}}`)
				table.AddColumn("zz", "xxx")
				table.AddColumn("tags", `{{join .Tags ","}}`)
				table.AddColumn("dirs", `{{join .Dirs ","}}`)

				type data struct {
					Host       string
					Time       string
					Tags, Dirs []string
				}
				table.AddRow(data{"foo", "2018-08-19 22:22:22", []string{"work"}, []string{"/home/user/work"}})
				table.AddRow(data{"foo", "2018-08-19 22:22:22", []string{"other"}, []string{"/home/user/other"}})
				table.AddRow(data{"foo", "2018-08-19 22:22:22", []string{"other"}, []string{"/home/user/other"}})
				return table
			},
			`
host name  time                 zz   tags   dirs
------------------------------------------------------------
foo        2018-08-19 22:22:22  xxx  work   /home/user/work
foo        2018-08-19 22:22:22  xxx  other  /home/user/other
foo        2018-08-19 22:22:22  xxx  other  /home/user/other
------------------------------------------------------------
`,
		},
		{
			func(t testing.TB) *Table {
				table := New()
				table.AddColumn("host name", `{{.Host}}`)
				table.AddColumn("time", `{{.Time}}`)
				table.AddColumn("zz", "xxx")
				table.AddColumn("tags", `{{join .Tags "\n"}}`)
				table.AddColumn("dirs", `{{join .Dirs "\n"}}`)

				type data struct {
					Host       string
					Time       string
					Tags, Dirs []string
				}
				table.AddRow(data{"foo", "2018-08-19 22:22:22", []string{"work", "go"}, []string{"/home/user/work", "/home/user/go"}})
				table.AddRow(data{"foo", "2018-08-19 22:22:22", []string{"other"}, []string{"/home/user/other"}})
				table.AddRow(data{"foo", "2018-08-19 22:22:22", []string{"other", "bar"}, []string{"/home/user/other"}})
				return table
			},
			`
host name  time                 zz   tags   dirs
------------------------------------------------------------
foo        2018-08-19 22:22:22  xxx  work   /home/user/work
                                     go     /home/user/go
foo        2018-08-19 22:22:22  xxx  other  /home/user/other
foo        2018-08-19 22:22:22  xxx  other  /home/user/other
                                     bar
------------------------------------------------------------
`,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			table := test.create(t)
			buf := bytes.NewBuffer(nil)
			err := table.Write(buf)
			if err != nil {
				t.Fatal(err)
			}

			want := strings.TrimLeft(test.output, "\n")
			if string(buf.Bytes()) != want {
				t.Errorf("wrong output\n---- want ---\n%s\n---- got ---\n%s\n-------\n", want, buf.Bytes())
			}
		})
	}
}
