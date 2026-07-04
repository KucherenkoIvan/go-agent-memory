// Package storage owns the memory database: default location, the
// single-process lock, opening, and migrations.
//
// The lock exists because the kernel's sqlite guidance says one process per
// file — agmem enforces it instead of relaxing it: a flock on <db>.lock is
// taken before opening. flock releases automatically when the process dies,
// so a crashed agent can never wedge the memory; the file carries the owner
// PID purely for the error message.
package storage

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/KucherenkoIvan/go-kernel/sqlite"
	"golang.org/x/sys/unix"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrLocked means another agmem process is using the database.
var ErrLocked = errors.New("memory database is in use by another agmem process")

// DefaultPath is the global memory location: cross-session knowledge is the
// point, so one file for everything unless overridden (--db / AGMEM_DB).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("storage: resolving home: %w", err)
	}
	return filepath.Join(home, ".local", "share", "agmem", "memory.db"), nil
}

// Store is the opened database plus its process lock.
type Store struct {
	DB   *sqlite.Client
	lock *os.File
}

// Open acquires the lock, opens (creating if missing) the database, and
// applies migrations. ":memory:" skips the lock (tests).
func Open(ctx context.Context, path string) (*Store, error) {
	store := &Store{}

	if !strings.Contains(path, ":memory:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("storage: creating data dir: %w", err)
		}
		lock, err := acquireLock(path + ".lock")
		if err != nil {
			return nil, err
		}
		store.lock = lock
	}

	db, err := sqlite.Open(path)
	if err != nil {
		store.releaseLock()
		return nil, err
	}

	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		_ = db.Close()
		store.releaseLock()
		return nil, fmt.Errorf("storage: embedded migrations: %w", err)
	}
	if err := db.Migrate(ctx, migrations); err != nil {
		_ = db.Close()
		store.releaseLock()
		return nil, err
	}

	store.DB = db
	return store, nil
}

// Close closes the database and releases the lock.
func (s *Store) Close() error {
	err := s.DB.Close()
	s.releaseLock()
	return err
}

func acquireLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("storage: opening lock file: %w", err)
	}

	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		owner, _ := os.ReadFile(path)
		_ = file.Close()
		if pid := strings.TrimSpace(string(owner)); pid != "" {
			return nil, fmt.Errorf("%w (pid %s)", ErrLocked, pid)
		}
		return nil, ErrLocked
	}

	// owner PID, for the error message only — the flock is the actual guard
	_ = file.Truncate(0)
	_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
	return file, nil
}

func (s *Store) releaseLock() {
	if s.lock == nil {
		return
	}
	_ = os.Remove(s.lock.Name())
	_ = s.lock.Close() // closing releases the flock
	s.lock = nil
}
