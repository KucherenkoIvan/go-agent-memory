// Package cli holds the server-admin commands: keys (create/list/revoke)
// and spaces (list/export). They run against the server data dir directly —
// safe while `recall serve` is up, same WAL multi-process model as local.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/domain"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/cliout"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

// Connect opens the key service over a server data dir; owned by the
// composition root like the memories Connect.
type Connect func(ctx context.Context, dir string) (apikeys.Service, func(), error)

// Export snapshots one space's database to a destination path.
type Export func(ctx context.Context, dir, space, dest string) error

// resolveDir: --dir flag > RECALL_SERVER_DIR > default.
func resolveDir(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env := os.Getenv("RECALL_SERVER_DIR"); env != "" {
		return env, nil
	}
	return storage.DefaultServerDir()
}

// outputMode reads the root --output persistent flag when present.
func outputMode(cmd *cobra.Command) string {
	if f := cmd.Flag("output"); f != nil {
		return f.Value.String()
	}
	return "auto"
}

func NewKeysCmd(connect Connect) *cobra.Command {
	var dir string
	root := &cobra.Command{
		Use:   "keys",
		Short: "Manage API keys for hosted mode (server-side)",
	}
	root.PersistentFlags().StringVar(&dir, "dir", "", "server data dir (default ~/.local/share/recall/server, env RECALL_SERVER_DIR)")

	withService := func(run func(cmd *cobra.Command, args []string, svc apikeys.Service) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			resolved, err := resolveDir(dir)
			if err != nil {
				return err
			}
			svc, cleanup, err := connect(cmd.Context(), resolved)
			if err != nil {
				return err
			}
			defer cleanup()
			return run(cmd, args, svc)
		}
	}

	var name, space string
	create := &cobra.Command{
		Use:   "create",
		Short: "Mint an API key into a space — the key is printed exactly once",
		RunE: withService(func(cmd *cobra.Command, _ []string, svc apikeys.Service) error {
			result, err := svc.Create(cmd.Context(), name, space)
			if err != nil {
				return err
			}
			text := fmt.Sprintf("key %s (%s) for space %s\n%s\nsave it now — it is shown exactly once and stored only as a hash",
				result.Key.ID, result.Key.Name, result.Key.Space, result.RawToken)
			return cliout.Emit(cmd.OutOrStdout(), outputMode(cmd), map[string]string{
				"id": result.Key.ID, "name": result.Key.Name, "space": result.Key.Space,
				"prefix": result.Key.Prefix, "key": result.RawToken,
			}, text)
		}),
	}
	create.Flags().StringVar(&name, "name", "", "key label, becomes memory source (required)")
	create.Flags().StringVar(&space, "space", "", "memory space this key unlocks (required, [a-z0-9-]+)")
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("space")

	list := &cobra.Command{
		Use:   "list",
		Short: "List API keys (never shows key material)",
		RunE: withService(func(cmd *cobra.Command, _ []string, svc apikeys.Service) error {
			keys, err := svc.List(cmd.Context())
			if err != nil {
				return err
			}
			var b strings.Builder
			for _, k := range keys {
				state := "active"
				if k.RevokedAt != nil {
					state = "revoked " + k.RevokedAt.Format("2006-01-02")
				}
				fmt.Fprintf(&b, "%s  %s…  %s  space:%s  %s\n", k.ID, k.Prefix, k.Name, k.Space, state)
			}
			return cliout.Emit(cmd.OutOrStdout(), outputMode(cmd),
				map[string]any{"keys": keys}, strings.TrimRight(b.String(), "\n"))
		}),
	}

	revoke := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke an API key — other keys into the space keep working",
		Args:  cobra.ExactArgs(1),
		RunE: withService(func(cmd *cobra.Command, args []string, svc apikeys.Service) error {
			if err := svc.Revoke(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cliout.Emit(cmd.OutOrStdout(), outputMode(cmd), map[string]bool{"ok": true}, "revoked "+args[0])
		}),
	}

	root.AddCommand(create, list, revoke)
	return root
}

func NewSpacesCmd(connect Connect, export Export) *cobra.Command {
	var dir string
	root := &cobra.Command{
		Use:   "spaces",
		Short: "Inspect and export memory spaces (server-side)",
	}
	root.PersistentFlags().StringVar(&dir, "dir", "", "server data dir (default ~/.local/share/recall/server, env RECALL_SERVER_DIR)")

	list := &cobra.Command{
		Use:   "list",
		Short: "List spaces with their active key counts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := resolveDir(dir)
			if err != nil {
				return err
			}
			svc, cleanup, err := connect(cmd.Context(), resolved)
			if err != nil {
				return err
			}
			defer cleanup()

			spaces, err := svc.Spaces(cmd.Context())
			if err != nil {
				return err
			}
			var b strings.Builder
			for _, s := range spaces {
				fmt.Fprintf(&b, "%s  keys:%d  since %s\n", s.Name, s.Keys, s.CreatedAt.Format("2006-01-02"))
			}
			return cliout.Emit(cmd.OutOrStdout(), outputMode(cmd),
				map[string]any{"spaces": spaces}, strings.TrimRight(b.String(), "\n"))
		},
	}

	exportCmd := &cobra.Command{
		Use:   "export <space> <path>",
		Short: "Snapshot a space to a file — the result IS a local memory.db",
		Long: `Snapshot a space's database to a file (live-safe under WAL).

The snapshot is byte-compatible with a local memory database: point
RECALL_DB at it (or drop it at ~/.local/share/recall/memory.db) and the
space's memories are fully local.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := domain.ValidateSpaceName(args[0]); err != nil {
				return err
			}
			resolved, err := resolveDir(dir)
			if err != nil {
				return err
			}
			if err := export(cmd.Context(), resolved, args[0], args[1]); err != nil {
				return err
			}
			return cliout.Emit(cmd.OutOrStdout(), outputMode(cmd),
				map[string]any{"ok": true, "path": args[1]}, "exported "+args[0]+" → "+args[1])
		},
	}

	root.AddCommand(list, exportCmd)
	return root
}
