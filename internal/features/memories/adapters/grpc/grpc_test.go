package grpcadapter_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	recallv1 "github.com/KucherenkoIvan/go-kernel/contracts/gen/grpc/recall/v1"
	"github.com/KucherenkoIvan/go-kernel/events"
	"github.com/KucherenkoIvan/go-kernel/grpckit"
	"github.com/KucherenkoIvan/go-kernel/grpckit/grpckittest"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/adapters/grpcauth"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	grpcadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/grpc"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

// mapResolver backs each space with a real local service over :memory:.
type mapResolver struct {
	t      *testing.T
	spaces map[string]memories.Service
}

func (r *mapResolver) ServiceFor(ctx context.Context, space string) (memories.Service, error) {
	if svc, ok := r.spaces[space]; ok {
		return svc, nil
	}
	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		return nil, err
	}
	r.t.Cleanup(func() { _ = store.Close() })
	pub := events.NewChannelPublisher()
	r.t.Cleanup(func() { _ = pub.Close(context.Background()) })
	svc := memories.New(store.DB, pub)
	r.spaces[space] = svc
	return svc, nil
}

// setup builds the full hosted stack in-process: real apikeys service, real
// auth interceptor, real handler over real per-space local services.
func setup(t *testing.T) (keys apikeys.Service, dial func(apiKey string) *grpcadapter.Client) {
	t.Helper()

	keysStore, err := storage.OpenServer(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = keysStore.Close() })
	keys = apikeys.New(keysStore.DB)

	srv := grpckit.NewServer(
		grpckit.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		grpckit.WithUnaryInterceptor(grpcauth.UnaryInterceptor(keys)),
	)
	recallv1.RegisterMemoryServiceServer(srv, grpcadapter.NewHandler(&mapResolver{t: t, spaces: map[string]memories.Service{}}))
	conn := grpckittest.Serve(t, srv)

	return keys, func(apiKey string) *grpcadapter.Client {
		return grpcadapter.NewClient(conn, apiKey)
	}
}

func TestRemoteRoundtrip_SourceIsKeyName(t *testing.T) {
	keys, dial := setup(t)
	ctx := context.Background()

	created, err := keys.Create(ctx, "laptop-ivan", "team-x")
	if err != nil {
		t.Fatal(err)
	}
	client := dial(created.RawToken)

	// store claims source "cli" — the server must override with the key name
	id, err := client.Store(ctx, managememories.StoreInput{
		Content: "remote memories work end to end", Summary: "remote e2e",
		Kind: "fact", Keywords: []string{"e2e", "project:recall"}, Source: "cli",
	})
	if err != nil {
		t.Fatal(err)
	}

	model, err := client.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if model.Source != "laptop-ivan" {
		t.Fatalf("source must be the key name, got %q", model.Source)
	}
	if model.Content != "remote memories work end to end" || len(model.Keywords) != 2 {
		t.Fatalf("model roundtrip: %+v", model)
	}

	// search with FTS + keyword filter through the wire
	results, err := client.Search(ctx, ports.SearchFilters{Query: "remote", Keywords: []string{"e2e"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != string(id) || results[0].Snippet == "" {
		t.Fatalf("search: %+v", results)
	}

	// rate + recall + delete
	if err := client.Rate(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	pack, err := client.Recall(ctx, []string{"project:recall"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pack, "remote e2e") {
		t.Fatalf("recall pack:\n%s", pack)
	}
	if err := client.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}

	// typed domain errors survive the wire
	var notFound *domain.MemoryNotFoundError
	if _, err := client.Get(ctx, id); !errors.As(err, &notFound) {
		t.Fatalf("not-found must map to the typed domain error, got: %v", err)
	}
}

func TestRemoteValidation_TypedErrorsAcrossTheWire(t *testing.T) {
	keys, dial := setup(t)
	ctx := context.Background()

	created, _ := keys.Create(ctx, "k", "space-a")
	client := dial(created.RawToken)

	var invalidKind *domain.InvalidKindError
	_, err := client.Store(ctx, managememories.StoreInput{
		Content: "x", Summary: "x", Kind: "nonsense", Keywords: []string{"k"},
	})
	if !errors.As(err, &invalidKind) {
		t.Fatalf("invalid kind: %v", err)
	}
}

func TestRemoteAuth_BadKeyIsFriendly(t *testing.T) {
	_, dial := setup(t)

	client := dial("rcl_" + strings.Repeat("0", 64))
	_, err := client.Search(context.Background(), ports.SearchFilters{})
	if err == nil || !strings.Contains(err.Error(), "recall remote status") {
		t.Fatalf("bad key error must point at remote status: %v", err)
	}
}

func TestSpaceIsolation_AtTheHandler(t *testing.T) {
	keys, dial := setup(t)
	ctx := context.Background()

	keyA, _ := keys.Create(ctx, "a", "space-a")
	keyB, _ := keys.Create(ctx, "b", "space-b")
	clientA, clientB := dial(keyA.RawToken), dial(keyB.RawToken)

	id, err := clientA.Store(ctx, managememories.StoreInput{
		Content: "secret of space a", Summary: "a's memory",
		Kind: "fact", Keywords: []string{"shared-keyword"},
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := clientB.Search(ctx, ports.SearchFilters{Keywords: []string{"shared-keyword"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("space b sees space a's memories: %+v", results)
	}

	var notFound *domain.MemoryNotFoundError
	if _, err := clientB.Get(ctx, id); !errors.As(err, &notFound) {
		t.Fatalf("cross-space get must be not-found: %v", err)
	}
	if err := clientB.Delete(ctx, id); !errors.As(err, &notFound) {
		t.Fatalf("cross-space delete must be not-found: %v", err)
	}

	// a's memory is untouched
	if _, err := clientA.Get(ctx, id); err != nil {
		t.Fatalf("a's memory must survive b's attempts: %v", err)
	}
}
