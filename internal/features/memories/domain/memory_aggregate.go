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

// KindTags are the conventional `kind:*` keywords. A convention, not a
// vocabulary: nothing validates them, nothing requires them, and a memory
// may carry another one entirely. They exist so human faces have something
// to offer as a browsing filter — kind was a required column until it
// proved to be a keyword wearing a schema.
var KindTags = []string{
	"kind:fact", "kind:preference", "kind:research",
	"kind:decision", "kind:location", "kind:reference",
}

// KindOf returns the value of the first `kind:` keyword, or "" when the
// memory carries none. Display affordance only — never a filter path.
func KindOf(keywords []string) string {
	for _, kw := range keywords {
		if after, ok := strings.CutPrefix(kw, "kind:"); ok {
			return after
		}
	}
	return ""
}

const (
	maxSummaryLength = 200
	maxKeywordLength = 64
	// maxTTLHours — ten years. Nothing lives forever, but the far end of
	// the range is "effectively permanent"; the bound also keeps the value
	// inside the int32 the wire contract carries.
	maxTTLHours = 10 * 365 * 24
)

type memoryState struct {
	id           MemoryID
	content      string
	summary      string
	keywords     []string
	source       string
	ttlHours     int // > 0 — every memory expires
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
func NewMemory(id MemoryID, content, summary string, keywords []string, source string, ttlHours int, createdAt time.Time) (*Memory, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, &EmptyContentError{}
	}

	summary = normalizeSummary(summary)
	if summary == "" || len(summary) > maxSummaryLength {
		return nil, &InvalidSummaryError{}
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
	// invariant: every memory expires — the writer picks the lifetime, there
	// is no "never". Memory that outlives its usefulness is worse than no
	// memory: it ranks, it gets read, and it misleads.
	if ttlHours <= 0 || ttlHours > maxTTLHours {
		return nil, &InvalidTTLError{}
	}

	m := &Memory{state: memoryState{
		id: id, content: content, summary: summary,
		keywords: normalized, source: source, ttlHours: ttlHours, createdAt: createdAt,
	}}
	m.PushEvent(NewMemoryStoredEvent(MemoryStoredData{MemoryID: id, Keywords: normalized}))
	return m, nil
}

func (m *Memory) ID() MemoryID { return m.state.id }

// ExpiresAt returns the expiry moment — always set; a memory without a
// lifetime cannot exist.
func (m *Memory) ExpiresAt() time.Time {
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
		Keywords: slices.Clone(m.state.keywords), Source: m.state.source,
		TTLHours: m.state.ttlHours, CreatedAt: m.state.createdAt,
		SupersededBy: m.state.supersededBy, VotesUp: m.state.votesUp, VotesDown: m.state.votesDown,
	}
}

func RestoreMemory(s MemorySnapshot) *Memory {
	return &Memory{state: memoryState{
		id: s.ID, content: s.Content, summary: s.Summary,
		keywords: slices.Clone(s.Keywords), source: s.Source, ttlHours: s.TTLHours,
		createdAt: s.CreatedAt, supersededBy: s.SupersededBy,
		votesUp: s.VotesUp, votesDown: s.VotesDown,
	}}
}
