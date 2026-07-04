// Package domain — the Memory aggregate: a fact an agent decided to keep.
// Agents never edit memories; corrections supersede. Ratings accumulate on
// the memory so retrieval can rank by usefulness.
package domain

import (
	"slices"
	"strings"
	"time"

	"github.com/KucherenkoIvan/go-kernel/ddd"
)

type MemoryID string

// Kind classifies what a memory is — it is a search filter, not a hierarchy.
type Kind string

const (
	KindFact       Kind = "fact"
	KindPreference Kind = "preference"
	KindResearch   Kind = "research"
	KindDecision   Kind = "decision"
	KindLocation   Kind = "location"
	KindReference  Kind = "reference"
)

var Kinds = []Kind{KindFact, KindPreference, KindResearch, KindDecision, KindLocation, KindReference}

const (
	maxSummaryLength = 200
	maxKeywordLength = 64
)

type memoryState struct {
	id           MemoryID
	content      string
	summary      string
	kind         Kind
	keywords     []string
	source       string
	ttlHours     int // 0 = never expires
	createdAt    time.Time
	supersededBy MemoryID // "" = live
	votesUp      int
	votesDown    int
}

type Memory struct {
	ddd.EventRecorder[ddd.DomainEvent]
	state memoryState
}

// NewMemory validates and creates a memory. Summary is required — it is
// what search results show, and retrieval quality depends on it.
func NewMemory(id MemoryID, content, summary string, kind Kind, keywords []string, source string, ttlHours int, createdAt time.Time) (*Memory, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, &EmptyContentError{}
	}

	summary = normalizeSummary(summary)
	if summary == "" || len(summary) > maxSummaryLength {
		return nil, &InvalidSummaryError{}
	}

	if !slices.Contains(Kinds, kind) {
		return nil, &InvalidKindError{}
	}

	normalized := NormalizeKeywords(keywords)
	if len(normalized) == 0 {
		return nil, &NoKeywordsError{}
	}
	for _, kw := range normalized {
		if len(kw) > maxKeywordLength {
			return nil, &NoKeywordsError{}
		}
	}

	if source = strings.TrimSpace(source); source == "" {
		return nil, &EmptySourceError{}
	}
	if ttlHours < 0 {
		return nil, &InvalidTTLError{}
	}

	m := &Memory{state: memoryState{
		id: id, content: content, summary: summary, kind: kind,
		keywords: normalized, source: source, ttlHours: ttlHours, createdAt: createdAt,
	}}
	m.PushEvent(NewMemoryStoredEvent(MemoryStoredData{MemoryID: id, Kind: kind, Keywords: normalized}))
	return m, nil
}

func (m *Memory) ID() MemoryID { return m.state.id }

// ExpiresAt returns the expiry moment, or zero time when the memory never
// expires.
func (m *Memory) ExpiresAt() time.Time {
	if m.state.ttlHours == 0 {
		return time.Time{}
	}
	return m.state.createdAt.Add(time.Duration(m.state.ttlHours) * time.Hour)
}

// Rate records explicit usefulness feedback.
func (m *Memory) Rate(up bool) error {
	// invariant: rating a superseded memory is meaningless
	if m.state.supersededBy != "" {
		return &MemorySupersededError{}
	}
	if up {
		m.state.votesUp++
	} else {
		m.state.votesDown++
	}
	m.PushEvent(NewMemoryRatedEvent(MemoryRatedData{MemoryID: m.state.id, Up: up}))
	return nil
}

// MarkSuperseded points this memory at its correction; it leaves default
// search from now on.
func (m *Memory) MarkSuperseded(by MemoryID) error {
	// invariants: superseded at most once, never by itself
	if m.state.supersededBy != "" {
		return &MemorySupersededError{}
	}
	if by == m.state.id || by == "" {
		return &InvalidSupersedeError{}
	}
	m.state.supersededBy = by
	m.PushEvent(NewMemorySupersededEvent(MemorySupersededData{MemoryID: m.state.id, By: by}))
	return nil
}

// NormalizeKeywords lowercases, trims, drops empties, and dedupes while
// preserving order. Exposed so readers filter with the same normalization.
func NormalizeKeywords(keywords []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		kw = strings.ToLower(strings.Join(strings.Fields(kw), "-")) // spaces inside a keyword become dashes
		if kw == "" || seen[kw] {
			continue
		}
		seen[kw] = true
		out = append(out, kw)
	}
	return out
}

func normalizeSummary(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(s), " ")) // collapse whitespace/newlines
}

// MemorySnapshot is the persistence view — explicit mapping, no reflection.
type MemorySnapshot struct {
	ID           MemoryID
	Content      string
	Summary      string
	Kind         Kind
	Keywords     []string
	Source       string
	TTLHours     int
	CreatedAt    time.Time
	SupersededBy MemoryID
	VotesUp      int
	VotesDown    int
}

func (m *Memory) Snapshot() MemorySnapshot {
	return MemorySnapshot{
		ID: m.state.id, Content: m.state.content, Summary: m.state.summary,
		Kind: m.state.kind, Keywords: slices.Clone(m.state.keywords), Source: m.state.source,
		TTLHours: m.state.ttlHours, CreatedAt: m.state.createdAt,
		SupersededBy: m.state.supersededBy, VotesUp: m.state.votesUp, VotesDown: m.state.votesDown,
	}
}

func RestoreMemory(s MemorySnapshot) *Memory {
	return &Memory{state: memoryState{
		id: s.ID, content: s.Content, summary: s.Summary, kind: s.Kind,
		keywords: slices.Clone(s.Keywords), source: s.Source, ttlHours: s.TTLHours,
		createdAt: s.CreatedAt, supersededBy: s.SupersededBy,
		votesUp: s.VotesUp, votesDown: s.VotesDown,
	}}
}
