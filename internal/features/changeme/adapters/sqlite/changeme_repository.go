// Package sqlite implements the feature's ports against the embedded
// database — the CRUD repository to copy for real aggregates.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/KucherenkoIvan/go-kernel/ddd"
	kernelsqlite "github.com/KucherenkoIvan/go-kernel/sqlite"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/domain"
)

type ChangeMeRepository struct {
	db *kernelsqlite.Client
}

func NewChangeMeRepository(db *kernelsqlite.Client) *ChangeMeRepository {
	return &ChangeMeRepository{db: db}
}

func (r *ChangeMeRepository) Save(ctx context.Context, tx ddd.Transaction, aggregate *domain.ChangeMe) error {
	snap := aggregate.Snapshot()
	_, err := r.db.Resolve(tx).ExecContext(ctx, `
		INSERT INTO changeme (id, name, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name`,
		string(snap.ID), snap.Name, snap.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("saving changeme: %w", err)
	}
	return nil
}

func (r *ChangeMeRepository) GetByID(ctx context.Context, tx ddd.Transaction, id domain.ChangeMeID) (*domain.ChangeMe, error) {
	row := r.db.Resolve(tx).QueryRowContext(ctx,
		`SELECT id, name, created_at FROM changeme WHERE id = $1`, string(id))

	var (
		rawID, createdAt string
		snap             domain.ChangeMeSnapshot
	)
	err := row.Scan(&rawID, &snap.Name, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("selecting changeme: %w", err)
	}

	snap.ID = domain.ChangeMeID(rawID)
	if snap.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, fmt.Errorf("parsing changeme created_at: %w", err)
	}
	return domain.RestoreChangeMe(snap), nil
}

func (r *ChangeMeRepository) Delete(ctx context.Context, tx ddd.Transaction, id domain.ChangeMeID) error {
	_, err := r.db.Resolve(tx).ExecContext(ctx, `DELETE FROM changeme WHERE id = $1`, string(id))
	if err != nil {
		return fmt.Errorf("deleting changeme: %w", err)
	}
	return nil
}
