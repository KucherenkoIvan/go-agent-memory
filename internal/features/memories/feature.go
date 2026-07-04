// Package memories is the feature: the Memory aggregate, its use-cases, and
// the Service facade every face (MCP, CLI, TUI) is built on. The facade has
// a local implementation today and a gRPC-client one in the hosted phase —
// faces never know which they got.
package memories

import (
	"context"

	"github.com/KucherenkoIvan/go-kernel/ddd"
	"github.com/KucherenkoIvan/go-kernel/events"
	kernelsqlite "github.com/KucherenkoIvan/go-kernel/sqlite"

	sqliteadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/sqlite"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// Service is the application facade the faces depend on.
type Service interface {
	Store(ctx context.Context, in managememories.StoreInput) (domain.MemoryID, error)
	Search(ctx context.Context, filters ports.SearchFilters) ([]domain.SearchResult, error)
	Get(ctx context.Context, id domain.MemoryID) (*domain.MemoryReadModel, error)
	Rate(ctx context.Context, id domain.MemoryID, up bool) error
	Recall(ctx context.Context, keywords []string, budgetChars int) (string, error)
}

// New wires the local Service: port → adapter, then use-cases.
func New(db *kernelsqlite.Client, pub *events.ChannelPublisher) Service {
	var repo ports.MemoryRepository = sqliteadapter.NewMemoryRepository(db)
	var reader ports.MemoryReader = sqliteadapter.NewMemoryReader(db)
	var producer ports.MemoryEventProducer = pub
	var txManager ddd.TxManager = db

	return &localService{
		store:  managememories.NewStoreCommand(txManager, ddd.UUIDv7Generator{}, ddd.SystemClock{}, repo, producer),
		rate:   managememories.NewRateCommand(txManager, repo, producer),
		search: managememories.NewSearchQuery(reader),
		get:    managememories.NewGetQuery(reader),
		recall: managememories.NewRecallQuery(reader),
	}
}

type localService struct {
	store  *managememories.StoreCommand
	rate   *managememories.RateCommand
	search *managememories.SearchQuery
	get    *managememories.GetQuery
	recall *managememories.RecallQuery
}

func (s *localService) Store(ctx context.Context, in managememories.StoreInput) (domain.MemoryID, error) {
	return s.store.Execute(ctx, in)
}

func (s *localService) Search(ctx context.Context, filters ports.SearchFilters) ([]domain.SearchResult, error) {
	return s.search.Execute(ctx, filters)
}

func (s *localService) Get(ctx context.Context, id domain.MemoryID) (*domain.MemoryReadModel, error) {
	return s.get.Execute(ctx, id)
}

func (s *localService) Rate(ctx context.Context, id domain.MemoryID, up bool) error {
	return s.rate.Execute(ctx, id, up)
}

func (s *localService) Recall(ctx context.Context, keywords []string, budgetChars int) (string, error) {
	return s.recall.Execute(ctx, keywords, budgetChars)
}
