package tui

import (
	"context"
	"time"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// fakeService is the hand-written test double for the facade: canned
// results, recorded calls, injectable errors.
type fakeService struct {
	searches []ports.SearchFilters
	rates    []struct {
		ID string
		Up bool
	}
	deletes []string
	stores  []managememories.StoreInput

	results  []domain.SearchResult
	memory   *domain.MemoryReadModel
	storeErr error
	storeID  domain.MemoryID
}

func (f *fakeService) Store(_ context.Context, in managememories.StoreInput) (domain.MemoryID, error) {
	f.stores = append(f.stores, in)
	if f.storeErr != nil {
		return "", f.storeErr
	}
	if f.storeID != "" {
		return f.storeID, nil
	}
	return "new-id", nil
}

func (f *fakeService) Search(_ context.Context, filters ports.SearchFilters) ([]domain.SearchResult, error) {
	f.searches = append(f.searches, filters)
	return f.results, nil
}

func (f *fakeService) Get(_ context.Context, id domain.MemoryID) (*domain.MemoryReadModel, error) {
	if f.memory != nil && f.memory.ID == string(id) {
		return f.memory, nil
	}
	return nil, &domain.MemoryNotFoundError{}
}

func (f *fakeService) Rate(_ context.Context, id domain.MemoryID, up bool) error {
	f.rates = append(f.rates, struct {
		ID string
		Up bool
	}{string(id), up})
	return nil
}

func (f *fakeService) Recall(context.Context, []string, int) (string, error) { return "", nil }

func (f *fakeService) Delete(_ context.Context, id domain.MemoryID) error {
	f.deletes = append(f.deletes, string(id))
	return nil
}

func someResult(id, summary string) domain.SearchResult {
	return domain.SearchResult{
		ID: id, Summary: summary, Kind: "fact",
		Keywords: []string{"k"}, Source: "test", CreatedAt: time.Now(),
	}
}
