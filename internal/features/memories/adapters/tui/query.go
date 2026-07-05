package tui

import (
	"context"
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// Humans don't query like LLMs: one box, whitespace-split terms, and the
// terms work three ways at once — exact keywords (OR, like pack), full
// text, and fuzzy against summaries+keywords for typos. Report order:
// keyword hits, then text hits, then fuzzy-only hits.
const (
	layerFetchLimit = 50
	widePoolLimit   = 200
)

// searchSpec is everything the list screen's state contributes to a query.
type searchSpec struct {
	terms       []string
	kind        string
	includeDead bool
	review      bool // review preset wants a wide fetch to filter client-side
}

func (s searchSpec) base() ports.SearchFilters {
	return ports.SearchFilters{Kind: s.kind, IncludeDead: s.includeDead}
}

// runSearch executes the layered query. Three Service calls when terms are
// present; a single timeline fetch otherwise.
func runSearch(ctx context.Context, svc memories.Service, spec searchSpec) ([]domain.SearchResult, error) {
	if len(spec.terms) == 0 {
		timeline := spec.base()
		timeline.Limit = layerFetchLimit
		if spec.review {
			timeline.Limit = widePoolLimit
		}
		return svc.Search(ctx, timeline)
	}

	byKeyword := spec.base()
	byKeyword.KeywordsAny = spec.terms
	byKeyword.Limit = layerFetchLimit
	keywordHits, err := svc.Search(ctx, byKeyword)
	if err != nil {
		return nil, err
	}

	byText := spec.base()
	byText.Query = strings.Join(spec.terms, " ")
	byText.Limit = layerFetchLimit
	textHits, err := svc.Search(ctx, byText)
	if err != nil {
		return nil, err
	}

	widePool := spec.base()
	widePool.Limit = widePoolLimit
	pool, err := svc.Search(ctx, widePool)
	if err != nil {
		return nil, err
	}

	return mergeLayers(keywordHits, textHits, fuzzyRank(pool, spec.terms)), nil
}

// mergeLayers concatenates the layers dropping duplicates — earlier layers
// win, which is exactly the "keywords rank above text above fuzzy" rule.
func mergeLayers(layers ...[]domain.SearchResult) []domain.SearchResult {
	seen := map[string]bool{}
	merged := []domain.SearchResult{}
	for _, layer := range layers {
		for _, r := range layer {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			merged = append(merged, r)
		}
	}
	return merged
}

// fuzzyRank keeps pool entries where every term fuzzy-matches the memory's
// keywords+summary (typo tolerance), ordered by total match score.
func fuzzyRank(pool []domain.SearchResult, terms []string) []domain.SearchResult {
	if len(terms) == 0 || len(pool) == 0 {
		return nil
	}

	targets := make([]string, len(pool))
	for i, r := range pool {
		targets[i] = strings.Join(r.Keywords, " ") + " " + r.Summary
	}

	// every term must match somewhere in the target; scores sum up
	scores := map[int]int{} // pool index → summed score
	for _, m := range fuzzy.Find(terms[0], targets) {
		scores[m.Index] = m.Score
	}
	for _, term := range terms[1:] {
		matched := map[int]int{}
		for _, m := range fuzzy.Find(term, targets) {
			matched[m.Index] = m.Score
		}
		for idx := range scores {
			if s, ok := matched[idx]; ok {
				scores[idx] += s
			} else {
				delete(scores, idx)
			}
		}
	}

	type scored struct {
		idx, score int
	}
	ranked := make([]scored, 0, len(scores))
	for idx, score := range scores {
		ranked = append(ranked, scored{idx, score})
	}
	// score desc, pool order (already rank-sorted) breaking ties
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].idx < ranked[j].idx
	})

	out := make([]domain.SearchResult, len(ranked))
	for i, s := range ranked {
		out[i] = pool[s.idx]
	}
	return out
}
