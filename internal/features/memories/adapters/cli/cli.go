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
// file or remote client) and returns the facade plus a cleanup.
type Connect func(ctx context.Context) (memories.Service, func(), error)

type options struct {
	output string // auto|json|text
	source string
}

// New builds the root command: the `memory` block, prompt, remote, and
// whatever the composition root hooks in via extra (run, server).
func New(version string, connect Connect, extra ...*cobra.Command) *cobra.Command {
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

	// -v prints the version; completion works but stays out of the listing.
	root.Flags().BoolP("version", "v", false, "print version information")
	root.CompletionOptions.HiddenDefaultCmd = true

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
		memoryCmd(opts, withService),
		promptCmd(),
		remoteCmd(opts),
	)
	root.AddCommand(extra...)

	// `recall help x` keeps working, but the listing shows only real
	// commands: cobra's default template force-includes help by name, so
	// strip that special case (a cobra rewording makes this a no-op).
	root.SetUsageTemplate(strings.Replace(root.UsageTemplate(),
		`(or .IsAvailableCommand (eq .Name "help"))`, ".IsAvailableCommand", 1))
	root.InitDefaultHelpCmd()
	for _, sub := range root.Commands() {
		if sub.Name() == "help" {
			sub.Hidden = true
		}
	}
	return root
}

// memoryCmd groups everything that touches memories themselves.
func memoryCmd(opts *options, withService func(func(*cobra.Command, []string, memories.Service) error) func(*cobra.Command, []string) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Store, search, read, rate, and pack memories",
	}
	cmd.AddCommand(
		storeCmd(opts, withService),
		searchCmd(opts, withService),
		getCmd(opts, withService),
		rateCmd(opts, withService),
		packCmd(withService),
	)
	return cmd
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
		text, kind, since, until string
		limit                    int
		all, and                 bool
	)
	cmd := &cobra.Command{
		Use:   "search [keyword]...",
		Short: "Search memories — keywords OR-match and boost rank, like pack",
		Long: `Search memories and print ranked summaries.

Positional arguments are keywords: any may match, and every additional
match boosts a memory's rank (the same semantics pack uses). --text
layers a full-text query over the keyword results. No arguments at all
returns the recent timeline.`,
		RunE: withService(func(cmd *cobra.Command, args []string, svc memories.Service) error {
			filters := ports.SearchFilters{Query: text, Kind: kind, Limit: limit, IncludeDead: all}
			if and {
				filters.Keywords = args
			} else {
				filters.KeywordsAny = args
			}
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
	cmd.Flags().StringVar(&text, "text", "", "full-text query layered on the keyword results")
	cmd.Flags().BoolVar(&and, "and", false, "require every keyword to match instead of any")
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
		text   string
		budget int
	)
	cmd := &cobra.Command{
		Use:   "pack <keyword>...",
		Short: "Assemble top-ranked memories into one context block",
		Long: `Assemble the top-ranked memories for the keywords into one markdown
block within a character budget — the session bootstrap.

Keywords OR-match: throw in every candidate topic; memories matching
more of them rank higher. --text layers a full-text query on top.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && text == "" {
				return fmt.Errorf("give at least one keyword (or --text)")
			}
			return nil
		},
		RunE: withService(func(cmd *cobra.Command, args []string, svc memories.Service) error {
			pack, err := svc.Recall(cmd.Context(), args, text, budget)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), pack) // always the raw block — it IS the machine format
			return nil
		}),
	}
	cmd.Flags().StringVar(&text, "text", "", "full-text query layered on the keyword results")
	cmd.Flags().IntVar(&budget, "budget", 0, "character budget (default 4000)")
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
