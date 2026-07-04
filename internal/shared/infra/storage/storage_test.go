package storage_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

// The deployment model: multiple agmem processes on one file. Two stores
// open the same database concurrently and both write — WAL + immediate
// transactions (kernel defaults) make this safe.
func TestMultiProcessModel_ConcurrentStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	ctx := context.Background()

	first, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })

	second, err := storage.Open(ctx, path) // second "process" — no lock, no error
	if err != nil {
		t.Fatalf("concurrent open must succeed: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	// both write concurrently into the real schema
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i, store := range []*storage.Store{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 10 {
				_, err := store.DB.DB().ExecContext(ctx, `
					INSERT INTO memories (id, content, summary, kind, keywords, source, created_at)
					VALUES ($1, 'c', 's', 'fact', ' k ', 'test', '2026-01-01T00:00:00Z')`,
					// unique ids across both writers
					filepath.Join("id", string(rune('a'+i)), string(rune('0'+j))))
				if err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent write failed: %v", err)
	}

	var n int
	if err := first.DB.DB().QueryRowContext(ctx, `SELECT count(*) FROM memories`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 20 {
		t.Fatalf("rows = %d, want 20", n)
	}

	// FTS triggers indexed writes from BOTH stores
	var matches int
	if err := second.DB.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM memories_fts WHERE memories_fts MATCH 'c'`).Scan(&matches); err != nil {
		t.Fatal(err)
	}
	if matches != 20 {
		t.Fatalf("fts matches = %d, want 20 (triggers must index all writers)", matches)
	}
}
