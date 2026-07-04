package grpcauth_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/adapters/grpcauth"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/identity"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

func setup(t *testing.T) (grpc.UnaryServerInterceptor, string) {
	t.Helper()
	store, err := storage.OpenServer(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := apikeys.New(store.DB)
	created, err := svc.Create(context.Background(), "test-key", "team-x")
	if err != nil {
		t.Fatal(err)
	}
	return grpcauth.UnaryInterceptor(svc), created.RawToken
}

func call(interceptor grpc.UnaryServerInterceptor, ctx context.Context, method string) (identity.Identity, error) {
	var seen identity.Identity
	handler := func(ctx context.Context, _ any) (any, error) {
		seen, _ = identity.From(ctx)
		return nil, nil
	}
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, handler)
	return seen, err
}

func withAuth(token string) context.Context {
	return metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer "+token))
}

func TestUnaryInterceptor(t *testing.T) {
	interceptor, token := setup(t)
	const method = "/grpc.agmem.v1.MemoryService/Search"

	// valid token → handler runs with identity in context
	ident, err := call(interceptor, withAuth(token), method)
	if err != nil {
		t.Fatal(err)
	}
	if ident.Space != "team-x" || ident.KeyName != "test-key" {
		t.Fatalf("identity: %+v", ident)
	}

	// all failure modes read identically: Unauthenticated invalid_api_key
	for name, ctx := range map[string]context.Context{
		"no metadata":   context.Background(),
		"no header":     metadata.NewIncomingContext(context.Background(), metadata.MD{}),
		"not bearer":    metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", token)),
		"empty bearer":  metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer ")),
		"unknown token": withAuth("agm_deadbeef00000000000000000000000000000000000000000000000000000000"),
	} {
		_, err := call(interceptor, ctx, method)
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.Unauthenticated || st.Message() != "invalid_api_key" {
			t.Errorf("%s: %v", name, err)
		}
	}

	// health and reflection bypass auth
	for _, open := range []string{
		"/grpc.health.v1.Health/Check",
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
	} {
		if _, err := call(interceptor, context.Background(), open); err != nil {
			t.Errorf("%s must bypass auth: %v", open, err)
		}
	}
}
