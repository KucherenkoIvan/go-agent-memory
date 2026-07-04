// Package managememories holds the memories use-cases: store/rate commands
// and the search/get/recall queries.
package managememories

import (
	"context"

	"github.com/KucherenkoIvan/go-kernel/ddd"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// StoreInput carries everything a writer provides.
type StoreInput struct {
	Content  string
	Summary  string
	Kind     string
	Keywords []string
	Source   string
	TTLHours int
	// Supersedes marks an existing memory as corrected by this one.
	Supersedes string
}

// StoreCommand — build the memory, persist it (and the superseded one)
// atomically, publish the facts.
type StoreCommand struct {
	txManager ddd.TxManager
	ids       ddd.IDGenerator
	clock     ddd.Clock
	repo      ports.MemoryRepository
	events    ports.MemoryEventProducer
}

func NewStoreCommand(txManager ddd.TxManager, ids ddd.IDGenerator, clock ddd.Clock, repo ports.MemoryRepository, events ports.MemoryEventProducer) *StoreCommand {
	return &StoreCommand{txManager: txManager, ids: ids, clock: clock, repo: repo, events: events}
}

func (c *StoreCommand) Execute(ctx context.Context, in StoreInput) (domain.MemoryID, error) {
	memory, err := domain.NewMemory(
		domain.MemoryID(c.ids.NewID()),
		in.Content, in.Summary, domain.Kind(in.Kind), in.Keywords, in.Source, in.TTLHours,
		c.clock.Now().UTC(),
	)
	if err != nil {
		return "", err
	}

	err = c.txManager.WithinTx(ctx, func(tx ddd.Transaction) error {
		if in.Supersedes != "" {
			old, err := c.repo.GetByID(ctx, tx, domain.MemoryID(in.Supersedes))
			if err != nil {
				return err
			}
			if old == nil {
				return &domain.MemoryNotFoundError{}
			}
			if err := old.MarkSuperseded(memory.ID()); err != nil {
				return err
			}
			if err := c.repo.Save(ctx, tx, old); err != nil {
				return err
			}
			if err := c.events.Publish(ctx, tx, old.PopEvents()...); err != nil {
				return err
			}
		}
		if err := c.repo.Save(ctx, tx, memory); err != nil {
			return err
		}
		return c.events.Publish(ctx, tx, memory.PopEvents()...)
	})
	if err != nil {
		return "", err
	}
	return memory.ID(), nil
}

// RateCommand — explicit usefulness feedback after an agent used a memory.
type RateCommand struct {
	txManager ddd.TxManager
	repo      ports.MemoryRepository
	events    ports.MemoryEventProducer
}

func NewRateCommand(txManager ddd.TxManager, repo ports.MemoryRepository, events ports.MemoryEventProducer) *RateCommand {
	return &RateCommand{txManager: txManager, repo: repo, events: events}
}

func (c *RateCommand) Execute(ctx context.Context, id domain.MemoryID, up bool) error {
	return c.txManager.WithinTx(ctx, func(tx ddd.Transaction) error {
		memory, err := c.repo.GetByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if memory == nil {
			return &domain.MemoryNotFoundError{}
		}
		if err := memory.Rate(up); err != nil {
			return err
		}
		if err := c.repo.Save(ctx, tx, memory); err != nil {
			return err
		}
		return c.events.Publish(ctx, tx, memory.PopEvents()...)
	})
}
