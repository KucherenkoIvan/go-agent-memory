// recall — harness-agnostic memory for AI agents. One binary, three faces:
// CLI subcommands for shell-out agents, `recall mcp` for MCP harnesses, and
// (phase 2) `recall tui` for humans.
package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/KucherenkoIvan/go-kernel/events"
	"github.com/KucherenkoIvan/go-kernel/grpckit"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys"
	apikeyscli "github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/adapters/cli"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/cli"
	grpcadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/grpc"
	mcpadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/mcp"
	tuiadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/tui"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/remotecfg"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

var version = "dev" // set via -ldflags; go-install builds resolve from build info

// resolveVersion prefers the ldflags stamp, falling back to the module
// version Go embeds when the binary was installed via `go install @tag`.
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func main() {
	// stdout belongs to command output (and to MCP's stdio transport!) —
	// logs would corrupt both
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	version := resolveVersion()

	connect := func(ctx context.Context) (memories.Service, func(), error) {
		// remote mode: one config file flips every face to the hosted memory
		if remote, err := remotecfg.Resolve(); err != nil {
			return nil, nil, err
		} else if remote != nil {
			conn, err := grpckit.Connect(remote.Addr)
			if err != nil {
				return nil, nil, err
			}
			return grpcadapter.NewClient(conn, remote.APIKey), func() { _ = conn.Close() }, nil
		}

		path := os.Getenv("RECALL_DB")
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
			_ = store.Close()
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
		tuiadapter.NewCmd(tuiadapter.Connect(connect), version),
		versionCmd(version),
	)
	os.Exit(cli.Execute(root))
}
