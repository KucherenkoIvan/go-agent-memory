// Package identity carries the authenticated caller through context.
// Adapters write it (grpc auth interceptor) and adapters read it (grpc
// handlers); use-cases never touch context values — they receive typed
// arguments instead.
package identity

import "context"

// Identity is who holds the API key: the key row and the space it unlocks.
type Identity struct {
	KeyID   string
	KeyName string // becomes memory `source` on server-side stores
	Space   string
}

type ctxKey struct{}

func With(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func From(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}
