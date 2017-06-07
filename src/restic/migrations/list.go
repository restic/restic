package migrations

// All contains all migrations.
var All []Migration

func register(m Migration) {
	All = append(All, m)
}
