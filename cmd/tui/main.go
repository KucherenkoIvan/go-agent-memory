// The second composition root: the same feature set as cmd/app, presented as
// a terminal application. No HTTP, no gRPC — use-cases are called in-process,
// the Bubble Tea loop owns the process lifecycle.
//
// One rule carried over from the sqlite guide: one process per database
// file — don't run the TUI against a file a server instance is using.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/KucherenkoIvan/go-kernel/config"
	"github.com/KucherenkoIvan/go-kernel/ddd"
	"github.com/KucherenkoIvan/go-kernel/events"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/adapters/tui"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/shared/infra/storage"
)

type TUIConfig struct {
	DBPath string `env:"DB_PATH" default:"app.db"`
}

func main() {
	// the terminal belongs to the UI — logs to stdout would corrupt it
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	cfg := config.MustLoad[TUIConfig]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "opening storage:", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	pub := events.NewChannelPublisher()
	feature := changeme.New(db, pub)

	program := tea.NewProgram(tui.New(feature.UseCases), tea.WithAltScreen())

	// committed domain events drive the UI — the SSE pattern, terminal edition
	pub.Subscribe(events.Handler{
		Name: "tui-refresh",
		Handle: func(context.Context, ddd.DomainEvent) error {
			program.Send(tui.RefreshMsg{})
			return nil
		},
	})
	go func() { _ = pub.Run(ctx) }()

	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cancel()
	_ = pub.Close(context.Background())
}
