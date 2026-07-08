package tui

import (
	"context"
	"testing"
	"time"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

func result(id string, keywords []string, summary string) domain.SearchResult {
	return domain.SearchResult{ID: id, Keywords: keywords, Summary: summary}
}

func ids(results []domain.SearchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.ID
	}
	return out
}

func TestMergeLayers_PriorityAndDedup(t *testing.T) {
	keyword := []domain.SearchResult{result("a", nil, ""), result("b", nil, "")}
	text := []domain.SearchResult{result("b", nil, ""), result("c", nil, "")}
	fuzzy := []domain.SearchResult{result("a", nil, ""), result("d", nil, "")}

	merged := ids(mergeLayers(keyword, text, fuzzy))
	want := []string{"a", "b", "c", "d"}
	if len(merged) != len(want) {
		t.Fatalf("merged: %v", merged)
	}
	for i := range want {
		if merged[i] != want[i] {
			t.Fatalf("keyword hits must outrank text must outrank fuzzy: %v", merged)
		}
	}
}

func TestFuzzyRank_CatchesTypos(t *testing.T) {
	pool := []domain.SearchResult{
		result("m1", []string{"go", "lint"}, "golangci-lint v2 config format basics"),
		result("m2", []string{"docker"}, "container networking notes"),
		result("m3", []string{"go"}, "goroutine leak patterns"),
	}

	// human typo: "golanci" — exact search misses, fuzzy must not
	hits := fuzzyRank(pool, []string{"golanci"}, sortCreatedAsc)
	if len(hits) == 0 || hits[0].ID != "m1" {
		t.Fatalf("typo must fuzzy-match golangci memory: %v", ids(hits))
	}

	// every term must match: "golanci" + "docker" share no memory
	if hits := fuzzyRank(pool, []string{"golanci", "docker"}, sortCreatedAsc); len(hits) != 0 {
		t.Fatalf("AND across terms: %v", ids(hits))
	}

	if hits := fuzzyRank(pool, nil, sortCreatedAsc); hits != nil {
		t.Fatalf("no terms → no fuzzy layer: %v", ids(hits))
	}
}

func TestRunSearch_EmptyTermsIsTimeline(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{result("x", nil, "recent")}}

	results, _, err := runSearch(context.Background(), fake, searchSpec{kind: "fact"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || len(fake.searches) != 1 {
		t.Fatalf("timeline must be a single fetch: %d searches", len(fake.searches))
	}
	if f := fake.searches[0]; f.Kind != "fact" || f.Query != "" || f.KeywordsAny != nil {
		t.Fatalf("timeline filters: %+v", f)
	}
}

func TestRunSearch_LayersCarryScreenState(t *testing.T) {
	fake := &fakeService{}

	if _, _, err := runSearch(context.Background(), fake,
		searchSpec{terms: []string{"go", "lint"}, kind: "research", includeDead: true}); err != nil {
		t.Fatal(err)
	}
	if len(fake.searches) != 3 {
		t.Fatalf("layered search must fetch 3 times, got %d", len(fake.searches))
	}
	for i, f := range fake.searches {
		if f.Kind != "research" || !f.IncludeDead {
			t.Fatalf("layer %d must keep kind/dead filters: %+v", i, f)
		}
	}
	if kw := fake.searches[0].KeywordsAny; len(kw) != 2 {
		t.Fatalf("keyword layer: %+v", fake.searches[0])
	}
	if fake.searches[1].Query != "go lint" {
		t.Fatalf("text layer: %+v", fake.searches[1])
	}
}

func TestSearchLayers_RelevanceBeatsSortMode(t *testing.T) {
	old := time.Now().Add(-72 * time.Hour)
	strong := result("strong", []string{"jwt"}, "jwt token rotation")
	strong.Score, strong.CreatedAt = 9.0, time.Now()
	weak := result("weak", []string{"jwt"}, "jwt mentioned once")
	weak.Score, weak.CreatedAt = 3.0, old
	tiedA := result("tied-new", []string{"jwt"}, "same score, newer")
	tiedA.Score, tiedA.CreatedAt = 1.0, time.Now()
	tiedB := result("tied-old", []string{"jwt"}, "same score, older")
	tiedB.Score, tiedB.CreatedAt = 1.0, old

	// default sort is created↑ — without relevance priority, "weak" (older)
	// would jump above "strong"
	fake := &fakeService{results: []domain.SearchResult{strong, weak, tiedA, tiedB}}
	results, _, err := runSearch(context.Background(), fake,
		searchSpec{terms: []string{"jwt"}, sort: sortCreatedAsc})
	if err != nil {
		t.Fatal(err)
	}
	got := ids(results)
	if got[0] != "strong" || got[1] != "weak" {
		t.Fatalf("relevance must outrank the sort mode: %v", got)
	}
	if got[2] != "tied-old" || got[3] != "tied-new" {
		t.Fatalf("equal relevance must fall back to the sort mode: %v", got)
	}
}

func TestFuzzyRank_CutoffDropsScatteredMatches(t *testing.T) {
	pool := []domain.SearchResult{
		result("scattered", []string{"k"}, "r_e_c_a_l_l spread out"),
		result("contiguous", []string{"k"}, "recall exact word"),
	}
	hits := fuzzyRank(pool, []string{"recall"}, sortCreatedAsc)
	if len(hits) != 1 || hits[0].ID != "contiguous" {
		t.Fatalf("sub-half coherence must be filtered out: %v", ids(hits))
	}

	// user typo: "recal" still carries "rec" — cutoff must not kill typos
	hits = fuzzyRank(pool, []string{"recal"}, sortCreatedAsc)
	if len(hits) != 1 || hits[0].ID != "contiguous" {
		t.Fatalf("typo-grade fuzz must survive the cutoff: %v", ids(hits))
	}
}
