// Package grpcadapter is the gRPC transport for the memories feature: the
// server-side handler (hosted mode's face) and the client-side Service
// implementation (what a remote-configured recall runs on).
package grpcadapter

import (
	"context"
	"time"

	recallv1 "github.com/KucherenkoIvan/go-kernel/contracts/gen/grpc/recall/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/identity"
)

// ServiceResolver hands out the per-space Service — the seam between the
// transport and the server's space registry.
type ServiceResolver interface {
	ServiceFor(ctx context.Context, space string) (memories.Service, error)
}

type Handler struct {
	recallv1.UnimplementedMemoryServiceServer
	resolver ServiceResolver
}

func NewHandler(resolver ServiceResolver) *Handler {
	return &Handler{resolver: resolver}
}

// resolve is every RPC's front door: the auth interceptor must have run.
func (h *Handler) resolve(ctx context.Context) (memories.Service, identity.Identity, error) {
	ident, ok := identity.From(ctx)
	if !ok {
		// defense in depth — only reachable if the interceptor is miswired
		return nil, identity.Identity{}, status.Error(codes.Unauthenticated, "invalid_api_key")
	}
	svc, err := h.resolver.ServiceFor(ctx, ident.Space)
	if err != nil {
		return nil, identity.Identity{}, err
	}
	return svc, ident, nil
}

func (h *Handler) Store(ctx context.Context, req *recallv1.StoreRequest) (*recallv1.StoreResponse, error) {
	svc, ident, err := h.resolve(ctx)
	if err != nil {
		return nil, err
	}
	id, err := svc.Store(ctx, managememories.StoreInput{
		Content:  req.GetContent(),
		Summary:  req.GetSummary(),
		Kind:     req.GetKind(),
		Keywords: req.GetKeywords(),
		// the key identity is the writer — client-supplied source is advisory
		Source:     ident.KeyName,
		TTLHours:   int(req.GetTtlHours()),
		Supersedes: req.GetSupersedes(),
	})
	if err != nil {
		return nil, err
	}
	return &recallv1.StoreResponse{Id: string(id)}, nil
}

func (h *Handler) Search(ctx context.Context, req *recallv1.SearchRequest) (*recallv1.SearchResponse, error) {
	svc, _, err := h.resolve(ctx)
	if err != nil {
		return nil, err
	}
	results, err := svc.Search(ctx, ports.SearchFilters{
		Query:       req.GetQuery(),
		Keywords:    req.GetKeywords(),
		KeywordsAny: req.GetKeywordsAny(),
		Kind:        req.GetKind(),
		Since:       fromTimestamp(req.GetSince()),
		Until:       fromTimestamp(req.GetUntil()),
		Limit:       int(req.GetLimit()),
		Offset:      int(req.GetOffset()),
		Order:       req.GetOrder(),
		IncludeDead: req.GetIncludeDead(),
	})
	if err != nil {
		return nil, err
	}

	out := make([]*recallv1.SearchResult, 0, len(results))
	for _, r := range results {
		out = append(out, &recallv1.SearchResult{
			Id: r.ID, Summary: r.Summary, Kind: r.Kind, Keywords: r.Keywords,
			Source: r.Source, CreatedAt: timestamppb.New(r.CreatedAt),
			ExpiresAt: optionalTimestamp(r.ExpiresAt), Snippet: r.Snippet,
			Score: r.Score, VotesUp: int32(r.VotesUp), VotesDown: int32(r.VotesDown), //nolint:gosec // vote counts
			AccessCount: int32(r.AccessCount), //nolint:gosec // access counts
		})
	}
	return &recallv1.SearchResponse{Results: out}, nil
}

func (h *Handler) Get(ctx context.Context, req *recallv1.GetRequest) (*recallv1.GetResponse, error) {
	svc, _, err := h.resolve(ctx)
	if err != nil {
		return nil, err
	}
	model, err := svc.Get(ctx, domain.MemoryID(req.GetId()))
	if err != nil {
		return nil, err
	}
	return &recallv1.GetResponse{Memory: &recallv1.Memory{
		Id: model.ID, Content: model.Content, Summary: model.Summary,
		Kind: model.Kind, Keywords: model.Keywords, Source: model.Source,
		CreatedAt: timestamppb.New(model.CreatedAt), ExpiresAt: optionalTimestamp(model.ExpiresAt),
		SupersededBy: model.SupersededBy,
		VotesUp:      int32(model.VotesUp), VotesDown: int32(model.VotesDown), //nolint:gosec // vote counts
		AccessCount: int32(model.AccessCount), //nolint:gosec // access counts
	}}, nil
}

func (h *Handler) Rate(ctx context.Context, req *recallv1.RateRequest) (*recallv1.RateResponse, error) {
	svc, _, err := h.resolve(ctx)
	if err != nil {
		return nil, err
	}
	if err := svc.Rate(ctx, domain.MemoryID(req.GetId()), req.GetUp()); err != nil {
		return nil, err
	}
	return &recallv1.RateResponse{}, nil
}

func (h *Handler) Recall(ctx context.Context, req *recallv1.RecallRequest) (*recallv1.RecallResponse, error) {
	svc, _, err := h.resolve(ctx)
	if err != nil {
		return nil, err
	}
	pack, err := svc.Recall(ctx, req.GetKeywords(), req.GetText(), int(req.GetBudgetChars()))
	if err != nil {
		return nil, err
	}
	return &recallv1.RecallResponse{Context: pack}, nil
}

func (h *Handler) Delete(ctx context.Context, req *recallv1.DeleteRequest) (*recallv1.DeleteResponse, error) {
	svc, _, err := h.resolve(ctx)
	if err != nil {
		return nil, err
	}
	if err := svc.Delete(ctx, domain.MemoryID(req.GetId())); err != nil {
		return nil, err
	}
	return &recallv1.DeleteResponse{}, nil
}

func fromTimestamp(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

func optionalTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}
