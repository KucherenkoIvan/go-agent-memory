// Package sqlite implements the apikeys ports against the server
// control-plane database (keys.db).
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/KucherenkoIvan/go-kernel/ddd"
	kernelsqlite "github.com/KucherenkoIvan/go-kernel/sqlite"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/domain"
)

type APIKeyRepository struct {
	db *kernelsqlite.Client
}

func NewAPIKeyRepository(db *kernelsqlite.Client) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

func (r *APIKeyRepository) Save(ctx context.Context, tx ddd.Transaction, key *domain.APIKey) error {
	snap := key.Snapshot()

	if _, err := r.db.Resolve(tx).ExecContext(ctx, `
		INSERT INTO spaces (name, created_at) VALUES ($1, $2)
		ON CONFLICT (name) DO NOTHING`,
		snap.Space, snap.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("saving space: %w", err)
	}

	var revokedAt any
	if !snap.RevokedAt.IsZero() {
		revokedAt = snap.RevokedAt.UTC().Format(time.RFC3339Nano)
	}
	if _, err := r.db.Resolve(tx).ExecContext(ctx, `
		INSERT INTO api_keys (id, name, space, token_hash, prefix, created_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET revoked_at = excluded.revoked_at`,
		string(snap.ID), snap.Name, snap.Space, snap.TokenHash, snap.Prefix,
		snap.CreatedAt.UTC().Format(time.RFC3339Nano), revokedAt); err != nil {
		return fmt.Errorf("saving api key: %w", err)
	}
	return nil
}

func (r *APIKeyRepository) GetByID(ctx context.Context, tx ddd.Transaction, id domain.APIKeyID) (*domain.APIKey, error) {
	return r.get(ctx, tx, `id = $1`, string(id))
}

func (r *APIKeyRepository) GetByHash(ctx context.Context, tx ddd.Transaction, tokenHash string) (*domain.APIKey, error) {
	return r.get(ctx, tx, `token_hash = $1`, tokenHash)
}

func (r *APIKeyRepository) get(ctx context.Context, tx ddd.Transaction, where string, arg any) (*domain.APIKey, error) {
	row := r.db.Resolve(tx).QueryRowContext(ctx, `
		SELECT id, name, space, token_hash, prefix, created_at, revoked_at
		FROM api_keys WHERE `+where, arg)

	var (
		snap             domain.APIKeySnapshot
		rawID, createdAt string
		revokedAt        sql.NullString
	)
	err := row.Scan(&rawID, &snap.Name, &snap.Space, &snap.TokenHash, &snap.Prefix, &createdAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("selecting api key: %w", err)
	}

	snap.ID = domain.APIKeyID(rawID)
	if snap.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, fmt.Errorf("parsing api key created_at: %w", err)
	}
	if revokedAt.Valid {
		if snap.RevokedAt, err = time.Parse(time.RFC3339Nano, revokedAt.String); err != nil {
			return nil, fmt.Errorf("parsing api key revoked_at: %w", err)
		}
	}
	return domain.RestoreAPIKey(snap), nil
}

func (r *APIKeyRepository) List(ctx context.Context, tx ddd.Transaction) ([]domain.APIKeyView, error) {
	rows, err := r.db.Resolve(tx).QueryContext(ctx, `
		SELECT id, name, space, prefix, created_at, revoked_at
		FROM api_keys ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("listing api keys: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only cursor

	views := []domain.APIKeyView{}
	for rows.Next() {
		var (
			view      domain.APIKeyView
			createdAt string
			revokedAt sql.NullString
		)
		if err := rows.Scan(&view.ID, &view.Name, &view.Space, &view.Prefix, &createdAt, &revokedAt); err != nil {
			return nil, fmt.Errorf("scanning api key: %w", err)
		}
		if view.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parsing api key created_at: %w", err)
		}
		if revokedAt.Valid {
			t, err := time.Parse(time.RFC3339Nano, revokedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parsing api key revoked_at: %w", err)
			}
			view.RevokedAt = &t
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

func (r *APIKeyRepository) ListSpaces(ctx context.Context, tx ddd.Transaction) ([]domain.SpaceView, error) {
	rows, err := r.db.Resolve(tx).QueryContext(ctx, `
		SELECT s.name, s.created_at, count(k.id)
		FROM spaces s
		LEFT JOIN api_keys k ON k.space = s.name AND k.revoked_at IS NULL
		GROUP BY s.name, s.created_at
		ORDER BY s.name`)
	if err != nil {
		return nil, fmt.Errorf("listing spaces: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only cursor

	views := []domain.SpaceView{}
	for rows.Next() {
		var (
			view      domain.SpaceView
			createdAt string
		)
		if err := rows.Scan(&view.Name, &createdAt, &view.Keys); err != nil {
			return nil, fmt.Errorf("scanning space: %w", err)
		}
		if view.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parsing space created_at: %w", err)
		}
		views = append(views, view)
	}
	return views, rows.Err()
}
