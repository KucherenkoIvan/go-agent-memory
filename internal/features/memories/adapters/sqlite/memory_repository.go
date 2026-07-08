// Package sqlite implements the memories ports against the embedded
// database. Keywords are stored space-separated with padding (the source of
// truth, feeding FTS and the read models) and mirrored into the
// memory_keywords index table for indexed search; the mirror is maintained
// here, in the write transaction. FTS5 indexing is handled by schema
// triggers.
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

	// mirror keywords into the index table, keyed by the integer pk.
	// OR IGNORE keeps re-saves (rate, supersede) idempotent — keywords are
	// immutable on the aggregate
	if len(snap.Keywords) > 0 {
		var pk int64
		if err := r.db.Resolve(tx).QueryRowContext(ctx,
			`SELECT pk FROM memories WHERE id = $1`, string(snap.ID)).Scan(&pk); err != nil {
			return fmt.Errorf("resolving memory pk: %w", err)
		}
		values := make([]string, len(snap.Keywords))
		args := make([]any, 0, len(snap.Keywords)*2)
		for i, kw := range snap.Keywords {
			values[i] = fmt.Sprintf("($%d, $%d)", 2*i+1, 2*i+2)
			args = append(args, kw, pk)
		}
		_, err = r.db.Resolve(tx).ExecContext(ctx,
			"INSERT OR IGNORE INTO memory_keywords (keyword, memory_pk) VALUES "+strings.Join(values, ", "),
			args...)
		if err != nil {
			return fmt.Errorf("saving memory keywords: %w", err)
		}
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

func (r *MemoryRepository) Delete(ctx context.Context, tx ddd.Transaction, id domain.MemoryID) error {
	// the memories_fts_delete trigger removes the FTS row; the keyword
	// mirror is repository-maintained, so it is cleaned here
	if _, err := r.db.Resolve(tx).ExecContext(ctx,
		`DELETE FROM memory_keywords WHERE memory_pk = (SELECT pk FROM memories WHERE id = $1)`, string(id)); err != nil {
		return fmt.Errorf("deleting memory keywords: %w", err)
	}
	if _, err := r.db.Resolve(tx).ExecContext(ctx, `DELETE FROM memories WHERE id = $1`, string(id)); err != nil {
		return fmt.Errorf("deleting memory: %w", err)
	}
	return nil
}

func packKeywords(keywords []string) string {
	return " " + strings.Join(keywords, " ") + " "
}

func unpackKeywords(packed string) []string {
	return strings.Fields(packed)
}
