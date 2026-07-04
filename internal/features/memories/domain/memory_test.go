package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/KucherenkoIvan/go-kernel/ddd"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

var now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func valid(t *testing.T) *domain.Memory {
	t.Helper()
	m, err := domain.NewMemory("m-1", "content", "summary", domain.KindFact, []string{"go"}, "test", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNewMemory_Invariants(t *testing.T) {
	cases := []struct {
		name                     string
		content, summary, source string
		kind                     domain.Kind
		keywords                 []string
		ttl                      int
	}{
		{"empty content", " ", "s", "src", domain.KindFact, []string{"k"}, 0},
		{"empty summary", "c", "  ", "src", domain.KindFact, []string{"k"}, 0},
		{"summary too long", "c", strings.Repeat("x", 300), "src", domain.KindFact, []string{"k"}, 0},
		{"bad kind", "c", "s", "src", "vibe", []string{"k"}, 0},
		{"no keywords", "c", "s", "src", domain.KindFact, []string{"  ", ""}, 0},
		{"empty source", "c", "s", " ", domain.KindFact, []string{"k"}, 0},
		{"negative ttl", "c", "s", "src", domain.KindFact, []string{"k"}, -1},
	}
	for _, tc := range cases {
		_, err := domain.NewMemory("m-1", tc.content, tc.summary, tc.kind, tc.keywords, tc.source, tc.ttl, now)
		if err == nil || !ddd.IsDomainError(err) {
			t.Errorf("%s: expected domain error, got %v", tc.name, err)
		}
	}
}

func TestNewMemory_NormalizationAndEvent(t *testing.T) {
	m, err := domain.NewMemory("m-1", "content", "  a\nsummary\twith   space  ", domain.KindResearch,
		[]string{"Go Lint", "go-lint", "  ", "API"}, "test", 12, now)
	if err != nil {
		t.Fatal(err)
	}
	snap := m.Snapshot()
	if snap.Summary != "a summary with space" {
		t.Errorf("summary: %q", snap.Summary)
	}
	if len(snap.Keywords) != 2 || snap.Keywords[0] != "go-lint" || snap.Keywords[1] != "api" {
		t.Errorf("keywords: %v", snap.Keywords) // "Go Lint" → "go-lint", deduped with "go-lint"
	}
	if got := m.ExpiresAt(); !got.Equal(now.Add(12 * time.Hour)) {
		t.Errorf("expiry: %v", got)
	}
	if events := m.PopEvents(); len(events) != 1 || events[0].EventName() != domain.MemoryStoredEventName {
		t.Errorf("events: %v", events)
	}
}

func TestRateAndSupersede(t *testing.T) {
	m := valid(t)
	m.PopEvents()

	if err := m.Rate(true); err != nil {
		t.Fatal(err)
	}
	if err := m.Rate(false); err != nil {
		t.Fatal(err)
	}
	snap := m.Snapshot()
	if snap.VotesUp != 1 || snap.VotesDown != 1 {
		t.Fatalf("votes: %+v", snap)
	}

	if err := m.MarkSuperseded("m-1"); err == nil {
		t.Fatal("self-supersede must fail")
	}
	if err := m.MarkSuperseded("m-2"); err != nil {
		t.Fatal(err)
	}
	if err := m.MarkSuperseded("m-3"); err == nil {
		t.Fatal("double supersede must fail")
	}
	if err := m.Rate(true); err == nil {
		t.Fatal("rating a superseded memory must fail")
	}
	if events := m.PopEvents(); len(events) != 3 { // rated, rated, superseded
		t.Fatalf("events: %v", events)
	}
}
