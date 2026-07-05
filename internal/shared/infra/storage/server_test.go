package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

func TestOpenServer_AppliesControlPlaneSchema(t *testing.T) {
	ctx := context.Background()

	store, err := storage.OpenServer(ctx, filepath.Join(t.TempDir(), "keys.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.DB.DB().ExecContext(ctx,
		`INSERT INTO spaces (name, created_at) VALUES ('team-x', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("spaces table missing: %v", err)
	}
	if _, err := store.DB.DB().ExecContext(ctx, `
		INSERT INTO api_keys (id, name, space, token_hash, prefix, created_at)
		VALUES ('k1', 'laptop', 'team-x', 'hash', 'rcl_a1b2c3d4', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("api_keys table missing: %v", err)
	}

	// the memories schema must NOT leak into the control plane
	var n int
	err = store.DB.DB().QueryRowContext(ctx, `SELECT count(*) FROM memories`).Scan(&n)
	if err == nil {
		t.Fatal("keys.db must not carry the memories schema")
	}
}

func TestExportSnapshot_ProducesLocalCompatibleDB(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "space.db")

	store, err := storage.Open(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.DB.DB().ExecContext(ctx, `
		INSERT INTO memories (id, content, summary, kind, keywords, source, created_at)
		VALUES ('m1', 'exported fact', 's', 'fact', ' k ', 'test', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "export", "snapshot.db")
	// source stays open — VACUUM INTO must be live-safe under WAL
	if err := storage.ExportSnapshot(ctx, src, dest); err != nil {
		t.Fatal(err)
	}

	// the snapshot opens as a plain local memory.db and has the data + FTS
	snap, err := storage.Open(ctx, dest)
	if err != nil {
		t.Fatalf("snapshot must open as a local db: %v", err)
	}
	t.Cleanup(func() { _ = snap.Close() })
	var matches int
	if err := snap.DB.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM memories_fts WHERE memories_fts MATCH 'exported'`).Scan(&matches); err != nil {
		t.Fatal(err)
	}
	if matches != 1 {
		t.Fatalf("snapshot fts matches = %d, want 1", matches)
	}

	if err := storage.ExportSnapshot(ctx, src, dest); err == nil {
		t.Fatal("export over an existing destination must fail")
	}
	if err := storage.ExportSnapshot(ctx, filepath.Join(dir, "nope.db"), filepath.Join(dir, "x.db")); err == nil {
		t.Fatal("export of a missing source must fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "x.db")); err == nil {
		t.Fatal("failed export must not leave a destination file")
	}
}
