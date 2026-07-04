package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/KucherenkoIvan/go-kernel/app"
	"github.com/KucherenkoIvan/go-kernel/config"
	changemev1 "github.com/KucherenkoIvan/go-kernel/contracts/gen/grpc/changeme/v1"
	"github.com/KucherenkoIvan/go-kernel/events"
	"github.com/KucherenkoIvan/go-kernel/grpckit"
	"github.com/KucherenkoIvan/go-kernel/health"
	"github.com/KucherenkoIvan/go-kernel/httpapi"
	"github.com/KucherenkoIvan/go-kernel/observability"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/shared/infra/storage"
)

type AppConfig struct {
	HTTPAddr string `env:"HTTP_ADDR" default:":8080"`
	GRPCAddr string `env:"GRPC_ADDR" default:":9090"`
	DBPath   string `env:"DB_PATH" default:"app.db"`
}

func main() {
	// 1. observe first
	logger := observability.NewLogger("tinycore") // rename with the project
	slog.SetDefault(logger)

	// 2. config: loaded once, passed down as values
	cfg := config.MustLoad[AppConfig]()
	ctx := context.Background()

	// 3. infra
	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		logger.Error("opening storage", "error", err)
		os.Exit(1)
	}
	pub := events.NewChannelPublisher(events.WithLogger(logger))

	// 4. features
	changeMeFeature := changeme.New(db, pub)

	// 5. transports — domain error codes map once, here
	r := httpapi.NewRouter(
		httpapi.WithLogger(logger),
		httpapi.WithErrorStatus("changeme_not_found", http.StatusNotFound),
	)
	changeMeFeature.Handlers.RegisterRoutes(r.Group("/api"))

	srv := grpckit.NewServer(
		grpckit.WithLogger(logger),
		grpckit.WithErrorCode("changeme_not_found", codes.NotFound),
	)
	changemev1.RegisterChangeMeServiceServer(srv, changeMeFeature.GRPC)

	// 6. health
	checker := health.NewChecker(health.Check{Name: "sqlite", Check: db.Ping})
	r.GET("/livez", gin.WrapH(health.Liveness()))
	r.GET("/healthz", gin.WrapH(checker.Handler()))
	checker.RegisterGRPC(srv)

	// 7. run — registration order is dependency order
	if err := app.Run(ctx,
		app.Adapter{Name: "sqlite", Close: func(context.Context) error { return db.Close() }},
		app.Adapter{Name: "events", Run: pub.Run, Close: pub.Close},
		app.Adapter{Name: "http", Run: func(ctx context.Context) error { return httpapi.Run(ctx, r, cfg.HTTPAddr) }},
		app.Adapter{Name: "grpc", Run: func(ctx context.Context) error { return grpckit.Run(ctx, srv, cfg.GRPCAddr) }},
	); err != nil {
		logger.Error("service failed", "error", err)
		os.Exit(1)
	}
}
