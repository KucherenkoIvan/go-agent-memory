package tui

import (
	"strings"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// ParseQuery turns the search box into filters: `k:<keyword>` tokens become
// AND keyword filters, `kind:<kind>` selects the kind, everything else is
// the full-text query.
func ParseQuery(raw string) ports.SearchFilters {
	var (
		filters ports.SearchFilters
		terms   []string
	)
	for _, token := range strings.Fields(raw) {
		switch {
		case strings.HasPrefix(token, "k:") && len(token) > 2:
			filters.Keywords = append(filters.Keywords, token[2:])
		case strings.HasPrefix(token, "kind:") && len(token) > 5:
			filters.Kind = token[5:]
		default:
			terms = append(terms, token)
		}
	}
	if len(filters.Keywords) > 0 {
		filters.Keywords = domain.NormalizeKeywords(filters.Keywords)
	}
	filters.Query = strings.Join(terms, " ")
	return filters
}
