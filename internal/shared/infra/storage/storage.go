// Package storage owns the memory database: default location, opening, and
// migrations.
//
// Multi-process access is the deployment model: a long-running MCP server
// and CLI one-shots (from any harness) share the same file. That is safe on
// one machine because the kernel's sqlite client sets WAL, busy_timeout, and
// BEGIN IMMEDIATE write transactions — see the kernel's sqlite guide. The
// file must live on a local filesystem, never NFS.
package storage

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/KucherenkoIvan/go-kernel/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DefaultPath is the global memory location: cross-session knowledge is the
// point, so one file for everything unless overridden (RECALL_DB).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("storage: resolving home: %w", err)
	}
	return filepath.Join(home, ".local", "share", "recall", "memory.db"), nil
}

// Store is the opened database.
type Store struct {
	DB *sqlite.Client
}

// Open opens (creating if missing) the database and applies migrations.
// Concurrent recall processes are fine — migrations are the one racy moment,
// and they are idempotent (schema_migrations) and fast.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}

	db, err := sqlite.Open(path)
	if err != nil {
		return nil, err
	}

	// read-path tuning on top of the kernel defaults; the client pools a
	// single connection, so per-connection pragmas hold for the process.
	// temp_store keeps sort/group spills in RAM; the cache ceiling (64MB)
	// matters for big stores and costs nothing for small ones.
	for _, pragma := range []string{"PRAGMA temp_store = MEMORY", "PRAGMA cache_size = -65536"} {
		if _, err := db.DB().ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("storage: %s: %w", pragma, err)
		}
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

	return &Store{DB: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.DB.Close()
}

func ensureDir(path string) error {
	if strings.Contains(path, ":memory:") {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("storage: creating data dir: %w", err)
	}
	return nil
}
