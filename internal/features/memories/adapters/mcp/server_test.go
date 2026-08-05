package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	mcpadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/mcp"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

var _ memories.Service = (*fakeService)(nil)

// fakeService records what the tools ask of the facade.
type fakeService struct {
	stores  []managememories.StoreInput
	rated   []string
	deleted []string
	results []domain.SearchResult
	memory  *domain.MemoryReadModel
}

func (f *fakeService) Store(_ context.Context, in managememories.StoreInput) (domain.MemoryID, error) {
	f.stores = append(f.stores, in)
	return "new-id", nil
}

func (f *fakeService) Search(context.Context, ports.SearchFilters) (domain.SearchPage, error) {
	return domain.SearchPage{Results: f.results, Total: len(f.results)}, nil
}

func (f *fakeService) Get(context.Context, domain.MemoryID) (*domain.MemoryReadModel, error) {
	return f.memory, nil
}

func (f *fakeService) Rate(_ context.Context, id domain.MemoryID, _ bool) error {
	f.rated = append(f.rated, string(id))
	return nil
}

func (f *fakeService) Recall(context.Context, []string, string, int) (string, error) {
	return "# Recalled memories\n", nil
}

func (f *fakeService) Delete(_ context.Context, id domain.MemoryID) error {
	f.deleted = append(f.deleted, string(id))
	return nil
}

func connect(t *testing.T, fake *fakeService) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := sdk.NewInMemoryTransports()

	server := mcpadapter.NewServer(fake, "test")
	serverSession, err := server.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callText(t *testing.T, session *sdk.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	// the whole point: one text block, no structured mirror of the same data
	if res.StructuredContent != nil {
		t.Fatalf("%s answered with structured content — the payload now ships twice", name)
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s: %d content blocks, want 1", name, len(res.Content))
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("%s: content is %T, want text", name, res.Content[0])
	}
	return text.Text
}

// The MCP face is a token budget as much as an API: an output schema makes
// the SDK send every answer twice (once as content, once structured), which
// is pure cost when the only consumer is a model reading the text.
func TestTools_AnswerInTextWithNoOutputSchema(t *testing.T) {
	session := connect(t, &fakeService{})

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 6 {
		t.Fatalf("tools: %d", len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		if tool.OutputSchema != nil {
			t.Errorf("%s declares an output schema — responses would ship twice", tool.Name)
		}
	}
}

func TestStore_RequiresTTLAndKeywords(t *testing.T) {
	session := connect(t, &fakeService{})

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var required []string
	for _, tool := range tools.Tools {
		if tool.Name != "store_memory" {
			continue
		}
		// the wire carries the schema as raw JSON — read it the way a client does
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		required = schema.Required
	}
	for _, field := range []string{"content", "summary", "keywords", "ttlHours"} {
		if !contains(required, field) {
			t.Errorf("store_memory must require %q, got %v", field, required)
		}
	}
	if contains(required, "kind") {
		t.Error("kind is a keyword convention now, not an argument")
	}
}

// Search results carry what decides "is this worth fetching" and nothing
// else: scores, votes and counters are a get away and were never acted on.
func TestSearch_RendersLeanLines(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{{
		ID: "m1", Summary: "the summary", Keywords: []string{"go", "kind:fact"},
		Source: "test", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
		Score: 4.128999932494888, VotesUp: 3, AccessCount: 7,
	}}}
	out := callText(t, connect(t, fake), "search_memory", map[string]any{"keywords": []any{"go"}})

	if !strings.Contains(out, "m1") || !strings.Contains(out, "the summary") ||
		!strings.Contains(out, "kind:fact") {
		t.Fatalf("id, summary and keywords must survive: %q", out)
	}
	for _, noise := range []string{"4.128", "votesUp", "accessCount", "score"} {
		if strings.Contains(out, noise) {
			t.Errorf("ranking noise leaked into the result line (%s): %q", noise, out)
		}
	}
}

func TestGetAndRate_TakeBatches(t *testing.T) {
	fake := &fakeService{memory: &domain.MemoryReadModel{
		ID: "m1", Summary: "s", Content: "body", Keywords: []string{"k"},
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}}
	session := connect(t, fake)

	out := callText(t, session, "get_memory", map[string]any{"ids": []any{"m1", "m2"}})
	if strings.Count(out, "body") != 2 {
		t.Fatalf("both memories must come back in one call: %q", out)
	}

	if got := callText(t, session, "rate_memory",
		map[string]any{"ids": []any{"m1", "m2"}, "verdict": "up"}); got != "rated 2 up" {
		t.Fatalf("rate: %q", got)
	}
	if len(fake.rated) != 2 {
		t.Fatalf("rated: %v", fake.rated)
	}
}

func TestDelete_ReachesTheFacade(t *testing.T) {
	fake := &fakeService{}
	if got := callText(t, connect(t, fake), "delete_memory", map[string]any{"id": "m1"}); got != "deleted m1" {
		t.Fatalf("delete: %q", got)
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != "m1" {
		t.Fatalf("deleted: %v", fake.deleted)
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
