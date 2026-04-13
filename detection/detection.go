package detection

import "encore.dev/storage/sqldb"

var db = sqldb.NewDatabase("detection", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})
