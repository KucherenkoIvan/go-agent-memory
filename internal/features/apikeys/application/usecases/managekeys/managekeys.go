// Package managekeys holds the apikeys use-cases: mint, revoke, list, and
// the per-request authenticate lookup.
package managekeys

import (
	"context"

	"github.com/KucherenkoIvan/go-kernel/ddd"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/domain"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/identity"
)

// CreateResult carries the raw token — the only moment it exists in the
// clear. Callers print it once; only the hash is stored.
type CreateResult struct {
	Key      domain.APIKeyView `json:"key"`
	RawToken string            `json:"rawToken"`
}

// CreateKeyCommand mints a key into a space (creating the space implicitly).
type CreateKeyCommand struct {
	txManager ddd.TxManager
	ids       ddd.IDGenerator
	clock     ddd.Clock
	repo      ports.APIKeyRepository
}

func NewCreateKeyCommand(txManager ddd.TxManager, ids ddd.IDGenerator, clock ddd.Clock, repo ports.APIKeyRepository) *CreateKeyCommand {
	return &CreateKeyCommand{txManager: txManager, ids: ids, clock: clock, repo: repo}
}

func (c *CreateKeyCommand) Execute(ctx context.Context, name, space string) (CreateResult, error) {
	raw, hash, prefix, err := domain.GenerateToken()
	if err != nil {
		return CreateResult{}, err
	}
	key, err := domain.NewAPIKey(domain.APIKeyID(c.ids.NewID()), name, space, hash, prefix, c.clock.Now().UTC())
	if err != nil {
		return CreateResult{}, err
	}

	err = c.txManager.WithinTx(ctx, func(tx ddd.Transaction) error {
		return c.repo.Save(ctx, tx, key)
	})
	if err != nil {
		return CreateResult{}, err
	}

	snap := key.Snapshot()
	return CreateResult{
		Key: domain.APIKeyView{
			ID: string(snap.ID), Name: snap.Name, Space: snap.Space,
			Prefix: snap.Prefix, CreatedAt: snap.CreatedAt,
		},
		RawToken: raw,
	}, nil
}

// RevokeKeyCommand deactivates a key; other keys into the space keep working.
type RevokeKeyCommand struct {
	txManager ddd.TxManager
	clock     ddd.Clock
	repo      ports.APIKeyRepository
}

func NewRevokeKeyCommand(txManager ddd.TxManager, clock ddd.Clock, repo ports.APIKeyRepository) *RevokeKeyCommand {
	return &RevokeKeyCommand{txManager: txManager, clock: clock, repo: repo}
}

func (c *RevokeKeyCommand) Execute(ctx context.Context, id domain.APIKeyID) error {
	return c.txManager.WithinTx(ctx, func(tx ddd.Transaction) error {
		key, err := c.repo.GetByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if key == nil {
			return &domain.KeyNotFoundError{}
		}
		key.Revoke(c.clock.Now().UTC())
		return c.repo.Save(ctx, tx, key)
	})
}

// ListKeysQuery — every key, active and revoked, without secrets.
type ListKeysQuery struct {
	repo ports.APIKeyRepository
}

func NewListKeysQuery(repo ports.APIKeyRepository) *ListKeysQuery {
	return &ListKeysQuery{repo: repo}
}

func (q *ListKeysQuery) Execute(ctx context.Context) ([]domain.APIKeyView, error) {
	return q.repo.List(ctx, ddd.NoTransaction)
}

// ListSpacesQuery — spaces with their active key counts.
type ListSpacesQuery struct {
	repo ports.APIKeyRepository
}

func NewListSpacesQuery(repo ports.APIKeyRepository) *ListSpacesQuery {
	return &ListSpacesQuery{repo: repo}
}

func (q *ListSpacesQuery) Execute(ctx context.Context) ([]domain.SpaceView, error) {
	return q.repo.ListSpaces(ctx, ddd.NoTransaction)
}

// AuthenticateQuery resolves a raw token to the caller's identity. Unknown
// and revoked tokens are indistinguishable to the caller.
type AuthenticateQuery struct {
	repo ports.APIKeyRepository
}

func NewAuthenticateQuery(repo ports.APIKeyRepository) *AuthenticateQuery {
	return &AuthenticateQuery{repo: repo}
}

func (q *AuthenticateQuery) Execute(ctx context.Context, rawToken string) (identity.Identity, error) {
	key, err := q.repo.GetByHash(ctx, ddd.NoTransaction, domain.HashToken(rawToken))
	if err != nil {
		return identity.Identity{}, err
	}
	if key == nil || key.Revoked() {
		return identity.Identity{}, &domain.InvalidAPIKeyError{}
	}
	return identity.Identity{KeyID: string(key.ID()), KeyName: key.Name(), Space: key.Space()}, nil
}
