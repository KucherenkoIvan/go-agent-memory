package managememories

import (
	"context"
	"fmt"
	"strings"

	"github.com/KucherenkoIvan/go-kernel/ddd"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 200
	recallSearchLimit  = 50
	defaultRecallChars = 4000
)

// SearchQuery — ranked summaries; agents scan these, then Get what matters.
type SearchQuery struct {
	reader ports.MemoryReader
}

func NewSearchQuery(reader ports.MemoryReader) *SearchQuery {
	return &SearchQuery{reader: reader}
}

func (q *SearchQuery) Execute(ctx context.Context, filters ports.SearchFilters) (domain.SearchPage, error) {
	if filters.Limit <= 0 {
		filters.Limit = defaultSearchLimit
	}
	filters.Limit = min(filters.Limit, maxSearchLimit)
	filters.Offset = max(filters.Offset, 0)
	if !ports.ValidOrder(filters.Order) {
		return domain.SearchPage{}, fmt.Errorf("invalid_order: %q — one of created_asc, created_desc, rating_asc, rating_desc, reads_asc, reads_desc, or empty for relevance", filters.Order)
	}
	filters.Keywords = domain.NormalizeKeywords(filters.Keywords)
	filters.KeywordsAny = domain.NormalizeKeywords(filters.KeywordsAny)
	return q.reader.Search(ctx, ddd.NoTransaction, filters)
}

// GetQuery — the full memory; reading bumps the implicit access signal.
type GetQuery struct {
	reader ports.MemoryReader
}

func NewGetQuery(reader ports.MemoryReader) *GetQuery {
	return &GetQuery{reader: reader}
}

func (q *GetQuery) Execute(ctx context.Context, id domain.MemoryID) (*domain.MemoryReadModel, error) {
	model, err := q.reader.GetFull(ctx, ddd.NoTransaction, id, true)
	if err != nil {
		return nil, err
	}
	if model == nil {
		return nil, &domain.MemoryNotFoundError{}
	}
	return model, nil
}

// RecallQuery — the one-call session bootstrap: top-ranked memories for the
// keywords, assembled into a markdown block trimmed to a character budget.
type RecallQuery struct {
	reader ports.MemoryReader
}

func NewRecallQuery(reader ports.MemoryReader) *RecallQuery {
	return &RecallQuery{reader: reader}
}

func (q *RecallQuery) Execute(ctx context.Context, keywords []string, text string, budgetChars int) (string, error) {
	if budgetChars <= 0 {
		budgetChars = defaultRecallChars
	}

	// OR semantics on purpose: session bootstrap throws candidate keywords
	// at the store — any match qualifies, more matches rank higher. An
	// optional full-text query narrows on top.
	page, err := q.reader.Search(ctx, ddd.NoTransaction, ports.SearchFilters{
		Query:       text,
		KeywordsAny: domain.NormalizeKeywords(keywords),
		Limit:       recallSearchLimit,
		SkipTotal:   true, // pack never shows counts
	})
	if err != nil {
		return "", err
	}
	// one batch read for the whole candidate page; no access bumps — pack
	// previews ranked results, it does not read them (only explicit gets
	// feed the implicit-usefulness signal)
	ids := make([]domain.MemoryID, len(page.Results))
	for i, result := range page.Results {
		ids[i] = domain.MemoryID(result.ID)
	}
	models, err := q.reader.GetMany(ctx, ddd.NoTransaction, ids)
	if err != nil {
		return "", err
	}
	byID := make(map[string]*domain.MemoryReadModel, len(models))
	for i := range models {
		byID[models[i].ID] = &models[i]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Recalled memories (%s)\n\n", strings.Join(keywords, ", "))
	included := 0

	for _, result := range page.Results {
		full := byID[result.ID]
		if full == nil {
			continue
		}

		entry := fmt.Sprintf("## [%s] %s\n_id: %s · %s · keywords: %s_\n\n%s\n\n",
			full.Kind, full.Summary, full.ID,
			full.CreatedAt.Format("2006-01-02"), strings.Join(full.Keywords, ", "),
			full.Content)

		if b.Len()+len(entry) > budgetChars && included > 0 {
			break
		}
		b.WriteString(entry)
		included++
		if b.Len() >= budgetChars {
			break
		}
	}

	if included == 0 {
		return fmt.Sprintf("# Recalled memories (%s)\n\nNothing stored for these keywords yet.\n", strings.Join(keywords, ", ")), nil
	}
	return b.String(), nil
}
