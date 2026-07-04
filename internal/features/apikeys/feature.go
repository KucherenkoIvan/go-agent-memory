// Package apikeys is the hosted-mode control plane: API keys that both
// authenticate callers and select the memory space they may touch. Faces
// (admin CLI, the gRPC auth interceptor) depend on the Service facade.
package apikeys

import (
	"context"

	"github.com/KucherenkoIvan/go-kernel/ddd"
	kernelsqlite "github.com/KucherenkoIvan/go-kernel/sqlite"

	sqliteadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/adapters/sqlite"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/application/usecases/managekeys"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/domain"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/identity"
)

// Service is the application facade over key management.
type Service interface {
	// Create mints a key into a space; the result carries the raw token —
	// the only time it exists in the clear.
	Create(ctx context.Context, name, space string) (managekeys.CreateResult, error)
	Revoke(ctx context.Context, id string) error
	List(ctx context.Context) ([]domain.APIKeyView, error)
	Spaces(ctx context.Context) ([]domain.SpaceView, error)
	// Authenticate resolves a raw token; unknown and revoked tokens fail
	// identically.
	Authenticate(ctx context.Context, rawToken string) (identity.Identity, error)
}

// New wires the local Service over the control-plane database (keys.db).
func New(db *kernelsqlite.Client) Service {
	var repo ports.APIKeyRepository = sqliteadapter.NewAPIKeyRepository(db)
	var txManager ddd.TxManager = db

	return &localService{
		create:       managekeys.NewCreateKeyCommand(txManager, ddd.UUIDv7Generator{}, ddd.SystemClock{}, repo),
		revoke:       managekeys.NewRevokeKeyCommand(txManager, ddd.SystemClock{}, repo),
		list:         managekeys.NewListKeysQuery(repo),
		spaces:       managekeys.NewListSpacesQuery(repo),
		authenticate: managekeys.NewAuthenticateQuery(repo),
	}
}

type localService struct {
	create       *managekeys.CreateKeyCommand
	revoke       *managekeys.RevokeKeyCommand
	list         *managekeys.ListKeysQuery
	spaces       *managekeys.ListSpacesQuery
	authenticate *managekeys.AuthenticateQuery
}

func (s *localService) Create(ctx context.Context, name, space string) (managekeys.CreateResult, error) {
	return s.create.Execute(ctx, name, space)
}

func (s *localService) Revoke(ctx context.Context, id string) error {
	return s.revoke.Execute(ctx, domain.APIKeyID(id))
}

func (s *localService) List(ctx context.Context) ([]domain.APIKeyView, error) {
	return s.list.Execute(ctx)
}

func (s *localService) Spaces(ctx context.Context) ([]domain.SpaceView, error) {
	return s.spaces.Execute(ctx)
}

func (s *localService) Authenticate(ctx context.Context, rawToken string) (identity.Identity, error) {
	return s.authenticate.Execute(ctx, rawToken)
}
