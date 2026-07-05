package server

import (
	"context"
	"path/filepath"

	"github.com/KucherenkoIvan/go-kernel/app"
	recallv1 "github.com/KucherenkoIvan/go-kernel/contracts/gen/grpc/recall/v1"
	"github.com/KucherenkoIvan/go-kernel/grpckit"
	"github.com/KucherenkoIvan/go-kernel/health"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/adapters/grpcauth"
	grpcadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/grpc"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

// Run serves shared memory over gRPC until the context is cancelled or a
// signal arrives. Transport is plaintext (kernel grpckit) — deploy on a
// private network or behind a TLS-terminating proxy.
func Run(ctx context.Context, cfg Config) error {
	keys, err := storage.OpenServer(ctx, filepath.Join(cfg.Dir, "keys.db"))
	if err != nil {
		return err
	}
	keysSvc := apikeys.New(keys.DB)
	registry := NewSpaceRegistry(filepath.Join(cfg.Dir, "spaces"))

	srv := grpckit.NewServer(
		grpckit.WithErrorCode("memory_not_found", codes.NotFound),
		grpckit.WithErrorCode("empty_content", codes.InvalidArgument),
		grpckit.WithErrorCode("invalid_summary", codes.InvalidArgument),
		grpckit.WithErrorCode("invalid_kind", codes.InvalidArgument),
		grpckit.WithErrorCode("no_keywords", codes.InvalidArgument),
		grpckit.WithErrorCode("empty_source", codes.InvalidArgument),
		grpckit.WithErrorCode("invalid_ttl", codes.InvalidArgument),
		grpckit.WithErrorCode("invalid_supersede", codes.InvalidArgument),
		grpckit.WithUnaryInterceptor(grpcauth.UnaryInterceptor(keysSvc)),
	)
	recallv1.RegisterMemoryServiceServer(srv, grpcadapter.NewHandler(registry))
	health.NewChecker(health.Check{Name: "keys-db", Check: keys.DB.Ping}).RegisterGRPC(srv)
	reflection.Register(srv) // the contract explorer reaches us via reflection

	return app.Run(ctx,
		app.Adapter{Name: "keys-db", Close: func(context.Context) error { return keys.Close() }},
		app.Adapter{Name: "spaces", Close: registry.Close},
		app.Adapter{Name: "grpc", Run: func(ctx context.Context) error { return grpckit.Run(ctx, srv, cfg.Addr) }},
	)
}
