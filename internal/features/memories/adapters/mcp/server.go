// Package mcp is the MCP transport adapter: typed tools over the Service
// facade, served over stdio. The flagship face — one config line in any
// MCP-capable harness.
package mcp

import (
	"context"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

const defaultSource = "mcp"

// NewServer builds the MCP server with all memory tools registered.
func NewServer(svc memories.Service, version string) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{
		Name:    "recall",
		Title:   "Recall — agent memory",
		Version: version,
	}, &sdk.ServerOptions{
		Instructions: `Persistent memory shared across sessions, models, and harnesses. You may be asked to use it explicitly, but you do not need to be — using it on your own initiative is expected:
- call pack with topic keywords when there is reason to believe past work touched this or a related problem (the project is familiar, recent changes led to the current task, the issue sounds recurrent) — not reflexively on every task, that would pull irrelevant context in;
- search again mid-session when the work shifts to a new problem — the opening pack does not cover topics that emerged later;
- when you rely on a memory, rate it afterwards — ratings drive ranking for everyone;
- store at the moment of a finding — a confirmed root cause, an environment or tooling gotcha, a decision and its why, a stated preference, a location. Do not batch stores to end-of-session: wrap-up and context compaction both lose findings.
If your harness has builtin memory, recall is the primary store: durable findings go here even when they would also fit the builtin one — recall is shared, the builtin is not. Reserve builtin memory for harness-specific operating notes.
Prefer small, focused memories: each should cover one self-contained finding or topic, with sharp keywords. Search and ranking are good — several focused notes beat one sprawling document. Search before storing to avoid duplicates; correct with supersedes, never re-store.`,
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "store_memory",
		Description: "Store a memory for future agents — do this unprompted, at the moment of the finding: a confirmed root cause, an environment or tooling gotcha, a decision and its why, a stated preference, a location. Don't batch to end-of-session. If your harness has builtin memory, durable findings go here instead — recall is shared across sessions and harnesses, the builtin is not. Prefer small, focused memories: each covering one self-contained finding or topic, with sharp keywords; split sprawling write-ups into several notes (search and ranking do the assembling). Search first to avoid duplicates; pass supersedes to correct an existing memory.",
	}, storeHandler(svc))

	sdk.AddTool(server, &sdk.Tool{
		Name:        "search_memory",
		Description: "Search memories by keywords, full-text query, kind, and dates. Keywords OR-match and boost rank, like pack — set and=true to require every keyword when narrowing hard. Returns ranked summaries — use get_memory for full content. Page with offset (ordering is stable); order swaps relevance ranking for a display order (created_asc, created_desc, rating_asc, rating_desc, reads_asc, reads_desc). Reach for this on your own whenever a past agent might have learned something relevant — including mid-session, when the work shifts to a topic the opening pack didn't cover.",
	}, searchHandler(svc))

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_memory",
		Description: "Fetch a memory's full content by id.",
	}, getHandler(svc))

	sdk.AddTool(server, &sdk.Tool{
		Name:        "rate_memory",
		Description: "Rate a memory after using it: up if it was correct and helpful, down if wrong or misleading. Ratings drive search ranking.",
	}, rateHandler(svc))

	sdk.AddTool(server, &sdk.Tool{
		Name:        "pack",
		Description: "Session bootstrap: top-ranked memories for the given keywords assembled into one markdown block within a character budget. Call this on your own when past work plausibly touched the same or a related problem — a familiar project, recent changes leading to the current task, a recurrent-sounding issue. Skip it when the task is clearly fresh ground; a reflexive pack pulls irrelevant context in. Keywords OR-match — throw in every candidate topic; memories matching more of them rank higher. If nothing is found, fall back to your harness's builtin memory for reading (if you have one) — new findings still get stored in recall either way.",
	}, packHandler(svc))

	return server
}

// Run serves over stdio until the client disconnects or ctx is cancelled.
func Run(ctx context.Context, server *sdk.Server) error {
	return server.Run(ctx, &sdk.StdioTransport{})
}

type storeIn struct {
	Content  string   `json:"content" jsonschema:"the memory body, markdown"`
	Summary  string   `json:"summary" jsonschema:"required one-line summary shown in search results"`
	Kind     string   `json:"kind" jsonschema:"one of: fact, preference, research, decision, location, reference"`
	Keywords []string `json:"keywords" jsonschema:"at least one; lowercase topic tags, conventions like project:name welcome"`
	TTLHours int      `json:"ttlHours,omitempty" jsonschema:"optional expiry in hours for time-boxed facts"`
	// Supersedes corrects an existing memory: it leaves default search, this
	// one replaces it.
	Supersedes string `json:"supersedes,omitempty" jsonschema:"id of the memory this one corrects"`
	Source     string `json:"source,omitempty" jsonschema:"who is writing, e.g. harness/model/session"`
}

type storeOut struct {
	ID string `json:"id"`
}

func storeHandler(svc memories.Service) sdk.ToolHandlerFor[storeIn, storeOut] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in storeIn) (*sdk.CallToolResult, storeOut, error) {
		source := in.Source
		if source == "" {
			source = defaultSource
		}
		id, err := svc.Store(ctx, managememories.StoreInput{
			Content: in.Content, Summary: in.Summary, Kind: in.Kind,
			Keywords: in.Keywords, Source: source, TTLHours: in.TTLHours, Supersedes: in.Supersedes,
		})
		if err != nil {
			return nil, storeOut{}, err
		}
		return nil, storeOut{ID: string(id)}, nil
	}
}

type searchIn struct {
	Query    string   `json:"query,omitempty" jsonschema:"full-text query layered over the keyword results"`
	Keywords []string `json:"keywords,omitempty" jsonschema:"any may match (OR); more matches rank higher"`
	And      bool     `json:"and,omitempty" jsonschema:"require every keyword to match instead of any"`
	Kind     string   `json:"kind,omitempty"`
	Since    string   `json:"since,omitempty" jsonschema:"RFC3339 or YYYY-MM-DD"`
	Until    string   `json:"until,omitempty" jsonschema:"RFC3339 or YYYY-MM-DD"`
	Limit    int      `json:"limit,omitempty" jsonschema:"default 20"`
	Offset   int      `json:"offset,omitempty" jsonschema:"ranked rows to skip — pagination; ordering is stable across pages"`
	Order    string   `json:"order,omitempty" jsonschema:"display order instead of relevance: created_asc|created_desc|rating_asc|rating_desc|reads_asc|reads_desc"`
	All      bool     `json:"all,omitempty" jsonschema:"include superseded and expired memories"`
}

type searchOut struct {
	Results []domain.SearchResult `json:"results"`
	// Total counts every match regardless of limit/offset — page with
	// offset when it exceeds len(results).
	Total int `json:"total"`
}

func searchHandler(svc memories.Service) sdk.ToolHandlerFor[searchIn, searchOut] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in searchIn) (*sdk.CallToolResult, searchOut, error) {
		since, err := parseTime(in.Since)
		if err != nil {
			return nil, searchOut{}, err
		}
		until, err := parseTime(in.Until)
		if err != nil {
			return nil, searchOut{}, err
		}
		filters := ports.SearchFilters{
			Query: in.Query, Kind: in.Kind,
			Since: since, Until: until, Limit: in.Limit,
			Offset: in.Offset, Order: in.Order, IncludeDead: in.All,
		}
		if in.And {
			filters.Keywords = in.Keywords
		} else {
			filters.KeywordsAny = in.Keywords
		}
		page, err := svc.Search(ctx, filters)
		if err != nil {
			return nil, searchOut{}, err
		}
		return nil, searchOut{Results: page.Results, Total: page.Total}, nil
	}
}

type getIn struct {
	ID string `json:"id"`
}

func getHandler(svc memories.Service) sdk.ToolHandlerFor[getIn, domain.MemoryReadModel] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in getIn) (*sdk.CallToolResult, domain.MemoryReadModel, error) {
		model, err := svc.Get(ctx, domain.MemoryID(in.ID))
		if err != nil {
			return nil, domain.MemoryReadModel{}, err
		}
		return nil, *model, nil
	}
}

type rateIn struct {
	ID      string `json:"id"`
	Verdict string `json:"verdict" jsonschema:"up or down"`
}

type rateOut struct {
	OK bool `json:"ok"`
}

func rateHandler(svc memories.Service) sdk.ToolHandlerFor[rateIn, rateOut] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in rateIn) (*sdk.CallToolResult, rateOut, error) {
		if err := svc.Rate(ctx, domain.MemoryID(in.ID), in.Verdict != "down"); err != nil {
			return nil, rateOut{}, err
		}
		return nil, rateOut{OK: true}, nil
	}
}

type packIn struct {
	Keywords    []string `json:"keywords" jsonschema:"any may match (OR); more matches rank higher"`
	Text        string   `json:"text,omitempty" jsonschema:"optional full-text query narrowing the keyword results"`
	BudgetChars int      `json:"budgetChars,omitempty" jsonschema:"max characters, default 4000"`
}

type packOut struct {
	Context string `json:"context"`
}

func packHandler(svc memories.Service) sdk.ToolHandlerFor[packIn, packOut] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in packIn) (*sdk.CallToolResult, packOut, error) {
		pack, err := svc.Recall(ctx, in.Keywords, in.Text, in.BudgetChars)
		if err != nil {
			return nil, packOut{}, err
		}
		return nil, packOut{Context: pack}, nil
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
