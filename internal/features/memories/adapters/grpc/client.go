package grpcadapter

import (
	"context"
	"time"

	recallv1 "github.com/KucherenkoIvan/go-kernel/contracts/gen/grpc/recall/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// Client is the remote implementation of memories.Service: same facade,
// gRPC underneath. Faces never know which one they got.
type Client struct {
	rpc    recallv1.MemoryServiceClient
	apiKey string
}

func NewClient(conn grpc.ClientConnInterface, apiKey string) *Client {
	return &Client{rpc: recallv1.NewMemoryServiceClient(conn), apiKey: apiKey}
}

func (c *Client) auth(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.apiKey)
}

func (c *Client) Store(ctx context.Context, in managememories.StoreInput) (domain.MemoryID, error) {
	resp, err := c.rpc.Store(c.auth(ctx), &recallv1.StoreRequest{
		Content: in.Content, Summary: in.Summary, Kind: in.Kind,
		Keywords: in.Keywords, Source: in.Source,
		TtlHours:   int32(in.TTLHours), //nolint:gosec // domain caps ttl
		Supersedes: in.Supersedes,
	})
	if err != nil {
		return "", mapRemoteError(err)
	}
	return domain.MemoryID(resp.GetId()), nil
}

func (c *Client) Search(ctx context.Context, filters ports.SearchFilters) (domain.SearchPage, error) {
	var since, until *timestamppb.Timestamp
	if !filters.Since.IsZero() {
		since = timestamppb.New(filters.Since)
	}
	if !filters.Until.IsZero() {
		until = timestamppb.New(filters.Until)
	}
	resp, err := c.rpc.Search(c.auth(ctx), &recallv1.SearchRequest{
		Query: filters.Query, Keywords: filters.Keywords, KeywordsAny: filters.KeywordsAny,
		Kind: filters.Kind, Since: since, Until: until,
		Limit:       int32(filters.Limit),  //nolint:gosec // use-case caps limit
		Offset:      int32(filters.Offset), //nolint:gosec // non-negative by use-case
		Order:       filters.Order,
		SkipTotal:   filters.SkipTotal,
		IncludeDead: filters.IncludeDead,
	})
	if err != nil {
		return domain.SearchPage{}, mapRemoteError(err)
	}

	results := make([]domain.SearchResult, 0, len(resp.GetResults()))
	for _, r := range resp.GetResults() {
		results = append(results, domain.SearchResult{
			ID: r.GetId(), Summary: r.GetSummary(), Kind: r.GetKind(),
			Keywords: r.GetKeywords(), Source: r.GetSource(),
			CreatedAt: r.GetCreatedAt().AsTime(), ExpiresAt: optionalTime(r.GetExpiresAt()),
			Snippet: r.GetSnippet(), Score: r.GetScore(),
			VotesUp: int(r.GetVotesUp()), VotesDown: int(r.GetVotesDown()),
			AccessCount: int(r.GetAccessCount()),
		})
	}
	return domain.SearchPage{Results: results, Total: int(resp.GetTotal())}, nil
}

func (c *Client) Get(ctx context.Context, id domain.MemoryID) (*domain.MemoryReadModel, error) {
	resp, err := c.rpc.Get(c.auth(ctx), &recallv1.GetRequest{Id: string(id)})
	if err != nil {
		return nil, mapRemoteError(err)
	}
	m := resp.GetMemory()
	return &domain.MemoryReadModel{
		ID: m.GetId(), Content: m.GetContent(), Summary: m.GetSummary(),
		Kind: m.GetKind(), Keywords: m.GetKeywords(), Source: m.GetSource(),
		CreatedAt: m.GetCreatedAt().AsTime(), ExpiresAt: optionalTime(m.GetExpiresAt()),
		SupersededBy: m.GetSupersededBy(),
		VotesUp:      int(m.GetVotesUp()), VotesDown: int(m.GetVotesDown()),
		AccessCount: int(m.GetAccessCount()),
	}, nil
}

func (c *Client) Rate(ctx context.Context, id domain.MemoryID, up bool) error {
	_, err := c.rpc.Rate(c.auth(ctx), &recallv1.RateRequest{Id: string(id), Up: up})
	return mapRemoteError(err)
}

func (c *Client) Recall(ctx context.Context, keywords []string, text string, budgetChars int) (string, error) {
	resp, err := c.rpc.Recall(c.auth(ctx), &recallv1.RecallRequest{
		Keywords:    keywords,
		Text:        text,
		BudgetChars: int32(budgetChars), //nolint:gosec // use-case defaults budget
	})
	if err != nil {
		return "", mapRemoteError(err)
	}
	return resp.GetContext(), nil
}

func (c *Client) Delete(ctx context.Context, id domain.MemoryID) error {
	_, err := c.rpc.Delete(c.auth(ctx), &recallv1.DeleteRequest{Id: string(id)})
	return mapRemoteError(err)
}

func optionalTime(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}
