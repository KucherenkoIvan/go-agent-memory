// Package ports defines the memories feature's infrastructure contracts.
package ports

import (
	"context"
	"time"

	"github.com/KucherenkoIvan/go-kernel/ddd"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// MemoryRepository persists the aggregate.
type MemoryRepository interface {
	Save(ctx context.Context, tx ddd.Transaction, memory *domain.Memory) error
	// GetByID returns (nil, nil) when the memory does not exist.
	GetByID(ctx context.Context, tx ddd.Transaction, id domain.MemoryID) (*domain.Memory, error)
	// Delete removes the row for good — human prune only, never agents.
	Delete(ctx context.Context, tx ddd.Transaction, id domain.MemoryID) error
}

// SearchFilters compose with AND. Zero values mean "no filter".
type SearchFilters struct {
	Query    string   // FTS5 match over summary+content+keywords
	Keywords []string // every keyword must be present
	// KeywordsAny keeps memories carrying at least one of these keywords;
	// each additional match boosts rank. Recall's keyword semantics.
	KeywordsAny []string
	Kind        string
	Since       time.Time
	Until       time.Time
	Limit       int
	// Offset skips ranked rows for pagination; ordering is stable (id
	// tiebreak), so pages never duplicate or drop rows.
	Offset int
	// Order overrides relevance ranking with a named display order; empty
	// keeps the score ranking. See the Order* constants.
	Order string
	// IncludeDead includes superseded and expired memories.
	IncludeDead bool
}

// Search orders — SearchFilters.Order values. Empty = relevance ranking.
const (
	OrderCreatedAsc  = "created_asc"
	OrderCreatedDesc = "created_desc"
	OrderRatingAsc   = "rating_asc"
	OrderRatingDesc  = "rating_desc"
	OrderReadsAsc    = "reads_asc"
	OrderReadsDesc   = "reads_desc"
)

// ValidOrder reports whether order names a supported display order.
func ValidOrder(order string) bool {
	switch order {
	case "", OrderCreatedAsc, OrderCreatedDesc, OrderRatingAsc,
		OrderRatingDesc, OrderReadsAsc, OrderReadsDesc:
		return true
	}
	return false
}

// MemoryReader serves the queries: ranked search and full reads.
type MemoryReader interface {
	Search(ctx context.Context, tx ddd.Transaction, filters SearchFilters) (domain.SearchPage, error)
	// GetFull returns (nil, nil) when the memory does not exist. bumpAccess
	// records the read as an implicit usefulness signal (fire-and-forget,
	// no transaction on the read path).
	GetFull(ctx context.Context, tx ddd.Transaction, id domain.MemoryID, bumpAccess bool) (*domain.MemoryReadModel, error)
}

// MemoryEventProducer publishes domain events (shape matches
// events.Producer, so events.ChannelPublisher satisfies it directly).
type MemoryEventProducer interface {
	Publish(ctx context.Context, tx ddd.Transaction, events ...ddd.DomainEvent) error
}
