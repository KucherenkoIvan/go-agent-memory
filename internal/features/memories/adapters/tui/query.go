package tui

import (
	"context"
	"sort"
	"strings"
	"time"

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
	recent      bool // only the last 3 days
	sort        sortMode
}

const recentCutoff = 3 * 24 * time.Hour

func (s searchSpec) base() ports.SearchFilters {
	f := ports.SearchFilters{Kind: s.kind, IncludeDead: s.includeDead}
	if s.recent {
		f.Since = time.Now().Add(-recentCutoff)
	}
	return f
}

// runSearch executes the layered query. Three Service calls when terms are
// present; a single timeline fetch otherwise. The spec's sort mode orders
// the timeline outright; under search it only sorts *within* each layer —
// relevance (keywords > text > fuzzy) stays the primary order.
func runSearch(ctx context.Context, svc memories.Service, spec searchSpec) ([]domain.SearchResult, error) {
	if len(spec.terms) == 0 {
		timeline := spec.base()
		timeline.Limit = layerFetchLimit
		if spec.review {
			timeline.Limit = widePoolLimit
		}
		results, err := svc.Search(ctx, timeline)
		if err != nil {
			return nil, err
		}
		sortResults(results, spec.sort)
		return results, nil
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

	// relevance first inside every layer — the reader's score for keyword
	// and text hits, the fuzzy match score for the pool — with the sort
	// mode breaking ties only
	byScore := func(r domain.SearchResult) float64 { return r.Score }
	sortLayer(keywordHits, byScore, spec.sort)
	sortLayer(textHits, byScore, spec.sort)
	return mergeLayers(keywordHits, textHits, fuzzyRank(pool, spec.terms, spec.sort)), nil
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
// keywords+summary (typo tolerance), ordered by total match score — closer
// matches on top, the sort mode breaking equal scores.
func fuzzyRank(pool []domain.SearchResult, terms []string, mode sortMode) []domain.SearchResult {
	if len(terms) == 0 || len(pool) == 0 {
		return nil
	}

	targets := make([]string, len(pool))
	for i, r := range pool {
		targets[i] = strings.Join(r.Keywords, " ") + " " + r.Summary
	}

	// every term must match somewhere in the target — coherently (see
	// coherentMatch); scores sum up
	scores := map[int]int{} // pool index → summed score
	for _, m := range fuzzy.Find(terms[0], targets) {
		if coherentMatch(terms[0], targets[m.Index]) {
			scores[m.Index] = m.Score
		}
	}
	for _, term := range terms[1:] {
		matched := map[int]int{}
		for _, m := range fuzzy.Find(term, targets) {
			if coherentMatch(term, targets[m.Index]) {
				matched[m.Index] = m.Score
			}
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
	// score desc; equal scores fall back to the sort mode, then pool order
	tie := cmpMode(mode)
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if c := tie(pool[ranked[i].idx], pool[ranked[j].idx]); c != 0 {
			return c < 0
		}
		return ranked[i].idx < ranked[j].idx
	})

	out := make([]domain.SearchResult, len(ranked))
	for i, s := range ranked {
		out[i] = pool[s.idx]
	}
	return out
}
