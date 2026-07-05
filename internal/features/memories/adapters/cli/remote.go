package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/KucherenkoIvan/go-kernel/grpckit"
	"github.com/spf13/cobra"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"

	grpcadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/grpc"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/remotecfg"
)

const probeTimeout = 5 * time.Second

// remoteCmd manages the client's remote endpoint. Once set, every face
// (CLI, MCP, TUI) talks to the hosted memory instead of the local file.
func remoteCmd(opts *options) *cobra.Command {
	root := &cobra.Command{
		Use:   "remote",
		Short: "Attach this recall to a hosted memory (or detach back to local)",
	}
	root.AddCommand(remoteSetCmd(opts), remoteUnsetCmd(opts), remoteStatusCmd(opts))
	return root
}

func remoteSetCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "set <addr> <api-key>",
		Short: "Point this recall at a hosted memory (verified before saving)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, key := args[0], args[1]
			if !strings.HasPrefix(key, "rcl_") {
				return fmt.Errorf("api key must start with rcl_ — got %q", mask(key))
			}

			// fail fast on typos: reach the server AND authenticate
			ctx, cancel := context.WithTimeout(cmd.Context(), probeTimeout)
			defer cancel()
			if server, auth := probe(ctx, addr, key); server != "ok" {
				return fmt.Errorf("server %s is not reachable (%s) — config not saved", addr, server)
			} else if auth != "ok" {
				return fmt.Errorf("server %s rejected the API key (%s) — config not saved", addr, auth)
			}

			if err := remotecfg.Save(remotecfg.Config{Addr: addr, APIKey: key}); err != nil {
				return err
			}
			return emit(cmd, opts, map[string]any{"ok": true, "addr": addr},
				"remote set: "+addr+" (key verified)")
		},
	}
}

func remoteUnsetCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "unset",
		Short: "Detach from the hosted memory — back to the local file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := remotecfg.Remove(); err != nil {
				return err
			}
			return emit(cmd, opts, map[string]any{"ok": true}, "remote unset — using local memory")
		},
	}
}

func remoteStatusCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the effective remote config and probe the server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := remotecfg.Resolve()
			if err != nil {
				return err
			}
			if cfg == nil {
				return emit(cmd, opts, map[string]any{"mode": "local"}, "local mode — no remote configured")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), probeTimeout)
			defer cancel()
			server, auth := probe(ctx, cfg.Addr, cfg.APIKey)
			return emit(cmd, opts, map[string]any{
				"mode": "remote", "addr": cfg.Addr, "apiKey": mask(cfg.APIKey),
				"server": server, "auth": auth,
			}, fmt.Sprintf("remote: %s · key %s · server %s · auth %s", cfg.Addr, mask(cfg.APIKey), server, auth))
		},
	}
}

// probe checks reachability (unauthenticated health) and key validity (an
// authenticated one-row search).
func probe(ctx context.Context, addr, key string) (server, auth string) {
	conn, err := grpckit.Connect(addr)
	if err != nil {
		return "down", "unknown"
	}
	defer conn.Close() //nolint:errcheck // probe connection

	if _, err := healthv1.NewHealthClient(conn).Check(ctx, &healthv1.HealthCheckRequest{}); err != nil {
		return "down", "unknown"
	}
	if _, err := grpcadapter.NewClient(conn, key).Search(ctx, ports.SearchFilters{Limit: 1}); err != nil {
		return "ok", "invalid"
	}
	return "ok", "ok"
}

func mask(key string) string {
	if len(key) <= 12 {
		return "****"
	}
	return key[:12] + "…" + key[len(key)-4:]
}
