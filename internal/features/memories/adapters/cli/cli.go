// Package cli is the command-line transport adapter (cobra) for shell-out
// agents and scripts. Agent contract: stable JSON on stdout when it is not a
// TTY (or --output json), exit code 0 on success / 1 on any error with a
// machine-readable error JSON, and never a prompt.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/cliout"
)

// Connect is provided by the composition root: it opens the backend (local
// file today, remote later) and returns the facade plus a cleanup.
type Connect func(ctx context.Context) (memories.Service, func(), error)

type options struct {
	output string // auto|json|text
	source string
}

// New builds the root command. runMCP is the mcp subcommand body (lives in
// the composition root, since it owns the server lifecycle). extra hooks in
// commands owned by other slices or the composition root (serve, keys,
// spaces, tui) without this package importing them.
func New(version string, connect Connect, runMCP func(ctx context.Context, svc memories.Service) error, extra ...*cobra.Command) *cobra.Command {
	opts := &options{}

	root := &cobra.Command{
		Use:           "recall",
		Short:         "Harness-agnostic memory for AI agents",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&opts.output, "output", "auto", "output format: auto|json|text")
	root.PersistentFlags().StringVar(&opts.source, "source", "cli", "who is writing (harness/model/session)")

	withService := func(run func(cmd *cobra.Command, args []string, svc memories.Service) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := connect(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			return run(cmd, args, svc)
		}
	}

	root.AddCommand(
		storeCmd(opts, withService),
		searchCmd(opts, withService),
		getCmd(opts, withService),
		rateCmd(opts, withService),
		packCmd(withService),
		promptCmd(),
		mcpCmd(withService, runMCP),
		remoteCmd(opts),
	)
	root.AddCommand(extra...)
	return root
}

func storeCmd(opts *options, withService func(func(*cobra.Command, []string, memories.Service) error) func(*cobra.Command, []string) error) *cobra.Command {
	var (
		summary, kind, supersedes string
		keywords                  []string
		ttlHours                  int
	)
	cmd := &cobra.Command{
		Use:   "store [content]",
		Short: "Store a memory (content from the argument or stdin)",
		Args:  cobra.MaximumNArgs(1),
		RunE: withService(func(cmd *cobra.Command, args []string, svc memories.Service) error {
			content, err := contentFrom(args, cmd.InOrStdin())
			if err != nil {
				return err
			}
			id, err := svc.Store(cmd.Context(), managememories.StoreInput{
				Content: content, Summary: summary, Kind: kind, Keywords: keywords,
				Source: opts.source, TTLHours: ttlHours, Supersedes: supersedes,
			})
			if err != nil {
				return err
			}
			return emit(cmd, opts, map[string]string{"id": string(id)}, "stored "+string(id))
		}),
	}
	cmd.Flags().StringVarP(&summary, "summary", "s", "", "one-line summary (required)")
	cmd.Flags().StringVar(&kind, "kind", "", "fact|preference|research|decision|location|reference (required)")
	cmd.Flags().StringArrayVarP(&keywords, "keyword", "k", nil, "keyword (repeatable, at least one)")
	cmd.Flags().IntVar(&ttlHours, "ttl", 0, "expiry in hours (0 = never)")
	cmd.Flags().StringVar(&supersedes, "supersedes", "", "id of the memory this one corrects")
	_ = cmd.MarkFlagRequired("summary")
	_ = cmd.MarkFlagRequired("kind")
	_ = cmd.MarkFlagRequired("keyword")
	return cmd
}

func searchCmd(opts *options, withService func(func(*cobra.Command, []string, memories.Service) error) func(*cobra.Command, []string) error) *cobra.Command {
	var (
		query, kind, since, until string
		keywords                  []string
		limit                     int
		all                       bool
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search memories — returns ranked summaries",
		RunE: withService(func(cmd *cobra.Command, _ []string, svc memories.Service) error {
			filters := ports.SearchFilters{Query: query, Keywords: keywords, Kind: kind, Limit: limit, IncludeDead: all}
			var err error
			if filters.Since, err = parseTimeFlag(since); err != nil {
				return err
			}
			if filters.Until, err = parseTimeFlag(until); err != nil {
				return err
			}
			results, err := svc.Search(cmd.Context(), filters)
			if err != nil {
				return err
			}
			return emit(cmd, opts, map[string]any{"results": results}, formatResults(results))
		}),
	}
	cmd.Flags().StringVarP(&query, "query", "q", "", "full-text query")
	cmd.Flags().StringArrayVarP(&keywords, "keyword", "k", nil, "keyword filter (repeatable, all must match)")
	cmd.Flags().StringVar(&kind, "kind", "", "kind filter")
	cmd.Flags().StringVar(&since, "since", "", "created after (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "created before (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "max results (default 20)")
	cmd.Flags().BoolVar(&all, "all", false, "include superseded and expired")
	return cmd
}

func getCmd(opts *options, withService func(func(*cobra.Command, []string, memories.Service) error) func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Fetch a memory's full content",
		Args:  cobra.ExactArgs(1),
		RunE: withService(func(cmd *cobra.Command, args []string, svc memories.Service) error {
			model, err := svc.Get(cmd.Context(), domain.MemoryID(args[0]))
			if err != nil {
				return err
			}
			text := fmt.Sprintf("[%s] %s\nid: %s · %s · keywords: %s\n\n%s",
				model.Kind, model.Summary, model.ID,
				model.CreatedAt.Format("2006-01-02"), strings.Join(model.Keywords, ", "), model.Content)
			return emit(cmd, opts, model, text)
		}),
	}
}

func rateCmd(opts *options, withService func(func(*cobra.Command, []string, memories.Service) error) func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{
		Use:   "rate <id> <up|down>",
		Short: "Rate a memory after using it — ratings drive ranking",
		Args:  cobra.ExactArgs(2),
		RunE: withService(func(cmd *cobra.Command, args []string, svc memories.Service) error {
			if args[1] != "up" && args[1] != "down" {
				return fmt.Errorf("verdict must be up or down, got %q", args[1])
			}
			if err := svc.Rate(cmd.Context(), domain.MemoryID(args[0]), args[1] == "up"); err != nil {
				return err
			}
			return emit(cmd, opts, map[string]bool{"ok": true}, "rated "+args[1])
		}),
	}
}

func packCmd(withService func(func(*cobra.Command, []string, memories.Service) error) func(*cobra.Command, []string) error) *cobra.Command {
	var (
		keywords []string
		budget   int
	)
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Assemble top-ranked memories into one context block (keywords OR-match)",
		RunE: withService(func(cmd *cobra.Command, _ []string, svc memories.Service) error {
			pack, err := svc.Recall(cmd.Context(), keywords, budget)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), pack) // always the raw block — it IS the machine format
			return nil
		}),
	}
	cmd.Flags().StringArrayVarP(&keywords, "keyword", "k", nil, "keyword (repeatable, any may match — more matches rank higher)")
	cmd.Flags().IntVar(&budget, "budget", 0, "character budget (default 4000)")
	_ = cmd.MarkFlagRequired("keyword")
	return cmd
}

func promptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prompt",
		Short: "Print the usage instructions block for AGENTS.md/CLAUDE.md files",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), agentPrompt)
			return nil
		},
	}
}

func mcpCmd(withService func(func(*cobra.Command, []string, memories.Service) error) func(*cobra.Command, []string) error, runMCP func(ctx context.Context, svc memories.Service) error) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve the memory as an MCP server over stdio",
		RunE: withService(func(cmd *cobra.Command, _ []string, svc memories.Service) error {
			return runMCP(cmd.Context(), svc)
		}),
	}
}

// Execute runs the root command and renders errors per the agent contract.
func Execute(root *cobra.Command) int {
	if err := root.Execute(); err != nil {
		payload, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintln(os.Stderr, string(payload))
		return 1
	}
	return 0
}

func emit(cmd *cobra.Command, opts *options, jsonValue any, text string) error {
	return cliout.Emit(cmd.OutOrStdout(), opts.output, jsonValue, text)
}

func formatResults(results []domain.SearchResult) string {
	if len(results) == 0 {
		return "no memories found"
	}
	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "%s  [%s] %s\n    %s · %s · ↑%d ↓%d\n",
			r.ID, r.Kind, r.Summary,
			r.CreatedAt.Format("2006-01-02"), strings.Join(r.Keywords, ", "), r.VotesUp, r.VotesDown)
	}
	return strings.TrimRight(b.String(), "\n")
}

func contentFrom(args []string, stdin io.Reader) (string, error) {
	if len(args) == 1 && args[0] != "-" {
		return args[0], nil
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("reading content from stdin: %w", err)
	}
	return string(raw), nil
}

func parseTimeFlag(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("time must be RFC3339 or YYYY-MM-DD, got %q", raw)
	}
	return t, nil
}
