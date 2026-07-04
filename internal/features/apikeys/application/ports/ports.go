// Package ports defines the apikeys feature's infrastructure contracts.
package ports

import (
	"context"

	"github.com/KucherenkoIvan/go-kernel/ddd"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/domain"
)

// APIKeyRepository persists keys and their spaces in the control plane.
type APIKeyRepository interface {
	// Save upserts the key and ensures its space row exists.
	Save(ctx context.Context, tx ddd.Transaction, key *domain.APIKey) error
	// GetByID returns (nil, nil) when the key does not exist.
	GetByID(ctx context.Context, tx ddd.Transaction, id domain.APIKeyID) (*domain.APIKey, error)
	// GetByHash returns (nil, nil) when no key matches the token hash.
	GetByHash(ctx context.Context, tx ddd.Transaction, tokenHash string) (*domain.APIKey, error)
	List(ctx context.Context, tx ddd.Transaction) ([]domain.APIKeyView, error)
	ListSpaces(ctx context.Context, tx ddd.Transaction) ([]domain.SpaceView, error)
}
