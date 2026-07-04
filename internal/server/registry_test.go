package server_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/server"
)

func TestSpaceRegistry_IsolatesSpacesOnDisk(t *testing.T) {
	dir := t.TempDir()
	registry := server.NewSpaceRegistry(dir)
	ctx := context.Background()
	t.Cleanup(func() { _ = registry.Close(ctx) })

	svcA, err := registry.ServiceFor(ctx, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	svcB, err := registry.ServiceFor(ctx, "team-b")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svcA.Store(ctx, managememories.StoreInput{
		Content: "a only", Summary: "s", Kind: "fact", Keywords: []string{"k"}, Source: "t",
	}); err != nil {
		t.Fatal(err)
	}

	// two real files, and b sees nothing of a
	for _, name := range []string{"team-a.db", "team-b.db"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("space file %s: %v", name, err)
		}
	}
	results, err := svcB.Search(ctx, ports.SearchFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("space b must be empty: %+v", results)
	}
}

func TestSpaceRegistry_SameSpaceSameHandle(t *testing.T) {
	registry := server.NewSpaceRegistry(t.TempDir())
	ctx := context.Background()
	t.Cleanup(func() { _ = registry.Close(ctx) })

	const workers = 8
	services := make([]any, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc, err := registry.ServiceFor(ctx, "same")
			if err != nil {
				t.Error(err)
				return
			}
			services[i] = svc
		}()
	}
	wg.Wait()

	for i := 1; i < workers; i++ {
		if services[i] != services[0] {
			t.Fatal("concurrent first-open must converge on one handle")
		}
	}
}

func TestSpaceRegistry_RejectsUnsafeNames(t *testing.T) {
	registry := server.NewSpaceRegistry(t.TempDir())
	t.Cleanup(func() { _ = registry.Close(context.Background()) })

	for _, space := range []string{"../escape", "a/b", "", "UPPER"} {
		if _, err := registry.ServiceFor(context.Background(), space); err == nil {
			t.Errorf("space %q must be rejected", space)
		}
	}
}
