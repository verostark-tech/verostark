package statements

import "encore.dev/storage/sqldb"

var db = sqldb.NewDatabase("statements", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})
