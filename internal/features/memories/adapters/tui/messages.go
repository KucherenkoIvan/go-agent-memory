package tui

import (
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// Typed results of async Service calls. Every command returns exactly one
// of these; the update loop never blocks on the network.
type (
	// debounceMsg fires after the typing pause; stale seqs are dropped.
	debounceMsg struct{ seq int }

	// searchDoneMsg carries the seq of the query that produced it — the
	// list ignores results that are not the latest issued query.
	searchDoneMsg struct {
		seq     int
		results []domain.SearchResult
		// fetch distinguishes a fresh query from pagination follow-ups.
		fetch fetchMode
		// expected is the requested row count — fewer back means exhausted.
		expected int
		// total is the store's exact match count (timeline only; 0 under
		// layered search, where no single total exists).
		total int
	}

	memoryLoadedMsg struct {
		model *domain.MemoryReadModel
		// openForm: e (supersede) needs the full body before the editor opens
		openForm bool
	}

	ratedMsg struct {
		id string
		up bool
	}

	deletedMsg struct{ id string }

	storedMsg struct{ id domain.MemoryID }

	reconnectedMsg struct {
		svc     memories.Service
		cleanup func()
	}

	reconnectFailedMsg struct{ err error }

	errMsg struct {
		op  string
		err error
	}
)
