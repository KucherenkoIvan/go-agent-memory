// Package ports defines the feature's infrastructure contracts. Adapters
// implement them; feature.go binds them.
package ports

import (
	"context"

	"github.com/KucherenkoIvan/go-kernel/ddd"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/domain"
)

// ChangeMeRepository persists the aggregate. One repository per aggregate.
type ChangeMeRepository interface {
	// Save inserts or updates (upsert).
	Save(ctx context.Context, tx ddd.Transaction, aggregate *domain.ChangeMe) error
	// GetByID returns (nil, nil) when the aggregate does not exist.
	GetByID(ctx context.Context, tx ddd.Transaction, id domain.ChangeMeID) (*domain.ChangeMe, error)
	Delete(ctx context.Context, tx ddd.Transaction, id domain.ChangeMeID) error
}

// ChangeMeReader serves the queries — read-models only, never aggregates.
type ChangeMeReader interface {
	// GetByID returns (nil, nil) when the read-model does not exist.
	GetByID(ctx context.Context, tx ddd.Transaction, id domain.ChangeMeID) (*domain.ChangeMeReadModel, error)
	List(ctx context.Context, tx ddd.Transaction) ([]domain.ChangeMeReadModel, error)
}

// ChangeMeEventProducer publishes domain events. The shape matches
// events.Producer, so events.ChannelPublisher satisfies it directly — and an
// outbox adapter can replace it without touching use-cases.
type ChangeMeEventProducer interface {
	Publish(ctx context.Context, tx ddd.Transaction, events ...ddd.DomainEvent) error
}
