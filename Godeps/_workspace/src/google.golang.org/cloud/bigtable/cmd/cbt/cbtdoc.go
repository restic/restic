// DO NOT EDIT. THIS IS AUTOMATICALLY GENERATED.
// Run "go generate" to regenerate.
//go:generate go run cbt.go -o cbtdoc.go doc

/*
Cbt is a tool for doing basic interactions with Cloud Bigtable.

Usage:

	cbt [options] command [arguments]

The commands are:

	count                     Count rows in a table
	createfamily              Create a column family
	createtable               Create a table
	deletefamily              Delete a column family
	deleterow                 Delete a row
	deletetable               Delete a table
	doc                       Print documentation for cbt
	help                      Print help text
	listclusters              List clusters in a project
	lookup                    Read from a single row
	ls                        List tables and column families
	read                      Read rows
	set                       Set value of a cell

Use "cbt help <command>" for more information about a command.


Count rows in a table

Usage:
	cbt count <table>




Create a column family

Usage:
	cbt createfamily <table> <family>




Create a table

Usage:
	cbt createtable <table>




Delete a column family

Usage:
	cbt deletefamily <table> <family>




Delete a row

Usage:
	cbt deleterow <table> <row>




Delete a table

Usage:
	cbt deletetable <table>




Print documentation for cbt

Usage:
	cbt doc




Print help text

Usage:
	cbt help [command]




List clusters in a project

Usage:
	cbt listclusters




Read from a single row

Usage:
	cbt lookup <table> <row>




List tables and column families

Usage:
	cbt ls			List tables
	cbt ls <table>		List column families in <table>




Read rows

Usage:
	cbt read <table> [start=<row>] [limit=<row>] [prefix=<prefix>]
	  start=<row>		Start reading at this row
	  limit=<row>		Stop reading before this row
	  prefix=<prefix>	Read rows with this prefix





Set value of a cell

Usage:
	cbt set <table> <row> family:column=val[@ts] ...
	  family:column=val[@ts] may be repeated to set multiple cells.

	  ts is an optional integer timestamp.
	  If it cannot be parsed, the `@ts` part will be
	  interpreted as part of the value.




*/
package main
