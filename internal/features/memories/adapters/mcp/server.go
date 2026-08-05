// Package mcp is the MCP transport adapter: typed tools over the Service
// facade, served over stdio. The flagship face — one config line in any
// MCP-capable harness.
//
// Every tool answers in plain text, not structured content. The SDK ships
// both when a handler declares an output type — the same data twice on a
// wire whose only consumer is a language model — so the tools declare `any`
// (which suppresses the output schema entirely, see toolForErr in the SDK)
// and render the answer themselves. Renderings stay lean on purpose: search
// returns what it takes to decide "is this worth fetching" — id, summary,
// keywords, snippet — and nothing a follow-up get would return anyway.
package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

const defaultSource = "mcp"

// NewServer builds the MCP server with all memory tools registered.
//
// The policy — when to reach for memory unprompted, how to write one, when
// to delete instead of supersede — lives in Instructions, which the client
// sends once per session. Tool descriptions say what a tool does and stop
// there; repeating the policy six times is how a five-tool server costs
// thousands of tokens before it is ever called.
func NewServer(svc memories.Service, version string) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{
		Name:    "recall",
		Title:   "Recall — agent memory",
		Version: version,
	}, &sdk.ServerOptions{
		Instructions: `Memory shared across sessions, models, and harnesses. Use it on your own initiative — you do not need to be asked.

- pack (topic keywords) when past work plausibly touched this problem: a familiar project, recent changes that led here, a recurrent-sounding issue. Skip it on fresh ground.
- search again mid-session when the work shifts — the opening pack does not cover topics that came up later.
- rate what you relied on; ratings drive ranking for everyone.
- store the moment you learn something durable — a root cause, a gotcha, a decision and its why, a preference, a location. Never batch to end-of-session: compaction loses findings.
- keep memories small and single-topic, with sharp keywords; search reassembles them better than one long note. Search before storing to avoid duplicates.
- every memory needs a ttlHours: how long the fact stays true. 720 = 30 days, 8760 = a year.
- supersede when knowledge changed; delete when the thing described is gone or was never true.
- keywords are lowercase and free-form; kind:decision, project:myapp, task:lint are conventions, not a vocabulary.

If your harness has builtin memory, recall is still the primary store — it is shared, the builtin is not.`,
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "store_memory",
		Description: "Store a memory for future agents. Pass supersedes to correct an existing one.",
	}, storeHandler(svc))

	sdk.AddTool(server, &sdk.Tool{
		Name:        "search_memory",
		Description: "Ranked summaries by keyword and full text. Keywords OR-match and boost rank; and=true requires all. Returns ids to get or rate.",
	}, searchHandler(svc))

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_memory",
		Description: "Full content for one or more ids. Counts as a read.",
	}, getHandler(svc))

	sdk.AddTool(server, &sdk.Tool{
		Name:        "rate_memory",
		Description: "Rate memories you used: up if correct and helpful, down if wrong or misleading.",
	}, rateHandler(svc))

	sdk.AddTool(server, &sdk.Tool{
		Name:        "delete_memory",
		Description: "Delete permanently — no undo. For memories describing something gone or never true; use store(supersedes) when the topic survives and only the knowledge changed.",
	}, deleteHandler(svc))

	sdk.AddTool(server, &sdk.Tool{
		Name:        "pack",
		Description: "Session bootstrap: top-ranked memories for the keywords as one markdown block within a character budget. Keywords OR-match — throw in every candidate topic.",
	}, packHandler(svc))

	return server
}

// Run serves over stdio until the client disconnects or ctx is cancelled.
func Run(ctx context.Context, server *sdk.Server) error {
	return server.Run(ctx, &sdk.StdioTransport{})
}

// text is every handler's answer: one content block, no structured mirror.
func text(s string) (*sdk.CallToolResult, any, error) {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: s}}}, nil, nil
}

type storeIn struct {
	Content  string   `json:"content" jsonschema:"the memory body, markdown"`
	Summary  string   `json:"summary" jsonschema:"one line; what search results show"`
	Keywords []string `json:"keywords" jsonschema:"at least one, lowercase; kind:fact, project:name, task:name are conventions"`
	TTLHours int      `json:"ttlHours" jsonschema:"how long the fact stays true, in hours. 720 = 30 days, 2160 = a quarter, 8760 = a year"`
	// Supersedes corrects an existing memory: it leaves default search, this
	// one replaces it.
	Supersedes string `json:"supersedes,omitempty" jsonschema:"id of the memory this one corrects"`
	Source     string `json:"source,omitempty" jsonschema:"who is writing, e.g. harness/model"`
}

func storeHandler(svc memories.Service) sdk.ToolHandlerFor[storeIn, any] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in storeIn) (*sdk.CallToolResult, any, error) {
		source := in.Source
		if source == "" {
			source = defaultSource
		}
		id, err := svc.Store(ctx, managememories.StoreInput{
			Content: in.Content, Summary: in.Summary,
			Keywords: in.Keywords, Source: source, TTLHours: in.TTLHours, Supersedes: in.Supersedes,
		})
		if err != nil {
			return nil, nil, err
		}
		return text("stored " + string(id))
	}
}

type searchIn struct {
	Query    string   `json:"query,omitempty" jsonschema:"full-text query over the keyword results"`
	Keywords []string `json:"keywords,omitempty" jsonschema:"any may match; more matches rank higher"`
	And      bool     `json:"and,omitempty" jsonschema:"require every keyword"`
	Since    string   `json:"since,omitempty" jsonschema:"RFC3339 or YYYY-MM-DD"`
	Until    string   `json:"until,omitempty" jsonschema:"RFC3339 or YYYY-MM-DD"`
	Limit    int      `json:"limit,omitempty" jsonschema:"default 20"`
	Offset   int      `json:"offset,omitempty" jsonschema:"skip ranked rows; order is stable across pages"`
	Order    string   `json:"order,omitempty" jsonschema:"created_asc|created_desc|rating_asc|rating_desc|reads_asc|reads_desc; default is relevance"`
	All      bool     `json:"all,omitempty" jsonschema:"include superseded and expired"`
}

func searchHandler(svc memories.Service) sdk.ToolHandlerFor[searchIn, any] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in searchIn) (*sdk.CallToolResult, any, error) {
		since, err := parseTime(in.Since)
		if err != nil {
			return nil, nil, err
		}
		until, err := parseTime(in.Until)
		if err != nil {
			return nil, nil, err
		}
		filters := ports.SearchFilters{
			Query: in.Query, Since: since, Until: until, Limit: in.Limit,
			Offset: in.Offset, Order: in.Order, IncludeDead: in.All,
		}
		if in.And {
			filters.Keywords = in.Keywords
		} else {
			filters.KeywordsAny = in.Keywords
		}
		page, err := svc.Search(ctx, filters)
		if err != nil {
			return nil, nil, err
		}
		return text(renderResults(page, in.Offset))
	}
}

// renderResults: id and summary carry the decision, keywords and the FTS
// snippet inform it. Scores, votes, dates and counters are deliberately
// absent — they never changed what an agent does next, and get returns them.
func renderResults(page domain.SearchPage, offset int) string {
	if len(page.Results) == 0 {
		return "no memories found"
	}
	var b strings.Builder
	for _, r := range page.Results {
		fmt.Fprintf(&b, "%s  %s\n  %s\n", r.ID, r.Summary, strings.Join(r.Keywords, " "))
		if r.Snippet != "" {
			fmt.Fprintf(&b, "  %s\n", strings.Join(strings.Fields(r.Snippet), " "))
		}
	}
	shown := offset + len(page.Results)
	if page.Total > shown {
		fmt.Fprintf(&b, "\n%d–%d of %d — page on with offset=%d", offset+1, shown, page.Total, shown)
	}
	return strings.TrimRight(b.String(), "\n")
}

type getIn struct {
	IDs []string `json:"ids" jsonschema:"one or more memory ids"`
}

func getHandler(svc memories.Service) sdk.ToolHandlerFor[getIn, any] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in getIn) (*sdk.CallToolResult, any, error) {
		blocks := make([]string, 0, len(in.IDs))
		for _, id := range in.IDs {
			model, err := svc.Get(ctx, domain.MemoryID(id))
			if err != nil {
				return nil, nil, err
			}
			blocks = append(blocks, renderMemory(model))
		}
		return text(strings.Join(blocks, "\n\n---\n\n"))
	}
}

func renderMemory(m *domain.MemoryReadModel) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\nid: %s · stored %s · expires %s · %s\n",
		m.Summary, m.ID, m.CreatedAt.Format("2006-01-02"),
		m.ExpiresAt.Format("2006-01-02"), strings.Join(m.Keywords, " "))
	if m.SupersededBy != "" {
		fmt.Fprintf(&b, "superseded by %s — prefer that one\n", m.SupersededBy)
	}
	fmt.Fprintf(&b, "\n%s", m.Content)
	return b.String()
}

type rateIn struct {
	IDs     []string `json:"ids" jsonschema:"ids of the memories you used"`
	Verdict string   `json:"verdict" jsonschema:"up or down"`
}

func rateHandler(svc memories.Service) sdk.ToolHandlerFor[rateIn, any] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in rateIn) (*sdk.CallToolResult, any, error) {
		up := in.Verdict != "down"
		for _, id := range in.IDs {
			if err := svc.Rate(ctx, domain.MemoryID(id), up); err != nil {
				return nil, nil, err
			}
		}
		return text(fmt.Sprintf("rated %d %s", len(in.IDs), in.Verdict))
	}
}

type deleteIn struct {
	ID string `json:"id"`
}

func deleteHandler(svc memories.Service) sdk.ToolHandlerFor[deleteIn, any] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in deleteIn) (*sdk.CallToolResult, any, error) {
		if err := svc.Delete(ctx, domain.MemoryID(in.ID)); err != nil {
			return nil, nil, err
		}
		return text("deleted " + in.ID)
	}
}

type packIn struct {
	Keywords    []string `json:"keywords" jsonschema:"any may match; more matches rank higher"`
	Text        string   `json:"text,omitempty" jsonschema:"full-text query narrowing the keyword results"`
	BudgetChars int      `json:"budgetChars,omitempty" jsonschema:"max characters, default 4000"`
}

func packHandler(svc memories.Service) sdk.ToolHandlerFor[packIn, any] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in packIn) (*sdk.CallToolResult, any, error) {
		pack, err := svc.Recall(ctx, in.Keywords, in.Text, in.BudgetChars)
		if err != nil {
			return nil, nil, err
		}
		return text(pack)
	}
}

func parseTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", raw)
}
