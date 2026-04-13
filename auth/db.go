package auth

import "encore.dev/storage/sqldb"

var db = sqldb.NewDatabase("auth", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})
