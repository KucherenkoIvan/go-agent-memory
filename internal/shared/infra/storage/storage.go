// Package storage owns the embedded database: opening it and applying the
// schema. Migrations live here — never in the repo root, never in features.
package storage

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/KucherenkoIvan/go-kernel/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open opens (creating if missing) the database and applies migrations.
// Use ":memory:" in tests.
func Open(ctx context.Context, path string) (*sqlite.Client, error) {
	db, err := sqlite.Open(path)
	if err != nil {
		return nil, err
	}

	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage: embedded migrations: %w", err)
	}
	if err := db.Migrate(ctx, migrations); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
