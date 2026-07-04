package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/KucherenkoIvan/go-agent-memory/internal/server"
)

// serveCmd lives in the composition root: it owns the server lifecycle the
// same way main owns connect(). Unlike the agent-facing commands, serve is
// a long-running process — its logs go to stderr (stdout stays sacred).
func serveCmd() *cobra.Command {
	var addr, dir string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Host shared memory over gRPC (plaintext — put TLS/VPN in front)",
		Long: `Host shared memory over gRPC.

API keys (see 'agmem keys') both authenticate callers and select their
memory space. Transport is plaintext: deploy on a private network or
behind a TLS-terminating reverse proxy.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "", "listen address (default :7846, env AGMEM_ADDR)")
	cmd.Flags().StringVar(&dir, "dir", "", "server data dir (default ~/.local/share/agmem/server, env AGMEM_SERVER_DIR)")
	return cmd
}
