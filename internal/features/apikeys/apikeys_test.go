package apikeys_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/domain"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

func setup(t *testing.T) apikeys.Service {
	t.Helper()
	store, err := storage.OpenServer(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return apikeys.New(store.DB)
}

func TestKeyLifecycle_CreateAuthenticateRevoke(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "laptop-ivan", "team-x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.RawToken, "agm_") {
		t.Fatalf("raw token: %q", created.RawToken)
	}
	if created.Key.Space != "team-x" || created.Key.Name != "laptop-ivan" {
		t.Fatalf("view: %+v", created.Key)
	}

	// authenticate resolves the raw token to key identity + space
	ident, err := svc.Authenticate(ctx, created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	if ident.Space != "team-x" || ident.KeyName != "laptop-ivan" || ident.KeyID != created.Key.ID {
		t.Fatalf("identity: %+v", ident)
	}

	// unknown tokens fail with the same error revoked ones will
	var invalid *domain.InvalidAPIKeyError
	if _, err := svc.Authenticate(ctx, "agm_"+strings.Repeat("0", 64)); !errors.As(err, &invalid) {
		t.Fatalf("unknown token: %v", err)
	}

	if err := svc.Revoke(ctx, created.Key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, created.RawToken); !errors.As(err, &invalid) {
		t.Fatalf("revoked token must fail authentication: %v", err)
	}

	var notFound *domain.KeyNotFoundError
	if err := svc.Revoke(ctx, "missing-id"); !errors.As(err, &notFound) {
		t.Fatalf("revoking a missing key: %v", err)
	}
}

func TestListings_NeverLeakSecrets(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()

	first, _ := svc.Create(ctx, "a", "team-x")
	_, _ = svc.Create(ctx, "b", "team-x")
	_, _ = svc.Create(ctx, "c", "solo")
	_ = svc.Revoke(ctx, first.Key.ID)

	keys, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("keys: %+v", keys)
	}
	for _, k := range keys {
		if strings.Contains(k.Prefix, first.RawToken) || len(k.Prefix) != 12 {
			t.Fatalf("listing leaks token material: %+v", k)
		}
	}
	if keys[0].RevokedAt == nil {
		t.Fatalf("revoked key must carry the timestamp: %+v", keys[0])
	}

	spaces, err := svc.Spaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// team-x has 1 active key (a revoked), solo has 1
	if len(spaces) != 2 || spaces[1].Name != "team-x" || spaces[1].Keys != 1 || spaces[0].Keys != 1 {
		t.Fatalf("spaces: %+v", spaces)
	}
}

func TestCreate_RejectsBadSpaceNames(t *testing.T) {
	svc := setup(t)

	var invalid *domain.InvalidSpaceNameError
	if _, err := svc.Create(context.Background(), "k", "../../etc"); !errors.As(err, &invalid) {
		t.Fatalf("traversal space name: %v", err)
	}
}
