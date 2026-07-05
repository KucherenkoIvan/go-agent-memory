package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

const debounceDelay = 300 * time.Millisecond

func debounceCmd(seq int) tea.Cmd {
	return tea.Tick(debounceDelay, func(time.Time) tea.Msg { return debounceMsg{seq: seq} })
}

func searchCmd(ctx context.Context, svc memories.Service, seq int, spec searchSpec) tea.Cmd {
	return func() tea.Msg {
		results, err := runSearch(ctx, svc, spec)
		if err != nil {
			return errMsg{op: "search", err: err}
		}
		return searchDoneMsg{seq: seq, results: results}
	}
}

func getCmd(ctx context.Context, svc memories.Service, id string, openForm bool) tea.Cmd {
	return func() tea.Msg {
		model, err := svc.Get(ctx, domain.MemoryID(id))
		if err != nil {
			return errMsg{op: "get", err: err}
		}
		return memoryLoadedMsg{model: model, openForm: openForm}
	}
}

func rateCmd(ctx context.Context, svc memories.Service, id string, up bool) tea.Cmd {
	return func() tea.Msg {
		if err := svc.Rate(ctx, domain.MemoryID(id), up); err != nil {
			return errMsg{op: "rate", err: err}
		}
		return ratedMsg{id: id, up: up}
	}
}

func deleteCmd(ctx context.Context, svc memories.Service, id string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.Delete(ctx, domain.MemoryID(id)); err != nil {
			return errMsg{op: "delete", err: err}
		}
		return deletedMsg{id: id}
	}
}

func storeCmd(ctx context.Context, svc memories.Service, in managememories.StoreInput) tea.Cmd {
	return func() tea.Msg {
		id, err := svc.Store(ctx, in)
		if err != nil {
			return errMsg{op: "store", err: err}
		}
		return storedMsg{id: id}
	}
}

// reconnectCmd tears down the current service and rebuilds through the
// composition root's connect — which re-reads the remote config.
func reconnectCmd(ctx context.Context, cleanup func(), connect Connect) tea.Cmd {
	return func() tea.Msg {
		if cleanup != nil {
			cleanup()
		}
		svc, newCleanup, err := connect(ctx)
		if err != nil {
			return reconnectFailedMsg{err: err}
		}
		return reconnectedMsg{svc: svc, cleanup: newCleanup}
	}
}
