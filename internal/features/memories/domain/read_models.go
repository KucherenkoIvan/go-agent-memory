package domain

import "time"

// SearchResult is what search returns — summaries, not bodies: agents scan
// these cheaply and fetch full content only for the hits that matter.
type SearchResult struct {
	ID          string     `json:"id"`
	Summary     string     `json:"summary"`
	Kind        string     `json:"kind"`
	Keywords    []string   `json:"keywords"`
	Source      string     `json:"source"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	Snippet     string     `json:"snippet,omitempty"` // FTS match context, when a query was given
	Score       float64    `json:"score"`
	VotesUp     int        `json:"votesUp"`
	VotesDown   int        `json:"votesDown"`
	AccessCount int        `json:"accessCount"`
}

// SearchPage is one page of ranked results plus the total match count —
// Total counts every row the filters match, regardless of limit/offset,
// so callers can paginate with exact page numbers.
type SearchPage struct {
	Results []SearchResult `json:"results"`
	Total   int            `json:"total"`
}

// MemoryReadModel is the full view returned by get.
type MemoryReadModel struct {
	ID           string     `json:"id"`
	Content      string     `json:"content"`
	Summary      string     `json:"summary"`
	Kind         string     `json:"kind"`
	Keywords     []string   `json:"keywords"`
	Source       string     `json:"source"`
	CreatedAt    time.Time  `json:"createdAt"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	SupersededBy string     `json:"supersededBy,omitempty"`
	VotesUp      int        `json:"votesUp"`
	VotesDown    int        `json:"votesDown"`
	AccessCount  int        `json:"accessCount"`
}
