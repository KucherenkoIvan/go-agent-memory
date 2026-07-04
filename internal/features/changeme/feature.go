// Package changeme is the placeholder feature — the layout to copy for real
// features, then delete: aggregate + CRUD events, commands and queries,
// sqlite adapters, REST and gRPC transports.
package changeme

import (
	"github.com/KucherenkoIvan/go-kernel/ddd"
	"github.com/KucherenkoIvan/go-kernel/events"
	kernelsqlite "github.com/KucherenkoIvan/go-kernel/sqlite"

	grpcadapter "github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/adapters/grpc"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/adapters/rest"
	sqliteadapter "github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/adapters/sqlite"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/application/ports"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/application/usecases/managechangeme"
)

// Feature is what a composition root sees: the application surface and the
// transport adapters built on it. Different binaries mount different parts —
// cmd/app uses Handlers + GRPC, cmd/tui builds its UI over UseCases.
type Feature struct {
	UseCases managechangeme.UseCases
	Handlers *rest.Handlers                  // r.Group("/api") → Handlers.RegisterRoutes
	GRPC     *grpcadapter.ChangeMeController // changemev1.RegisterChangeMeServiceServer(srv, GRPC)
}

// New is the feature's composition root: port → adapter, then use-cases,
// then transports. No business logic here — only wiring.
func New(db *kernelsqlite.Client, pub *events.ChannelPublisher) *Feature {
	// ports → adapters
	var repo ports.ChangeMeRepository = sqliteadapter.NewChangeMeRepository(db)
	var reader ports.ChangeMeReader = sqliteadapter.NewChangeMeReader(db)
	var producer ports.ChangeMeEventProducer = pub // an outbox adapter would slot in here
	var txManager ddd.TxManager = db

	// use-cases
	uc := managechangeme.UseCases{
		Create: managechangeme.NewCreateCommand(txManager, ddd.UUIDv7Generator{}, ddd.SystemClock{}, repo, producer),
		Update: managechangeme.NewUpdateCommand(txManager, repo, producer),
		Delete: managechangeme.NewDeleteCommand(txManager, repo, producer),
		Get:    managechangeme.NewGetQuery(reader),
		List:   managechangeme.NewListQuery(reader),
	}

	// async reactions go here when they appear:
	// pub.Subscribe(events.Handler{Name: "...", Events: []string{...}, Handle: ...})

	return &Feature{
		UseCases: uc,
		Handlers: rest.NewHandlers(uc.Create, uc.Update, uc.Delete, uc.Get, uc.List),
		GRPC:     grpcadapter.NewChangeMeController(uc.Create, uc.Update, uc.Delete, uc.Get, uc.List),
	}
}
