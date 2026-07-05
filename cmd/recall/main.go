// recall — harness-agnostic memory for AI agents. One binary, three faces
// behind `recall run`: MCP (default), gRPC server, and TUI; plus one-shot
// CLI commands for shell-out agents and admins.
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
	"github.com/spf13/cobra"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys"
	apikeyscli "github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/adapters/cli"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/cli"
	grpcadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/grpc"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/remotecfg"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

var version = "dev" // set via -ldflags; go-install builds resolve from build info

// resolveVersion prefers the ldflags stamp, then the module version Go
// embeds for `go install @tag` builds, and annotates with the VCS revision
// when available.
func resolveVersion() string {
	v := version
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v
	}
	if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	revision, modified := "", false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if len(revision) >= 12 {
		v += " (" + revision[:12]
		if modified {
			v += ", modified"
		}
		v += ")"
	}
	return v
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

	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Administer a hosted memory server (keys, spaces)",
	}
	serverCmd.AddCommand(
		apikeyscli.NewKeysCmd(connectKeys),
		apikeyscli.NewSpacesCmd(connectKeys, exportSpace),
	)

	root := cli.New(version, connect,
		runCmd(version, connect),
		serverCmd,
	)
	os.Exit(cli.Execute(root))
}
