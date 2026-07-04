// Package sqlite implements the memories ports against the embedded
// database. Keywords are stored space-separated with padding so exact
// keyword filters are simple substring checks; FTS5 indexing is handled by
// schema triggers.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/KucherenkoIvan/go-kernel/ddd"
	kernelsqlite "github.com/KucherenkoIvan/go-kernel/sqlite"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

type MemoryRepository struct {
	db *kernelsqlite.Client
}

func NewMemoryRepository(db *kernelsqlite.Client) *MemoryRepository {
	return &MemoryRepository{db: db}
}

func (r *MemoryRepository) Save(ctx context.Context, tx ddd.Transaction, memory *domain.Memory) error {
	snap := memory.Snapshot()

	var expiresAt any
	if exp := memory.ExpiresAt(); !exp.IsZero() {
		expiresAt = exp.UTC().Format(time.RFC3339Nano)
	}
	var supersededBy any
	if snap.SupersededBy != "" {
		supersededBy = string(snap.SupersededBy)
	}

	_, err := r.db.Resolve(tx).ExecContext(ctx, `
		INSERT INTO memories (id, content, summary, kind, keywords, source, ttl_hours, expires_at, created_at, superseded_by, votes_up, votes_down)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			superseded_by = excluded.superseded_by,
			votes_up = excluded.votes_up,
			votes_down = excluded.votes_down`,
		string(snap.ID), snap.Content, snap.Summary, string(snap.Kind), packKeywords(snap.Keywords),
		snap.Source, snap.TTLHours, expiresAt, snap.CreatedAt.UTC().Format(time.RFC3339Nano),
		supersededBy, snap.VotesUp, snap.VotesDown)
	if err != nil {
		return fmt.Errorf("saving memory: %w", err)
	}
	return nil
}

func (r *MemoryRepository) GetByID(ctx context.Context, tx ddd.Transaction, id domain.MemoryID) (*domain.Memory, error) {
	row := r.db.Resolve(tx).QueryRowContext(ctx, `
		SELECT id, content, summary, kind, keywords, source, ttl_hours, created_at, superseded_by, votes_up, votes_down
		FROM memories WHERE id = $1`, string(id))

	var (
		snap         domain.MemorySnapshot
		rawID, kind  string
		keywords     string
		createdAt    string
		supersededBy sql.NullString
	)
	err := row.Scan(&rawID, &snap.Content, &snap.Summary, &kind, &keywords, &snap.Source,
		&snap.TTLHours, &createdAt, &supersededBy, &snap.VotesUp, &snap.VotesDown)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("selecting memory: %w", err)
	}

	snap.ID = domain.MemoryID(rawID)
	snap.Kind = domain.Kind(kind)
	snap.Keywords = unpackKeywords(keywords)
	snap.SupersededBy = domain.MemoryID(supersededBy.String)
	if snap.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, fmt.Errorf("parsing memory created_at: %w", err)
	}
	return domain.RestoreMemory(snap), nil
}

func packKeywords(keywords []string) string {
	return " " + strings.Join(keywords, " ") + " "
}

func unpackKeywords(packed string) []string {
	return strings.Fields(packed)
}
