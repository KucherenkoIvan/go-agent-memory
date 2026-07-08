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

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// Ranking weights — the one place retrieval tuning happens. Explicit
// feedback (votes) dominates the implicit access signal.
const (
	weightFTS     = 2.0  // relevance of the text match (bm25, lower-is-better → negated)
	weightVotes   = 1.0  // votes_up - votes_down
	weightAccess  = 0.05 // per access, capped
	accessCap     = 20
	weightRecency = 3.0 // decays with age: w / (1 + age_days/30)
	weightKeyword = 1.5 // per matched KeywordsAny keyword — more matches rank higher
)

type MemoryReader struct {
	db *kernelsqlite.Client
}

func NewMemoryReader(db *kernelsqlite.Client) *MemoryReader {
	return &MemoryReader{db: db}
}

const searchColumns = `m.id, m.summary, m.kind, m.keywords, m.source, m.created_at, m.expires_at, m.votes_up, m.votes_down, m.access_count`

func (r *MemoryReader) Search(ctx context.Context, tx ddd.Transaction, f ports.SearchFilters) ([]domain.SearchResult, error) {
	var (
		where []string
		args  []any
		arg   = func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	)

	score := fmt.Sprintf(
		"(m.votes_up - m.votes_down) * %v + min(m.access_count, %d) * %v + %v / (1.0 + (julianday('now') - julianday(m.created_at)) / 30.0)",
		weightVotes, accessCap, weightAccess, weightRecency)

	var from, snippet string
	if f.Query != "" {
		from = `memories_fts f JOIN memories m ON m.rowid = f.rowid`
		where = append(where, "memories_fts MATCH "+arg(ftsQuery(f.Query)))
		score = fmt.Sprintf("-bm25(memories_fts) * %v + ", weightFTS) + score
		snippet = `snippet(memories_fts, 1, '>>', '<<', ' … ', 12)`
	} else {
		from = `memories m`
		snippet = `''`
	}

	if !f.IncludeDead {
		where = append(where, "m.superseded_by IS NULL")
		where = append(where, "(m.expires_at IS NULL OR m.expires_at > "+arg(nowRFC3339())+")")
	}
	for _, kw := range f.Keywords {
		where = append(where, "m.id IN (SELECT memory_id FROM memory_keywords WHERE keyword = "+arg(kw)+")")
	}
	if len(f.KeywordsAny) > 0 {
		// index probe instead of scanning every row's keywords string: the
		// join filters (≥1 keyword must match) and its COUNT doubles as the
		// match-count ranking signal
		placeholders := make([]string, 0, len(f.KeywordsAny))
		for _, kw := range f.KeywordsAny {
			placeholders = append(placeholders, arg(kw))
		}
		from += " JOIN (SELECT memory_id, COUNT(*) AS matches FROM memory_keywords WHERE keyword IN (" +
			strings.Join(placeholders, ", ") + ") GROUP BY memory_id) mk ON mk.memory_id = m.id"
		score += fmt.Sprintf(" + mk.matches * %v", weightKeyword)
	}
	if f.Kind != "" {
		where = append(where, "m.kind = "+arg(f.Kind))
	}
	if !f.Since.IsZero() {
		where = append(where, "m.created_at >= "+arg(f.Since.UTC().Format(time.RFC3339Nano)))
	}
	if !f.Until.IsZero() {
		where = append(where, "m.created_at <= "+arg(f.Until.UTC().Format(time.RFC3339Nano)))
	}

	query := fmt.Sprintf(`SELECT %s, %s AS snippet, (%s) AS score FROM %s`, searchColumns, snippet, score, from)
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY score DESC LIMIT " + arg(f.Limit)

	rows, err := r.db.Resolve(tx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("searching memories: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only cursor

	results := []domain.SearchResult{}
	for rows.Next() {
		var (
			res       domain.SearchResult
			keywords  string
			createdAt string
			expiresAt sql.NullString
		)
		if err := rows.Scan(&res.ID, &res.Summary, &res.Kind, &keywords, &res.Source,
			&createdAt, &expiresAt, &res.VotesUp, &res.VotesDown, &res.AccessCount,
			&res.Snippet, &res.Score); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		res.Keywords = unpackKeywords(keywords)
		if res.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parsing created_at: %w", err)
		}
		if expiresAt.Valid {
			t, err := time.Parse(time.RFC3339Nano, expiresAt.String)
			if err != nil {
				return nil, fmt.Errorf("parsing expires_at: %w", err)
			}
			res.ExpiresAt = &t
		}
		results = append(results, res)
	}
	return results, rows.Err()
}

func (r *MemoryReader) GetFull(ctx context.Context, tx ddd.Transaction, id domain.MemoryID, bumpAccess bool) (*domain.MemoryReadModel, error) {
	row := r.db.Resolve(tx).QueryRowContext(ctx, `
		SELECT id, content, summary, kind, keywords, source, created_at, expires_at, superseded_by, votes_up, votes_down, access_count
		FROM memories WHERE id = $1`, string(id))

	var (
		model                   domain.MemoryReadModel
		keywords, createdAt     string
		expiresAt, supersededBy sql.NullString
	)
	err := row.Scan(&model.ID, &model.Content, &model.Summary, &model.Kind, &keywords, &model.Source,
		&createdAt, &expiresAt, &supersededBy, &model.VotesUp, &model.VotesDown, &model.AccessCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading memory: %w", err)
	}

	model.Keywords = unpackKeywords(keywords)
	model.SupersededBy = supersededBy.String
	if model.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}
	if expiresAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, expiresAt.String)
		if err != nil {
			return nil, fmt.Errorf("parsing expires_at: %w", err)
		}
		model.ExpiresAt = &t
	}

	if bumpAccess {
		// fire-and-forget implicit signal — no transaction on the read path
		_, _ = r.db.Resolve(tx).ExecContext(ctx,
			`UPDATE memories SET access_count = access_count + 1, last_accessed_at = $1 WHERE id = $2`,
			nowRFC3339(), string(id))
		model.AccessCount++
	}
	return &model, nil
}

// ftsQuery turns free text into an FTS5 query: each term quoted (so user
// input can't inject FTS syntax) and prefix-matched.
func ftsQuery(raw string) string {
	terms := strings.Fields(raw)
	quoted := make([]string, 0, len(terms))
	for _, t := range terms {
		quoted = append(quoted, `"`+strings.ReplaceAll(t, `"`, ``)+`"*`)
	}
	return strings.Join(quoted, " ")
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
