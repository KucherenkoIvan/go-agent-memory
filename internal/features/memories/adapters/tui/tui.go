// Package tui is the human face: browse, search, read, rate, prune, and
// correct memories — plus remote endpoint management. It sits on the same
// Service facade as every other face, so it works against local and hosted
// memory alike.
package tui

import (
	"context"
	"errors"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/remotecfg"
)

// Connect is provided by the composition root; the TUI keeps it so a remote
// config change can rebuild the service without restarting.
type Connect func(ctx context.Context) (memories.Service, func(), error)

// remoteUnset is what ctrl+x on the remote screen does.
func remoteUnset() error { return remotecfg.Remove() }

// Run starts the TUI. It refuses non-interactive terminals — the agent
// contract (JSON on stdout) belongs to the other faces.
func Run(ctx context.Context, connect Connect, version string) error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("tui requires an interactive terminal")
	}

	svc, cleanup, err := connect(ctx)
	if err != nil {
		return err
	}

	app := newApp(ctx, svc, cleanup, connect, version)
	_, err = tea.NewProgram(app, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}

// NewCmd is the cobra face, injected into the root command by main.
func NewCmd(connect Connect, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Browse and curate memories interactively (humans)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return Run(cmd.Context(), connect, version)
		},
	}
}
