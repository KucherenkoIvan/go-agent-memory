package memories_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KucherenkoIvan/go-kernel/ddd"
	"github.com/KucherenkoIvan/go-kernel/ddd/dddtest"
	"github.com/KucherenkoIvan/go-kernel/events"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	sqliteadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/sqlite"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

func setup(t *testing.T) (memories.Service, *storage.Store) {
	t.Helper()
	store, err := storage.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	pub := events.NewChannelPublisher()
	t.Cleanup(func() { _ = pub.Close(context.Background()) })

	return memories.New(store.DB, pub), store
}

func mustStore(t *testing.T, svc memories.Service, in managememories.StoreInput) domain.MemoryID {
	t.Helper()
	if in.Source == "" {
		in.Source = "test"
	}
	id, err := svc.Store(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestStoreSearchGet_EndToEnd(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()

	id := mustStore(t, svc, managememories.StoreInput{
		Content: "golangci-lint v2 config uses version: \"2\" and a linters.default set",
		Summary: "golangci-lint v2 config format basics",
		Kind:    "research", Keywords: []string{"go", "lint", "project:kernel"},
	})
	mustStore(t, svc, managememories.StoreInput{
		Content: "the user prefers tabs",
		Summary: "indentation preference",
		Kind:    "preference", Keywords: []string{"style"},
	})

	// FTS query finds the right one, with snippet and keywords intact
	results, err := searchResults(svc, ctx, ports.SearchFilters{Query: "golangci config"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != string(id) {
		t.Fatalf("query results: %+v", results)
	}
	if results[0].Summary == "" || len(results[0].Keywords) != 3 {
		t.Fatalf("result shape: %+v", results[0])
	}

	// keyword filter composes (normalized)
	results, _ = searchResults(svc, ctx, ports.SearchFilters{Keywords: []string{"GO", "project:kernel"}})
	if len(results) != 1 || results[0].ID != string(id) {
		t.Fatalf("keyword results: %+v", results)
	}

	// kind filter
	results, _ = searchResults(svc, ctx, ports.SearchFilters{Kind: "preference"})
	if len(results) != 1 || results[0].Kind != "preference" {
		t.Fatalf("kind results: %+v", results)
	}

	// get returns full content and bumps the access count
	model, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(model.Content, "linters.default") || model.AccessCount != 1 {
		t.Fatalf("get: %+v", model)
	}
	model, _ = svc.Get(ctx, id)
	if model.AccessCount != 2 {
		t.Fatalf("access not bumped: %+v", model)
	}

	// missing id is a domain error
	var notFound *domain.MemoryNotFoundError
	if _, err := svc.Get(ctx, "nope"); !errors.As(err, &notFound) {
		t.Fatalf("missing get: %v", err)
	}
}

func TestSearch_OrderAndOffset_PaginateStably(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()

	for _, name := range []string{"first", "second", "third"} {
		mustStore(t, svc, managememories.StoreInput{
			Content: name + " body", Summary: name + " notes",
			Kind: "fact", Keywords: []string{"paging"},
		})
	}

	page := func(offset int) []domain.SearchResult {
		results, err := searchResults(svc, ctx, ports.SearchFilters{
			Order: ports.OrderCreatedAsc, Limit: 2, Offset: offset,
		})
		if err != nil {
			t.Fatal(err)
		}
		return results
	}

	one, two := page(0), page(2)
	if len(one) != 2 || len(two) != 1 {
		t.Fatalf("pages: %d + %d", len(one), len(two))
	}
	if one[0].Summary != "first notes" || one[1].Summary != "second notes" || two[0].Summary != "third notes" {
		t.Fatalf("created_asc pages out of order: %v %v", one, two)
	}

	descPage, err := svc.Search(ctx, ports.SearchFilters{Order: ports.OrderCreatedDesc, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(descPage.Results) != 1 || descPage.Results[0].Summary != "third notes" {
		t.Fatalf("created_desc must lead with the newest: %+v", descPage.Results)
	}
	if descPage.Total != 3 {
		t.Fatalf("total must count all matches regardless of limit: %d", descPage.Total)
	}

	if _, err := svc.Search(ctx, ports.SearchFilters{Order: "bogus"}); err == nil {
		t.Fatal("invalid order must be rejected")
	}
}

func TestRating_DrivesRanking(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()

	first := mustStore(t, svc, managememories.StoreInput{
		Content: "use depguard for lint architecture rules",
		Summary: "lint: depguard for layer rules",
		Kind:    "research", Keywords: []string{"lint"},
	})
	second := mustStore(t, svc, managememories.StoreInput{
		Content: "lint runs via make lint",
		Summary: "lint: how to run",
		Kind:    "fact", Keywords: []string{"lint"},
	})

	for range 3 {
		if err := svc.Rate(ctx, second, true); err != nil {
			t.Fatal(err)
		}
	}
	_ = svc.Rate(ctx, first, false)

	results, err := searchResults(svc, ctx, ports.SearchFilters{Keywords: []string{"lint"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].ID != string(second) {
		t.Fatalf("upvoted memory must rank first: %+v", results)
	}
	if results[0].VotesUp != 3 {
		t.Fatalf("votes: %+v", results[0])
	}
}

func TestSupersede_LeavesDefaultSearch(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()

	old := mustStore(t, svc, managememories.StoreInput{
		Content: "the db lives at /old/path", Summary: "db location",
		Kind: "location", Keywords: []string{"db"},
	})
	newer := mustStore(t, svc, managememories.StoreInput{
		Content: "the db lives at /new/path", Summary: "db location (corrected)",
		Kind: "location", Keywords: []string{"db"}, Supersedes: string(old),
	})

	results, _ := searchResults(svc, ctx, ports.SearchFilters{Keywords: []string{"db"}})
	if len(results) != 1 || results[0].ID != string(newer) {
		t.Fatalf("default search: %+v", results)
	}

	all, _ := searchResults(svc, ctx, ports.SearchFilters{Keywords: []string{"db"}, IncludeDead: true})
	if len(all) != 2 {
		t.Fatalf("--all search: %+v", all)
	}

	model, _ := svc.Get(ctx, old)
	if model.SupersededBy != string(newer) {
		t.Fatalf("supersede link: %+v", model)
	}

	// superseding an already-superseded memory fails
	_, err := svc.Store(ctx, managememories.StoreInput{
		Content: "x", Summary: "x", Kind: "fact", Keywords: []string{"db"},
		Source: "test", Supersedes: string(old),
	})
	var superseded *domain.MemorySupersededError
	if !errors.As(err, &superseded) {
		t.Fatalf("double supersede: %v", err)
	}
}

func TestTTL_ExpiredLeavesDefaultSearch(t *testing.T) {
	_, store := setup(t)
	ctx := context.Background()

	// wire a store command with a clock two days in the past
	pub := events.NewChannelPublisher()
	t.Cleanup(func() { _ = pub.Close(context.Background()) })
	repo := sqliteadapter.NewMemoryRepository(store.DB)
	reader := sqliteadapter.NewMemoryReader(store.DB)
	past := dddtest.FixedClock{Time: time.Now().Add(-48 * time.Hour)}
	oldStore := managememories.NewStoreCommand(store.DB, ddd.UUIDv7Generator{}, past, repo, pub)

	if _, err := oldStore.Execute(ctx, managememories.StoreInput{
		Content: "freeze until yesterday", Summary: "temporary freeze",
		Kind: "fact", Keywords: []string{"ops"}, Source: "test", TTLHours: 1, // expired 47h ago
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := oldStore.Execute(ctx, managememories.StoreInput{
		Content: "permanent fact", Summary: "keeper",
		Kind: "fact", Keywords: []string{"ops"}, Source: "test",
	}); err != nil {
		t.Fatal(err)
	}

	search := managememories.NewSearchQuery(reader)
	page, err := search.Execute(ctx, ports.SearchFilters{Keywords: []string{"ops"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 1 || page.Results[0].Summary != "keeper" {
		t.Fatalf("expired must be excluded: %+v", page.Results)
	}

	all, _ := search.Execute(ctx, ports.SearchFilters{Keywords: []string{"ops"}, IncludeDead: true})
	if len(all.Results) != 2 {
		t.Fatalf("--all must include expired: %+v", all.Results)
	}
}

func TestDelete_RemovesForGood(t *testing.T) {
	svc, store := setup(t)
	ctx := context.Background()

	id := mustStore(t, svc, managememories.StoreInput{
		Content: "obsolete garbage", Summary: "to be pruned",
		Kind: "fact", Keywords: []string{"prune-me"},
	})

	if err := svc.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}

	var notFound *domain.MemoryNotFoundError
	if _, err := svc.Get(ctx, id); !errors.As(err, &notFound) {
		t.Fatalf("get after delete: %v", err)
	}
	if err := svc.Delete(ctx, id); !errors.As(err, &notFound) {
		t.Fatalf("double delete: %v", err)
	}

	// the FTS trigger must have dropped the index row too
	results, err := searchResults(svc, ctx, ports.SearchFilters{Query: "obsolete garbage", IncludeDead: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("FTS row survived delete: %+v", results)
	}
	var ftsRows int
	if err := store.DB.DB().QueryRowContext(ctx, `SELECT count(*) FROM memories_fts WHERE memories_fts MATCH 'obsolete'`).Scan(&ftsRows); err != nil {
		t.Fatal(err)
	}
	if ftsRows != 0 {
		t.Fatalf("fts table still has %d rows", ftsRows)
	}

	// the repository must have cleaned the keyword mirror in the same tx —
	// stale links are how a reused id would produce false search hits
	results, _ = searchResults(svc, ctx, ports.SearchFilters{KeywordsAny: []string{"prune-me"}, IncludeDead: true})
	if len(results) != 0 {
		t.Fatalf("keyword search hits deleted memory: %+v", results)
	}
	var linkRows int
	if err := store.DB.DB().QueryRowContext(ctx, `SELECT count(*) FROM memory_keywords WHERE memory_id = $1`, string(id)).Scan(&linkRows); err != nil {
		t.Fatal(err)
	}
	if linkRows != 0 {
		t.Fatalf("keyword mirror still has %d links", linkRows)
	}
}

func TestKeywordMirror_IdempotentAcrossResaves(t *testing.T) {
	svc, store := setup(t)
	ctx := context.Background()

	id := mustStore(t, svc, managememories.StoreInput{
		Content: "keyword mirror subject", Summary: "mirror notes",
		Kind: "fact", Keywords: []string{"mirror-kw", "project:mirror"},
	})

	// rate and supersede both re-save the row; the mirror must not
	// duplicate or drop links
	if err := svc.Rate(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	mustStore(t, svc, managememories.StoreInput{
		Content: "corrected subject", Summary: "mirror notes v2",
		Kind: "fact", Keywords: []string{"mirror-kw"}, Supersedes: string(id),
	})

	var linkRows int
	if err := store.DB.DB().QueryRowContext(ctx, `SELECT count(*) FROM memory_keywords WHERE memory_id = $1`, string(id)).Scan(&linkRows); err != nil {
		t.Fatal(err)
	}
	if linkRows != 2 {
		t.Fatalf("expected 2 links for the original, got %d", linkRows)
	}

	// default search sees only the superseding memory despite live links on both
	results, err := searchResults(svc, ctx, ports.SearchFilters{KeywordsAny: []string{"mirror-kw"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Summary != "mirror notes v2" {
		t.Fatalf("superseded leak through keyword search: %+v", results)
	}
}

func TestRecall_AssemblesWithinBudget(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()

	for i, name := range []string{"alpha", "beta", "gamma"} {
		id := mustStore(t, svc, managememories.StoreInput{
			Content: strings.Repeat(name+" ", 30),
			Summary: name + " notes", Kind: "research", Keywords: []string{"recall-test"},
		})
		for range 3 - i { // alpha most upvoted
			_ = svc.Rate(ctx, id, true)
		}
	}

	pack, err := svc.Recall(ctx, []string{"recall-test"}, "", 600)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pack, "alpha notes") {
		t.Fatalf("top memory missing:\n%s", pack)
	}
	if len(pack) > 700 { // budget respected (small tolerance for the header)
		t.Fatalf("budget exceeded: %d chars", len(pack))
	}

	empty, _ := svc.Recall(ctx, []string{"nothing-here"}, "", 600)
	if !strings.Contains(empty, "Nothing stored") {
		t.Fatalf("empty recall: %q", empty)
	}
}

func TestRecall_KeywordsORMatch_MoreMatchesRankHigher(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()

	mustStore(t, svc, managememories.StoreInput{
		Content: "only the go keyword", Summary: "go-only notes",
		Kind: "fact", Keywords: []string{"go"},
	})
	mustStore(t, svc, managememories.StoreInput{
		Content: "only the project keyword", Summary: "project-only notes",
		Kind: "fact", Keywords: []string{"project:app"},
	})
	mustStore(t, svc, managememories.StoreInput{
		Content: "both keywords", Summary: "both notes",
		Kind: "fact", Keywords: []string{"go", "project:app"},
	})

	// any keyword qualifies — unknown ones don't empty the result
	pack, err := svc.Recall(ctx, []string{"go", "project:app", "no-such-topic"}, "", 4000)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"go-only notes", "project-only notes", "both notes"} {
		if !strings.Contains(pack, want) {
			t.Fatalf("OR recall must include %q:\n%s", want, pack)
		}
	}

	// the memory matching more keywords comes first
	if strings.Index(pack, "both notes") > strings.Index(pack, "go-only notes") ||
		strings.Index(pack, "both notes") > strings.Index(pack, "project-only notes") {
		t.Fatalf("double match must rank first:\n%s", pack)
	}

	// search keeps AND semantics: both keywords → only the double-tagged one
	results, err := searchResults(svc, ctx, ports.SearchFilters{Keywords: []string{"go", "project:app"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Summary != "both notes" {
		t.Fatalf("search must stay AND: %+v", results)
	}
}

// searchResults unwraps the page — most assertions only care about rows.
func searchResults(svc memories.Service, ctx context.Context, f ports.SearchFilters) ([]domain.SearchResult, error) {
	page, err := svc.Search(ctx, f)
	return page.Results, err
}
