package projects

import "encore.dev/storage/sqldb"

// db is the Encore-managed Postgres database for the projects service.
var db = sqldb.NewDatabase("projects", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})
