package storage_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

func TestLock_SecondOpenFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")

	first, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := storage.Open(context.Background(), path); !errors.Is(err, storage.ErrLocked) {
		t.Fatalf("second open: %v, want ErrLocked", err)
	}

	// releasing the lock frees the file for the next process
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open after release: %v", err)
	}
	_ = second.Close()
}
