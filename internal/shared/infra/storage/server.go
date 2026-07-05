package storage

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/KucherenkoIvan/go-kernel/sqlite"
)

//go:embed servermigrations/*.sql
var serverMigrationsFS embed.FS

// DefaultServerDir is where a hosted recall keeps its control plane
// (keys.db) and the per-space memory files (spaces/<name>.db).
func DefaultServerDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("storage: resolving home: %w", err)
	}
	return filepath.Join(home, ".local", "share", "recall", "server"), nil
}

// OpenServer opens (creating if missing) the server control-plane database
// — API keys and spaces — with its own migration set. Space memory files
// are NOT opened here; they go through plain Open so they stay
// byte-identical to a local memory.db.
func OpenServer(ctx context.Context, path string) (*Store, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}

	db, err := sqlite.Open(path)
	if err != nil {
		return nil, err
	}

	migrations, err := fs.Sub(serverMigrationsFS, "servermigrations")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage: embedded server migrations: %w", err)
	}
	if err := db.Migrate(ctx, migrations); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{DB: db}, nil
}

// ExportSnapshot writes a consistent point-in-time copy of a database to
// dest via VACUUM INTO — safe while other processes write under WAL. The
// snapshot of a space file IS a local memory.db: hand it to a user and
// they point RECALL_DB at it. Fails if dest already exists (VACUUM INTO
// requirement).
func ExportSnapshot(ctx context.Context, dbPath, dest string) error {
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("storage: source database: %w", err)
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("storage: export destination %s already exists", dest)
	}
	if err := ensureDir(dest); err != nil {
		return err
	}

	db, err := sqlite.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck // read-only handle for the snapshot

	if _, err := db.DB().ExecContext(ctx, `VACUUM INTO $1`, dest); err != nil {
		return fmt.Errorf("storage: exporting snapshot: %w", err)
	}
	return nil
}
