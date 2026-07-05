package tui

import (
	"context"
	"testing"

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
	hits := fuzzyRank(pool, []string{"golanci"})
	if len(hits) == 0 || hits[0].ID != "m1" {
		t.Fatalf("typo must fuzzy-match golangci memory: %v", ids(hits))
	}

	// every term must match: "golanci" + "docker" share no memory
	if hits := fuzzyRank(pool, []string{"golanci", "docker"}); len(hits) != 0 {
		t.Fatalf("AND across terms: %v", ids(hits))
	}

	if hits := fuzzyRank(pool, nil); hits != nil {
		t.Fatalf("no terms → no fuzzy layer: %v", ids(hits))
	}
}

func TestRunSearch_EmptyTermsIsTimeline(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{result("x", nil, "recent")}}

	results, err := runSearch(context.Background(), fake, searchSpec{kind: "fact"})
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

	if _, err := runSearch(context.Background(), fake,
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
