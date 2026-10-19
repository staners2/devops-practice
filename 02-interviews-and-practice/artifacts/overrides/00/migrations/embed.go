package migrations

import _ "embed"

// Initial contains schema version 1 used by the migration command.
//
//go:embed 001_initial.sql
var Initial string
