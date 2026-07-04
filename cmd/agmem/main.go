// agmem — harness-agnostic memory for AI agents. One binary, three faces:
// CLI subcommands for shell-out agents, `agmem mcp` for MCP harnesses, and
// (phase 2) `agmem tui` for humans.
package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/KucherenkoIvan/go-kernel/events"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys"
	apikeyscli "github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/adapters/cli"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/cli"
	mcpadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/mcp"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

var version = "dev" // set via -ldflags at release time

func main() {
	// stdout belongs to command output (and to MCP's stdio transport!) —
	// logs would corrupt both
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	connect := func(ctx context.Context) (memories.Service, func(), error) {
		path := os.Getenv("AGMEM_DB")
		if path == "" {
			var err error
			if path, err = storage.DefaultPath(); err != nil {
				return nil, nil, err
			}
		}

		store, err := storage.Open(ctx, path)
		if err != nil {
			return nil, nil, err
		}

		pub := events.NewChannelPublisher()
		svc := memories.New(store.DB, pub)

		cleanup := func() {
			_ = pub.Close(context.Background()) // drain in-process reactions
			_ = store.Close()                   // releases the flock
		}
		return svc, cleanup, nil
	}

	runMCP := func(ctx context.Context, svc memories.Service) error {
		return mcpadapter.Run(ctx, mcpadapter.NewServer(svc, version))
	}

	// server-admin commands work on the server data dir directly
	connectKeys := func(ctx context.Context, dir string) (apikeys.Service, func(), error) {
		store, err := storage.OpenServer(ctx, filepath.Join(dir, "keys.db"))
		if err != nil {
			return nil, nil, err
		}
		return apikeys.New(store.DB), func() { _ = store.Close() }, nil
	}
	exportSpace := func(ctx context.Context, dir, space, dest string) error {
		return storage.ExportSnapshot(ctx, filepath.Join(dir, "spaces", space+".db"), dest)
	}

	root := cli.New(version, connect, runMCP,
		serveCmd(),
		apikeyscli.NewKeysCmd(connectKeys),
		apikeyscli.NewSpacesCmd(connectKeys, exportSpace),
	)
	os.Exit(cli.Execute(root))
}
