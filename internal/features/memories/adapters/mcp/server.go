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
		Instructions: `Persistent memory shared across sessions, models, and harnesses. Use it on your own initiative — nobody will prompt you to:
- at session start, call pack with your task's topic keywords to recall what past agents learned;
- when you rely on a memory, rate it afterwards — ratings drive ranking for everyone;
- when you learn something worth keeping (a finding, a decision and its why, a preference, a location), store it before finishing.
Store small, atomic memories: one fact per memory, with sharp keywords. Search and ranking are good — many small notes beat one big document. Search before storing to avoid duplicates; correct with supersedes, never re-store.`,
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "store_memory",
		Description: "Store a memory for future agents — do this unprompted whenever you learn something durable. Keep memories small and atomic: one fact each, sharp keywords; split big findings into several notes (search and ranking do the assembling). Search first to avoid duplicates; pass supersedes to correct an existing memory.",
	}, storeHandler(svc))

	sdk.AddTool(server, &sdk.Tool{
		Name:        "search_memory",
		Description: "Search memories by full-text query, keywords, kind, and dates. Returns ranked summaries — use get_memory for full content. Reach for this on your own whenever a past agent might have learned something relevant.",
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
		Description: "Session bootstrap: top-ranked memories for the given keywords assembled into one markdown block within a character budget. Call this unprompted at the start of any task. Keywords OR-match — throw in every candidate topic; memories matching more of them rank higher. If nothing is found, fall back to your harness's builtin memory (if you have one).",
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
	Query    string   `json:"query,omitempty" jsonschema:"full-text query over summaries and content"`
	Keywords []string `json:"keywords,omitempty" jsonschema:"all must match"`
	Kind     string   `json:"kind,omitempty"`
	Since    string   `json:"since,omitempty" jsonschema:"RFC3339 or YYYY-MM-DD"`
	Until    string   `json:"until,omitempty" jsonschema:"RFC3339 or YYYY-MM-DD"`
	Limit    int      `json:"limit,omitempty" jsonschema:"default 20"`
	All      bool     `json:"all,omitempty" jsonschema:"include superseded and expired memories"`
}

type searchOut struct {
	Results []domain.SearchResult `json:"results"`
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
		results, err := svc.Search(ctx, ports.SearchFilters{
			Query: in.Query, Keywords: in.Keywords, Kind: in.Kind,
			Since: since, Until: until, Limit: in.Limit, IncludeDead: in.All,
		})
		if err != nil {
			return nil, searchOut{}, err
		}
		return nil, searchOut{Results: results}, nil
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
	BudgetChars int      `json:"budgetChars,omitempty" jsonschema:"max characters, default 4000"`
}

type packOut struct {
	Context string `json:"context"`
}

func packHandler(svc memories.Service) sdk.ToolHandlerFor[packIn, packOut] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in packIn) (*sdk.CallToolResult, packOut, error) {
		pack, err := svc.Recall(ctx, in.Keywords, in.BudgetChars)
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
