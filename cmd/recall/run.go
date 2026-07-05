package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/cli"
	mcpadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/mcp"
	tuiadapter "github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/adapters/tui"
	"github.com/KucherenkoIvan/go-agent-memory/internal/server"
)

// runCmd starts one of the long-running faces; which one is a flag, not a
// subcommand. MCP is the default — it is what harness configs invoke.
func runCmd(version string, connect cli.Connect) *cobra.Command {
	var (
		mcp, srv, tui bool
		addr, dir     string
	)
	cmd := &cobra.Command{
		Use:   "run [-m|-s|-t]",
		Short: "Run a long-lived face: MCP server (default), gRPC server, or TUI",
		Long: `Run one of recall's long-lived faces:

  -m, --mcp     MCP server over stdio (default) — the harness face
  -s, --server  gRPC server hosting shared memory (plaintext; VPN/TLS proxy in front)
  -t, --tui     interactive terminal UI for humans`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch {
			case srv:
				// long-running non-MCP process: logs go to stderr
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
				cfg, err := server.LoadConfig()
				if err != nil {
					return err
				}
				if addr != "" {
					cfg.Addr = addr
				}
				if dir != "" {
					cfg.Dir = dir
				}
				return server.Run(cmd.Context(), cfg)
			case tui:
				return tuiadapter.Run(cmd.Context(), tuiadapter.Connect(connect), version)
			default: // -m or no flag
				svc, cleanup, err := connect(cmd.Context())
				if err != nil {
					return err
				}
				defer cleanup()
				return mcpadapter.Run(cmd.Context(), mcpadapter.NewServer(svc, version))
			}
		},
	}
	cmd.Flags().BoolVarP(&mcp, "mcp", "m", false, "MCP server over stdio (default)")
	cmd.Flags().BoolVarP(&srv, "server", "s", false, "gRPC server hosting shared memory")
	cmd.Flags().BoolVarP(&tui, "tui", "t", false, "interactive terminal UI")
	cmd.MarkFlagsMutuallyExclusive("mcp", "server", "tui")
	cmd.Flags().StringVar(&addr, "addr", "", "server listen address (default :7846, env RECALL_ADDR)")
	cmd.Flags().StringVar(&dir, "dir", "", "server data dir (default ~/.local/share/recall/server, env RECALL_SERVER_DIR)")
	return cmd
}
