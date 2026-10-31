package app

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (read-only access to OMP DBs)
)

// sqlOpen opens a SQLite database file with the pure-Go driver. Used for the
// read-only OMP catalogs (models.db, agent.db).
func sqlOpen(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("Failed to open database: %w", err)
	}
	return db, nil
}
