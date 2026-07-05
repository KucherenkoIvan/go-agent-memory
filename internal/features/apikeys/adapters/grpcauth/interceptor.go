// Package grpcauth guards the hosted-mode gRPC surface: every RPC must
// carry a valid API key, whose identity (key name + space) rides the
// context into the handlers.
package grpcauth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/identity"
)

// unauthenticated paths: liveness probes and the reflection-based explorer.
var openPrefixes = []string{"/grpc.health.v1.Health/", "/grpc.reflection."}

// UnaryInterceptor authenticates `authorization: Bearer rcl_...` metadata.
// Every failure mode reads the same — callers never learn whether a token
// is unknown, malformed, or revoked.
func UnaryInterceptor(svc apikeys.Service) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		for _, prefix := range openPrefixes {
			if strings.HasPrefix(info.FullMethod, prefix) {
				return handler(ctx, req)
			}
		}

		token, ok := bearerToken(ctx)
		if !ok {
			return nil, errUnauthenticated
		}
		ident, err := svc.Authenticate(ctx, token)
		if err != nil {
			return nil, errUnauthenticated
		}
		return handler(identity.With(ctx, ident), req)
	}
}

var errUnauthenticated = status.Error(codes.Unauthenticated, "invalid_api_key")

func bearerToken(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", false
	}
	token, found := strings.CutPrefix(values[0], "Bearer ")
	if !found || token == "" {
		return "", false
	}
	return token, true
}
